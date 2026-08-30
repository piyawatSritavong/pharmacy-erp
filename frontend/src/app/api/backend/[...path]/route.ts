import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";

import { AUTH_COOKIE } from "@/lib/auth";

const backendURL =
  process.env.BACKEND_INTERNAL_URL ||
  process.env.NEXT_PUBLIC_BACKEND_URL ||
  "http://localhost:8080";

async function proxy(request: NextRequest, path: string[]) {
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
