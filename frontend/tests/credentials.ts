/**
 * Seed credentials for the local stack.
 *
 * The seed no longer has a password of its own: SEED_ADMIN_PASSWORD and
 * SEED_POS_PASSWORD are required, must be at least sixteen characters, and must
 * differ from each other. These are the values deploy/docker-compose.yml sets
 * for development, and they exist nowhere else — production sets its own in the
 * Render dashboard.
 *
 * Two passwords, not one: head office and the tills held the same credential
 * before, so a till account was a head-office account.
 */
export const ADMIN_PASSWORD = "LocalDevAdmin-2026!";
export const POS_PASSWORD = "LocalDevTill-2026!";

/** The tills are the accounts under pos.*; everything else is head office. */
export function passwordFor(email: string): string {
  return email.startsWith("pos.") ? POS_PASSWORD : ADMIN_PASSWORD;
}
