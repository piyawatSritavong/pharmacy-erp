/**
 * Where the browser-facing server talks to the Go API.
 *
 * The browser never calls the API directly: client code fetches /api/backend/*
 * on its own origin and the route handler forwards it. So this value is only
 * ever read on the server, and NEXT_PUBLIC_API_URL — despite the prefix — is
 * read at runtime here rather than inlined into a client bundle at build time.
 *
 * Three names are accepted because three already exist in the wild:
 * BACKEND_INTERNAL_URL is what compose sets, NEXT_PUBLIC_BACKEND_URL is what
 * the older deployment notes use, and NEXT_PUBLIC_API_URL is what the Render
 * blueprint sets.
 */
const configured =
  process.env.BACKEND_INTERNAL_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  process.env.NEXT_PUBLIC_BACKEND_URL;

/**
 * Falling back to localhost in production is how a deployment comes up green
 * and then fails on every page: the container is healthy, the port is open, and
 * every request goes to a backend that is not there. Somewhere that is not
 * development, a missing value is an error at startup instead.
 */
function resolve(): string {
  if (configured) {
    return configured.replace(/\/+$/, "");
  }
  if (process.env.NODE_ENV === "production") {
    throw new Error(
      "ไม่ได้ตั้งค่า URL ของ backend — ต้องตั้ง NEXT_PUBLIC_API_URL (หรือ BACKEND_INTERNAL_URL) ก่อนรันในโหมด production"
    );
  }
  return "http://localhost:8080";
}

export const backendURL = resolve();
