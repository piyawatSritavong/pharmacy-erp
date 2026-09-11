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
		proxied.cookies.set(AUTH_COOKIE, login.token, {
      httpOnly: true,
      sameSite: "lax",
      secure: process.env.NODE_ENV === "production",
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
