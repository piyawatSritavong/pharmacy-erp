/**
 * Downtime alerts: UptimeRobot watches the API and calls our webhook; this
 * turns its call into a LINE message for the support team.
 *
 * The relay lives in the web service, not the API, so it still runs while the
 * API is the thing that is down. If the web service is down too, UptimeRobot's
 * own email alert is the fallback.
 */

export type UptimeEvent = {
  monitor: string;
  url: string;
  down: boolean;
  details: string;
  durationSeconds: number;
};

/** UptimeRobot's webhook variables, from the query string or the body. */
export function readUptimeEvent(params: Record<string, string>): UptimeEvent | null {
  const type = params.alertType || params.alert_type || "";
  // 1 = down, 2 = up. Anything else (SSL expiry and the like) is not a
  // downtime alert.
  if (type !== "1" && type !== "2") return null;
  return {
    monitor: params.monitorFriendlyName || params.monitor_friendly_name || "ระบบ",
    url: params.monitorURL || params.monitor_url || "",
    down: type === "1",
    details: params.alertDetails || params.alert_details || "",
    durationSeconds: Number(params.alertDuration || params.alert_duration || 0) || 0,
  };
}

function bangkokTime(at: Date) {
  return at.toLocaleString("th-TH", { timeZone: "Asia/Bangkok", dateStyle: "medium", timeStyle: "short" });
}

function minutes(seconds: number) {
  if (seconds < 60) return `${seconds} วินาที`;
  const total = Math.round(seconds / 60);
  return total < 60 ? `${total} นาที` : `${Math.floor(total / 60)} ชม. ${total % 60} นาที`;
}

export function uptimeMessage(event: UptimeEvent, at: Date = new Date()): string {
  if (event.down) {
    return [
      `🔴 ${event.monitor} ใช้งานไม่ได้`,
      `เวลา ${bangkokTime(at)}`,
      event.details ? `สาเหตุ: ${event.details}` : "",
      event.url,
      "หน้าร้านใช้ขั้นตอนขายสำรองไปก่อน",
    ].filter(Boolean).join("\n");
  }
  return [
    `🟢 ${event.monitor} กลับมาใช้งานได้แล้ว`,
    `เวลา ${bangkokTime(at)}`,
    event.durationSeconds ? `ล่มไป ${minutes(event.durationSeconds)}` : "",
  ].filter(Boolean).join("\n");
}

/** Each LINE user or group id in LINE_ALERT_TO (comma separated). */
export function alertRecipients(raw: string | undefined): string[] {
  return (raw || "").split(",").map((id) => id.trim()).filter(Boolean);
}
