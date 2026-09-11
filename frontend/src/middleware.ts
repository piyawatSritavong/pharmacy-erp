import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

import { AUTH_COOKIE } from "@/lib/auth";

export function middleware(request: NextRequest) {
  const isPublic = request.nextUrl.pathname === "/login" || request.nextUrl.pathname.startsWith("/api/backend");
  const hasToken = Boolean(request.cookies.get(AUTH_COOKIE)?.value);

  if (!isPublic && !hasToken) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"]
};
