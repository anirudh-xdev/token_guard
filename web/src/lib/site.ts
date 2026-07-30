/** Public marketing CTAs — never put admin secrets or tg_ keys here. */

function trimSlash(url: string): string {
  return url.replace(/\/+$/, "");
}

export const siteConfig = {
  name: "TokenGuard",
  tagline: "Financial firewall for LLM APIs",
  description:
    "Sit TokenGuard between your app and providers. Estimate cost, reserve budget, trip agent loops, then forward — or block before money leaves.",
  githubUrl:
    process.env.NEXT_PUBLIC_GITHUB_URL?.trim() ||
    "https://github.com/anirudh-xdev/token_guard",
  /** Optional live proxy base (e.g. https://tokenguard.onrender.com). Empty = hide demo CTAs. */
  demoBaseUrl: process.env.NEXT_PUBLIC_DEMO_URL?.trim()
    ? trimSlash(process.env.NEXT_PUBLIC_DEMO_URL.trim())
    : "",
  docsRepoPaths: {
    howToUse: "/blob/main/HOW_TO_USE.md",
    api: "/blob/main/docs/API.md",
    architecture: "/blob/main/docs/ARCHITECTURE.md",
    design: "/blob/main/docs/DESIGN.md",
    deploy: "/blob/main/docs/DEPLOY.md",
    integration: "/blob/main/docs/INTEGRATION.md",
  },
} as const;

export function demoUrl(path = ""): string | null {
  if (!siteConfig.demoBaseUrl) return null;
  if (!path) return siteConfig.demoBaseUrl;
  return `${siteConfig.demoBaseUrl}${path.startsWith("/") ? path : `/${path}`}`;
}

export function githubDoc(path: keyof typeof siteConfig.docsRepoPaths): string {
  return `${siteConfig.githubUrl}${siteConfig.docsRepoPaths[path]}`;
}
