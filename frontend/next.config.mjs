/** @type {import('next').NextConfig} */
const nextConfig = {
  // Verification builds (npm run build:check) write to a separate dist dir so
  // they can never corrupt the .next directory of a running dev server.
  distDir: process.env.NEXT_DIST_DIR || ".next",
  // Standalone emits a self-contained server plus only the node_modules it
  // actually traced, so the runtime image carries that instead of the whole
  // dependency tree — dev dependencies, the Next build toolchain and all.
  output: "standalone",
  typedRoutes: false
};

export default nextConfig;
