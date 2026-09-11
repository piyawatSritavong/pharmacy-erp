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

async function proxy(request: NextRequest, path: string[]) {
  let backendURL: string;
  try {
    backendURL = resolveBackendURL();
  } catch (error) {
    if (error instanceof BackendURLNotConfiguredError) {
      // 503, not 500: the service is correctly built and running, and the one
      // thing it is missing is named in the body and in the log.
      console.error(error.message);
      return NextResponse.json({ message: error.message }, { status: 503 });
    }
    throw error;
  }

  const token = (await cookies()).get(AUTH_COOKIE)?.value;
  const target = `${backendURL}/api/v1/${path.join("/")}${request.nextUrl.search}`;
	const body = request.method === "GET" || request.method === "HEAD"
		? undefined
		: await request.arrayBuffer();
	const contentType = request.headers.get("content-type");

  const response = await fetch(target, {
    method: request.method,
		headers: {
			...(contentType ? { "Content-Type": contentType } : {}),
			...(token ? { Authorization: `Bearer ${token}` } : {})
    },
    body,
    cache: "no-store"
  });

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

	if (path.join("/") === "auth/login" && response.ok) {
		const login = JSON.parse(new TextDecoder().decode(payload)) as { token: string };
		const secure = requestIsHTTPS(request);
		proxied.cookies.set(AUTH_COOKIE, login.token, {
      httpOnly: true,
      sameSite: "lax",
      secure,
      path: "/"
    });
  }

  if (path.join("/") === "auth/logout") {
    proxied.cookies.delete(AUTH_COOKIE);
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
