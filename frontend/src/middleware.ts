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
  // Static public files a browser fetches before (or without) signing in —
  // the app icons, the web manifest and robots.txt — are left alone; they
  // used to be redirected to the login page like any other path.
  matcher: ["/((?!_next/static|_next/image|favicon.ico|icon.svg|icon-512.png|apple-touch-icon.png|manifest.webmanifest|robots.txt).*)"]
};
