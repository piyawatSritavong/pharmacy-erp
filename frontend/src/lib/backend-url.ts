/**
 * Where the browser-facing server talks to the Go API.
 *
 * The browser never calls the API directly: client code fetches /api/backend/*
 * on its own origin and the route handler forwards it. So this value is only
 * ever read on the server, and NEXT_PUBLIC_API_URL — despite the prefix — is
 * read at runtime here rather than inlined into a client bundle at build time.
 *
 * Three names are accepted because three already exist in the wild:
 * BACKEND_INTERNAL_URL is what compose and the Render blueprint set,
 * NEXT_PUBLIC_BACKEND_URL is what the older deployment notes use, and
 * NEXT_PUBLIC_API_URL is accepted so a value set under that name is not
 * silently ignored.
 *
 * Nothing here runs at import. It used to: the resolved value was a module
 * constant, so the production check fired the moment the module was loaded —
 * and `next build` loads every route module to collect its page data, with
 * NODE_ENV already set to production and no runtime environment injected yet.
 * The check meant to protect a misconfigured deployment was instead failing the
 * build that produces the image. The check is unchanged; only its timing is.
 */

export class BackendURLNotConfiguredError extends Error {
  constructor() {
    super(
      "ไม่ได้ตั้งค่า URL ของ backend — ต้องตั้ง BACKEND_INTERNAL_URL (หรือ NEXT_PUBLIC_API_URL / NEXT_PUBLIC_BACKEND_URL) ก่อนรันในโหมด production"
    );
    this.name = "BackendURLNotConfiguredError";
  }
}

/**
 * Resolves the API base URL for one request.
 *
 * Falling back to localhost in production is how a deployment comes up green
 * and then fails on every page: the container is healthy, the port is open, and
 * every request goes to a backend that is not there. So outside development a
 * missing value throws — on the request that needs it, where the message
 * reaches a log with a URL beside it, rather than at import where it takes the
 * build down instead.
 */
export function resolveBackendURL(): string {
  const configured =
    process.env.BACKEND_INTERNAL_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    process.env.NEXT_PUBLIC_BACKEND_URL;

  if (configured && configured.trim()) {
    return configured.trim().replace(/\/+$/, "");
  }
  if (process.env.NODE_ENV === "production") {
    throw new BackendURLNotConfiguredError();
  }
  return "http://localhost:8080";
}
