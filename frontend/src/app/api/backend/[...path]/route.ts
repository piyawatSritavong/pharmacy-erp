import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";

import { AUTH_COOKIE } from "@/lib/auth";
import { BackendURLNotConfiguredError, resolveBackendURL } from "@/lib/backend-url";


/**
 * Route handlers that proxy a request are dynamic by definition — there is
 * nothing here to prerender. Saying so keeps `next build` from evaluating this
 * module to collect page data, which is when it would otherwise reach for an
 * environment the build does not have.
 */
export const dynamic = "force-dynamic";

/**
 * Whether the browser reached us over TLS.
 *
 * Render terminates TLS at its edge and forwards plain HTTP to the container,
 * so the request URL always says http:// in production and cannot be used to
 * decide this. The edge records the original scheme in x-forwarded-proto;
 * that is read first, and the URL's own scheme is the fallback for anything
 * that runs without a proxy in front of it. When several proxies have
 * appended to the header it is a comma-separated list and the first entry is
 * the one the browser used.
 *
 * The Secure attribute used to be `process.env.NODE_ENV === "production"`,
 * which the build folds to a constant: true everywhere, including on a plain
 * HTTP origin, where a browser will store the cookie and then never send it.
 * Deriving it from the real scheme cannot weaken anything on HTTPS — there
 * the answer is the same as before — and stops the cookie being unusable on
 * HTTP.
 */
function requestIsHTTPS(request: NextRequest): boolean {
  const forwarded = request.headers.get("x-forwarded-proto");
  if (forwarded) {
    return forwarded.split(",")[0].trim().toLowerCase() === "https";
  }
  return request.nextUrl.protocol === "https:";
}

/**
 * Ends the session by clearing the auth cookie, explicitly.
 *
 * The cookie is the session: the Go API's /auth/logout does nothing but return
 * a message, there is no server-side session store, and a stateless JWT cannot
 * be revoked. Clearing this cookie is therefore the whole of logging out, and
 * it must not be contingent on that API call succeeding — it used to be,
 * because the delete ran only on the path where the upstream had answered. A
 * backend that was down left the browser holding a live session while being
 * shown a login screen.
 *
 * Cleared by writing it expired with the same attributes it was set with,
 * rather than by name alone: a delete whose path or security attributes differ
 * from the original can leave the original in place.
 */
function endSession(request: NextRequest, status: number, message: string) {
  const response = NextResponse.json({ message }, { status });
  response.cookies.set(AUTH_COOKIE, "", {
    httpOnly: true,
    sameSite: "lax",
    secure: requestIsHTTPS(request),
    path: "/",
    maxAge: 0
  });
  return response;
}

async function proxy(request: NextRequest, path: string[]) {
  const route = path.join("/");
  const isLogout = route === "auth/logout";

  let backendURL: string;
  try {
    backendURL = resolveBackendURL();
  } catch (error) {
    if (error instanceof BackendURLNotConfiguredError) {
      // 503, not 500: the service is correctly built and running, and the one
      // thing it is missing is named in the body and in the log.
      console.error(error.message);
      // A logout still ends the session — clearing this cookie needs nothing
      // from the API, least of all its address.
      if (isLogout) return endSession(request, 200, "ออกจากระบบแล้ว");
      return NextResponse.json({ message: error.message }, { status: 503 });
    }
    throw error;
  }

  const token = (await cookies()).get(AUTH_COOKIE)?.value;
  const target = `${backendURL}/api/v1/${route}${request.nextUrl.search}`;
	const body = request.method === "GET" || request.method === "HEAD"
		? undefined
		: await request.arrayBuffer();
	const contentType = request.headers.get("content-type");

  let response: Response;
  try {
    response = await fetch(target, {
      method: request.method,
      headers: {
        ...(contentType ? { "Content-Type": contentType } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {})
      },
      body,
      cache: "no-store"
    });
  } catch (error) {
    // The API was unreachable. For every other route that is the caller's
    // problem, but a logout against a stateless API has succeeded the moment
    // the cookie goes: keeping the session alive because a no-op call failed
    // is the worse outcome, especially on a till somebody is walking away from.
    if (isLogout) {
      const cause = (error as { cause?: { code?: unknown } })?.cause;
      console.error(JSON.stringify({
        event: "logout.upstream_unreachable",
        detail: "session ended locally; the API was not reached",
        error_message: error instanceof Error ? error.message : String(error),
        cause_code: typeof cause?.code === "string" ? cause.code : null
      }));
      return endSession(request, 200, "ออกจากระบบแล้ว");
    }
    throw error;
  }

	const payload = await response.arrayBuffer();
	const responseType = response.headers.get("content-type") || "application/octet-stream";
	const proxied = new NextResponse(payload, {
		status: response.status,
		headers: {
			"Content-Type": responseType,
			...(response.headers.get("content-disposition")
				? { "Content-Disposition": response.headers.get("content-disposition") as string }
				: {}),
			...(response.headers.get("cache-control")
				? { "Cache-Control": response.headers.get("cache-control") as string }
				: {})
		}
	});

	if (route === "auth/login" && response.ok) {
		const login = JSON.parse(new TextDecoder().decode(payload)) as { token: string };
		const secure = requestIsHTTPS(request);
		proxied.cookies.set(AUTH_COOKIE, login.token, {
      httpOnly: true,
      sameSite: "lax",
      secure,
      path: "/"
    });
  }

  if (isLogout) {
    // Answer 200 whatever the upstream said, having cleared the cookie.
    //
    // Mirroring the upstream status here was wrong in a way that only shows in
    // production. An unreachable API does not always make fetch reject: the
    // blueprint points BACKEND_INTERNAL_URL at the API's public URL, so the
    // call goes through the platform edge, and an edge whose service is down
    // answers 502 rather than refusing the connection. fetch resolves, this
    // path runs, the cookie is cleared — and returning that 502 made
    // proxyClient throw, so the button reported a failed logout and stayed on
    // an authenticated screen whose session had in fact just ended. The
    // inverse of the bug this whole change set out to fix.
    //
    // With this, a non-2xx reaching the client means exactly one thing: the
    // route never ran. That is precisely when staying put is right.
    if (!response.ok) {
      console.error(JSON.stringify({
        event: "logout.upstream_error",
        detail: "session ended locally; the API answered with an error",
        upstream_status: response.status
      }));
    }
    return endSession(request, 200, "ออกจากระบบแล้ว");
  }

  return proxied;
}

type RouteContext = { params: Promise<{ path: string[] }> };

export async function GET(request: NextRequest, { params }: RouteContext) {
  return proxy(request, (await params).path);
}

export async function POST(request: NextRequest, { params }: RouteContext) {
  return proxy(request, (await params).path);
}

export async function PUT(request: NextRequest, { params }: RouteContext) {
	return proxy(request, (await params).path);
}

export async function PATCH(request: NextRequest, { params }: RouteContext) {
	return proxy(request, (await params).path);
}

export async function DELETE(request: NextRequest, { params }: RouteContext) {
	return proxy(request, (await params).path);
}
