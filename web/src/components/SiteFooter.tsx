import Link from "next/link";
import { siteConfig } from "@/lib/site";

export function SiteFooter() {
  return (
    <footer className="border-t border-line bg-panel">
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-5 py-10 sm:flex-row sm:items-end sm:justify-between sm:px-8">
        <div>
          <p className="font-display text-xl font-bold tracking-tight">
            Token<span className="text-signal">Guard</span>
          </p>
          <p className="mt-2 max-w-sm text-sm text-muted">
            Financial firewall for LLM APIs. Budget, loops, pricing — before
            upstream.
          </p>
        </div>
        <div className="flex flex-wrap gap-x-6 gap-y-2 text-[0.7rem] uppercase tracking-[0.12em] text-muted">
          <Link href="/how-it-works" className="hover:text-signal">
            How it works
          </Link>
          <Link href="/architecture" className="hover:text-signal">
            Architecture
          </Link>
          <Link href="/docs" className="hover:text-signal">
            Docs
          </Link>
          <a
            href={siteConfig.githubUrl}
            className="hover:text-signal"
            rel="noopener noreferrer"
            target="_blank"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  );
}
