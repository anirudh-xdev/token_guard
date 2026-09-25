import type { NextConfig } from "next";
import path from "node:path";

const nextConfig: NextConfig = {
  // A lockfile on Desktop makes Turbopack treat that folder as [project],
  // which breaks the React Client Manifest (builtin global-error.js).
  turbopack: {
    root: path.resolve(__dirname),
  },
};

export default nextConfig;
