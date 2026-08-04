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

        <nav className="flex items-center gap-4 text-[0.65rem] uppercase tracking-[0.12em] text-muted sm:gap-6 sm:text-[0.7rem] sm:tracking-[0.14em]">
          {nav.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={
                item.priority
                  ? "transition-colors hover:text-signal"
                  : "hidden transition-colors hover:text-signal md:inline"
              }
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="flex items-center gap-3 text-[0.7rem] uppercase tracking-[0.12em]">
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
            className="btn-ghost px-3 py-1.5 text-text"
            rel="noopener noreferrer"
            target="_blank"
          >
            GitHub
          </a>
          <Link
            href="/portal"
            className="rounded-md bg-signal px-3 py-1.5 font-semibold text-on-signal"
          >
            Portal
          </Link>
        </div>
      </div>
    </header>
  );
}
