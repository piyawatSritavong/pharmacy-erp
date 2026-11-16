import { timingSafeEqual } from "node:crypto";
import { NextRequest, NextResponse } from "next/server";

import { alertRecipients, readUptimeEvent, uptimeMessage } from "@/lib/uptime-alert";

export const dynamic = "force-dynamic";

function sameSecret(given: string, expected: string) {
  const a = Buffer.from(given);
  const b = Buffer.from(expected);
  return a.length === b.length && timingSafeEqual(a, b);
}

async function readParams(request: NextRequest) {
  const params: Record<string, string> = {};
  request.nextUrl.searchParams.forEach((value, key) => {
    params[key] = value;
  });
  if (request.method !== "POST") return params;
  const type = request.headers.get("content-type") || "";
  try {
    if (type.includes("application/json")) {
      const body = (await request.json()) as Record<string, unknown>;
      for (const [key, value] of Object.entries(body)) params[key] = String(value ?? "");
    } else if (type.includes("form")) {
      const form = await request.formData();
      form.forEach((value, key) => {
        params[key] = String(value);
      });
    }
  } catch {
    // A body we cannot read leaves the query string to speak for itself.
  }
  return params;
}

/**
 * UptimeRobot's webhook alert contact calls this with ?token=… and its
 * alert variables. The token is the only thing standing between the public
 * internet and the shop's LINE quota, so a wrong or missing one is refused
 * before anything is sent.
 */
async function handle(request: NextRequest) {
  const expected = process.env.UPTIME_WEBHOOK_TOKEN || "";
  const lineToken = process.env.LINE_CHANNEL_ACCESS_TOKEN || "";
  const recipients = alertRecipients(process.env.LINE_ALERT_TO);
  if (!expected || !lineToken || recipients.length === 0) {
    return NextResponse.json({ message: "ยังไม่ได้ตั้งค่าการแจ้งเตือน LINE" }, { status: 503 });
  }
  const given = request.nextUrl.searchParams.get("token") || "";
  if (!sameSecret(given, expected)) {
    return NextResponse.json({ message: "ไม่มีสิทธิ์" }, { status: 401 });
  }
  const event = readUptimeEvent(await readParams(request));
  if (!event) {
    return NextResponse.json({ message: "ไม่ใช่การแจ้งเตือนระบบล่ม" }, { status: 200 });
  }
  const text = uptimeMessage(event);
  const base = process.env.LINE_API_BASE || "https://api.line.me";
  let sent = 0;
  for (const to of recipients) {
    const response = await fetch(`${base}/v2/bot/message/push`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${lineToken}` },
      body: JSON.stringify({ to, messages: [{ type: "text", text }] }),
      cache: "no-store",
    }).catch(() => null);
    if (response?.ok) sent += 1;
  }
  return NextResponse.json({ sent, recipients: recipients.length }, { status: sent > 0 ? 200 : 502 });
}

export const GET = handle;
export const POST = handle;
