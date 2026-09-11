import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import { AUTH_COOKIE } from "@/lib/auth";

export function middleware(request: NextRequest) {
  const isPublic = request.nextUrl.pathname === "/login" || request.nextUrl.pathname.startsWith("/api/backend");
  const hasToken = Boolean(request.cookies.get(AUTH_COOKIE)?.value);

  // DIAG — temporary. What the gate saw on every request: cookie names only,
  // never values. Paired with the login.set-cookie line this shows whether the
  // browser returned what it was given.
  console.log(JSON.stringify({
    DIAG: "middleware",
    path: request.nextUrl.pathname,
    cookie_names: request.cookies.getAll().map((cookie) => cookie.name),
    host: request.headers.get("host"),
    x_forwarded_proto: request.headers.get("x-forwarded-proto"),
    hasToken,
    isPublic
  }));

  if (!isPublic && !hasToken) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"]
};
