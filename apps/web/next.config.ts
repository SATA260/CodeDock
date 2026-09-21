import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  allowedDevOrigins: ["127.0.0.1", "localhost"],
  async redirects() {
    return [
      { source: "/codex", destination: "/", permanent: false },
      { source: "/codex/:path*", destination: "/", permanent: false },
    ];
  },
  transpilePackages: [
    "@codedock/core",
    "@codedock/ui",
    "@codedock/views",
    "@git-diff-view/react",
    "@git-diff-view/core",
    "@git-diff-view/utils",
    "streamdown",
    "@streamdown/cjk",
    "@streamdown/code",
    "@streamdown/math",
    "@streamdown/mermaid",
  ],
};

export default nextConfig;
