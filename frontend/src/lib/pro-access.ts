// Temporary business decision, 2026-10-09: the Pro gate is off in general.
// Role permissions still apply.
export const PRO_GATE_ENABLED = false;

// Except these, which stay shown as Pro (2026-10-18): the government (รพ.สต.)
// and FDA (อย.) documents wait on requirements the owner does not have yet.
export const PRO_GATED_PATHS = ["/government-sales", "/fda-reports"];

export function isProGated(href?: string) {
  return PRO_GATE_ENABLED || (href ? PRO_GATED_PATHS.includes(href) : false);
}
