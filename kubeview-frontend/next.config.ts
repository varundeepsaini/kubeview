import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Emit a self-contained server under .next/standalone so the Docker image
  // only needs that directory plus the static assets.
  output: "standalone",
};

export default nextConfig;
