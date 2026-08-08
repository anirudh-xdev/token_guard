import Link from "next/link";
import { demoUrl, siteConfig } from "@/lib/site";

const nav = [
  { href: "/#problems", label: "Problems", priority: true },
  { href: "/how-it-works", label: "How it works", priority: false },
  { href: "/architecture", label: "Architecture", priority: false },
  { href: "/docs", label: "Docs", priority: true },
];

export function SiteHeader() {
  const liveDocs = demoUrl("/docs");

  return (
    <header className="absolute inset-x-0 top-0 z-30 border-b border-line/70 bg-[var(--header-bg)] backdrop-blur-xl">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-5 sm:h-16 sm:px-8">
        <Link
          href="/"
          className="font-display text-lg font-bold tracking-tight text-text sm:text-xl"
        >
          Token<span className="text-signal">Guard</span>
        </Link>

        <nav
          aria-label="Site navigation"
          className="hidden items-center gap-6 text-[0.7rem] uppercase tracking-[0.14em] text-muted md:flex"
        >
          {nav.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="min-h-11 content-center transition-colors hover:text-signal"
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-2 text-[0.7rem] uppercase tracking-[0.12em]">
          <details className="group relative md:hidden">
            <summary className="flex min-h-11 cursor-pointer list-none items-center rounded-md border border-line bg-panel px-3 font-semibold text-text">
              Menu
            </summary>
            <nav
              aria-label="Mobile site navigation"
              className="absolute right-0 top-12 z-50 grid min-w-52 rounded-lg border border-line bg-panel p-2 text-sm normal-case tracking-normal shadow-lg"
            >
              {nav.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className="min-h-11 rounded-md px-3 py-3 text-text-dim hover:bg-ink-2 hover:text-text"
                >
                  {item.label}
                </Link>
              ))}
              <a
                href={siteConfig.githubUrl}
                className="min-h-11 rounded-md px-3 py-3 text-text-dim hover:bg-ink-2 hover:text-text"
                rel="noopener noreferrer"
                target="_blank"
              >
                GitHub
              </a>
            </nav>
          </details>
          {liveDocs ? (
            <a
              href={liveDocs}
              className="hidden text-muted transition-colors hover:text-text sm:inline"
              rel="noopener noreferrer"
              target="_blank"
            >
              Live docs
            </a>
          ) : null}
          <a
            href={siteConfig.githubUrl}
            className="btn-ghost hidden min-h-11 items-center px-3 py-1.5 text-text sm:inline-flex"
            rel="noopener noreferrer"
            target="_blank"
          >
            GitHub
          </a>
          <Link
            href="/portal"
            className="inline-flex min-h-11 items-center rounded-md bg-signal px-3 py-1.5 font-semibold text-on-signal"
          >
            Portal
          </Link>
        </div>
      </div>
    </header>
  );
}
