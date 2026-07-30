import type { Metadata } from "next";
import { CtaGroup } from "@/components/CtaGroup";
import { demoUrl, githubDoc, siteConfig } from "@/lib/site";

export const metadata: Metadata = {
  title: "Docs",
  description:
    "Pointers into TokenGuard’s real documentation — setup, API, deploy, integration.",
};

const links = [
  {
    title: "How to use",
    body: "Env, providers, budgets, provisioning, dashboard.",
    href: githubDoc("howToUse"),
  },
  {
    title: "HTTP API",
    body: "Routes, headers, status codes (401 / 402 / 409 / 400 / 503).",
    href: githubDoc("api"),
  },
  {
    title: "Architecture",
    body: "Request flow, packages, Turso / Redis / pricing.",
    href: githubDoc("architecture"),
  },
  {
    title: "Design invariants",
    body: "Why no price guessing, micro-USD, fail closed.",
    href: githubDoc("design"),
  },
  {
    title: "Deploy",
    body: "Render Docker, health checks, free-tier cold starts.",
    href: githubDoc("deploy"),
  },
  {
    title: "Integration",
    body: "Point OpenAI / Anthropic SDKs at TokenGuard.",
    href: githubDoc("integration"),
  },
];

export default function DocsPage() {
  const liveDocs = demoUrl("/docs");

  return (
    <div className="pt-14 sm:pt-16">
      <div className="border-b border-line">
        <div className="mx-auto max-w-6xl px-5 py-16 sm:px-8 sm:py-20">
          <p className="text-[0.7rem] uppercase tracking-[0.18em] text-signal">
            Documentation
          </p>
          <h1 className="font-display mt-3 max-w-2xl text-4xl font-bold tracking-tight sm:text-5xl">
            Source of truth lives in the repo.
          </h1>
          <p className="mt-5 max-w-xl text-sm leading-relaxed text-muted sm:text-base">
            This showcase does not duplicate the API reference. Use the links
            below — or the public /docs page on a live TokenGuard instance.
          </p>
          <CtaGroup className="mt-8" primaryLabel="Open repository" />
        </div>
      </div>

      <div className="mx-auto max-w-6xl px-5 py-16 sm:px-8">
        {liveDocs ? (
          <a
            href={liveDocs}
            className="mb-10 block border border-signal/40 bg-signal/5 px-5 py-4 text-sm text-signal transition hover:bg-signal/10"
            rel="noopener noreferrer"
            target="_blank"
          >
            Live instance docs → {liveDocs}
          </a>
        ) : null}

        <ul className="divide-y divide-line border-y border-line">
          {links.map((link) => (
            <li key={link.title}>
              <a
                href={link.href}
                className="group flex flex-col gap-1 py-6 transition sm:flex-row sm:items-baseline sm:justify-between sm:gap-8"
                rel="noopener noreferrer"
                target="_blank"
              >
                <span className="font-display text-lg font-semibold text-text group-hover:text-signal">
                  {link.title}
                </span>
                <span className="max-w-md text-sm text-muted sm:text-right">
                  {link.body}
                </span>
              </a>
            </li>
          ))}
        </ul>

        <p className="mt-10 text-xs text-muted">
          Repo:{" "}
          <a
            href={siteConfig.githubUrl}
            className="text-signal"
            rel="noopener noreferrer"
            target="_blank"
          >
            {siteConfig.githubUrl}
          </a>
        </p>
      </div>
    </div>
  );
}
