import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";

import { AUTH_COOKIE } from "@/lib/auth";

const backendURL =
  process.env.BACKEND_INTERNAL_URL ||
  process.env.NEXT_PUBLIC_BACKEND_URL ||
  "http://localhost:8080";

async function proxy(request: NextRequest, path: string[]) {
  const token = cookies().get(AUTH_COOKIE)?.value;
  const target = `${backendURL}/api/v1/${path.join("/")}${request.nextUrl.search}`;
  const body =
    request.method === "GET" || request.method === "DELETE"
      ? undefined
      : await request.text();

  const response = await fetch(target, {
    method: request.method,
    headers: {
      "Content-Type": request.headers.get("content-type") || "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {})
    },
    body,
    cache: "no-store"
  });

  const text = await response.text();
  const proxied = new NextResponse(text, {
    status: response.status,
    headers: {
      "Content-Type": response.headers.get("content-type") || "application/json"
    }
  });

  if (path.join("/") === "auth/login" && response.ok) {
    const payload = JSON.parse(text) as { token: string };
    proxied.cookies.set(AUTH_COOKIE, payload.token, {
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

export async function GET(request: NextRequest, { params }: { params: { path: string[] } }) {
  return proxy(request, params.path);
}

export async function POST(request: NextRequest, { params }: { params: { path: string[] } }) {
  return proxy(request, params.path);
}

export async function PUT(request: NextRequest, { params }: { params: { path: string[] } }) {
  return proxy(request, params.path);
}
