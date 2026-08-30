/** @type {import('next').NextConfig} */
const nextConfig = {
  // Verification builds (npm run build:check) write to a separate dist dir so
  // they can never corrupt the .next directory of a running dev server.
  distDir: process.env.NEXT_DIST_DIR || ".next",
  typedRoutes: false
};

export default nextConfig;
