import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
import { AUTH_COOKIE } from "@/lib/auth";

export const dynamic = "force-dynamic";

async function proxy(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const base = process.env.BACKEND_V2_INTERNAL_URL?.trim().replace(/\/$/, "");
  if (!base) return NextResponse.json({ message: "ยังไม่ได้ตั้งค่าการเชื่อมต่อระบบ V2" }, { status: 503 });
  const token = (await cookies()).get(AUTH_COOKIE)?.value;
  if (!token) return NextResponse.json({ message: "กรุณาเข้าสู่ระบบ" }, { status: 401 });
  const { path } = await context.params;
  if (path.some((part) => !/^[a-zA-Z0-9_-]+$/.test(part))) {
    return NextResponse.json({ message: "เส้นทางไม่ถูกต้อง" }, { status: 400 });
  }
  try {
    const response = await fetch(`${base}/api/v2/${path.join("/")}${request.nextUrl.search}`, {
      method: request.method,
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
        ...(request.headers.get("idempotency-key") ? { "Idempotency-Key": request.headers.get("idempotency-key")! } : {})
      },
      body: request.method === "GET" ? undefined : await request.arrayBuffer(),
      cache: "no-store",
      signal: AbortSignal.timeout(30_000)
    });
    return new NextResponse(await response.arrayBuffer(), {
      status: response.status,
      headers: { "Content-Type": "application/json", "Cache-Control": "no-store" }
    });
  } catch {
    return NextResponse.json({ message: "ระบบ V2 ไม่พร้อมเชื่อมต่อ กรุณาลองอีกครั้ง" }, { status: 503 });
  }
}

export const GET = proxy;
export const POST = proxy;
