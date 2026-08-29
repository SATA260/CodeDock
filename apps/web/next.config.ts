import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  transpilePackages: [
    "@codedock/core",
    "@codedock/ui",
    "@codedock/views",
    "streamdown",
    "@streamdown/cjk",
    "@streamdown/code",
    "@streamdown/math",
    "@streamdown/mermaid",
  ],
};

export default nextConfig;
