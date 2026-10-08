/** @type {import('next').NextConfig} */
const nextConfig = {
  // Verification builds (npm run build:check) write to a separate dist dir so
  // they can never corrupt the .next directory of a running dev server.
  distDir: process.env.NEXT_DIST_DIR || ".next",
  // Standalone emits a self-contained server plus only the node_modules it
  // actually traced, so the runtime image carries that instead of the whole
  // dependency tree — dev dependencies, the Next build toolchain and all.
  output: "standalone",
  typedRoutes: false,
  // No need to announce the framework to every response.
  poweredByHeader: false,
  async headers() {
    // Baseline hardening for every page, the login form included. A full
    // Content-Security-Policy is deliberately left out for now: the theme
    // script and Next's inline runtime would need nonces, and a wrong policy
    // breaks the app outright.
    return [
      {
        source: "/:path*",
        headers: [
          // The login form must not be framed by another site (clickjacking).
          { key: "X-Frame-Options", value: "DENY" },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" },
          // Camera stays available to this origin for barcode scanning.
          { key: "Permissions-Policy", value: "camera=(self), microphone=(), geolocation=()" }
        ]
      }
    ];
  }
};

export default nextConfig;
