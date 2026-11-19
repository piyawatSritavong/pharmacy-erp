/**
 * One web app, two front doors: the till's domain serves only tills (the pos
 * portal), the back office's domain serves only back-office roles. Each door is
 * named by environment, so a local or preview deployment without them keeps
 * letting every role in.
 *
 *   POS_HOSTS=front.meapplication.co.th
 *   ADMIN_HOSTS=admin.meapplication.co.th
 */

export type HostPortal = "pos" | "backoffice";

function hosts(raw: string | undefined) {
  return (raw || "").split(",").map((host) => host.trim().toLowerCase()).filter(Boolean);
}

/** Which portal a request's host is for, or null when the host is unrestricted. */
export function portalForHost(rawHost: string | null | undefined): HostPortal | null {
  const host = (rawHost || "").split(":")[0].trim().toLowerCase();
  if (!host) return null;
  if (hosts(process.env.POS_HOSTS).includes(host)) return "pos";
  if (hosts(process.env.ADMIN_HOSTS).includes(host)) return "backoffice";
  return null;
}

/** The address the other kind of account should use instead. */
export function otherDoor(portal: HostPortal): string {
  const target = portal === "pos" ? hosts(process.env.ADMIN_HOSTS)[0] : hosts(process.env.POS_HOSTS)[0];
  return target ? `https://${target}` : "";
}

/** Whether a signed-in user's portal may use this host. */
export function portalAllowed(hostPortal: HostPortal | null, userPortal: unknown) {
  if (!hostPortal) return true;
  return hostPortal === "pos" ? userPortal === "pos" : userPortal !== "pos";
}

export function wrongDoorMessage(hostPortal: HostPortal) {
  const door = otherDoor(hostPortal);
  return hostPortal === "pos"
    ? `บัญชีนี้เป็นบัญชีหลังบ้าน กรุณาเข้าที่ ${door || "เว็บหลังบ้าน"}`
    : `บัญชีนี้เป็นบัญชีหน้าร้าน กรุณาเข้าที่ ${door || "เว็บหน้าร้าน"}`;
}
