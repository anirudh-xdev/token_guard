import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import Link from "next/link";
import { siteConfig } from "@/lib/site";

export function SiteFooter() {
  return (
    <footer className="border-t border-border bg-card">
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-5 py-10 sm:flex-row sm:items-end sm:justify-between sm:px-8">
        <div>
          <p className="font-display text-xl font-bold tracking-tight">
            Token<span className="text-signal">Guard</span>
          </p>
          <p className="mt-2 max-w-sm text-sm text-muted-foreground">
            Financial firewall for LLM APIs. Budget, loops, pricing — before
            upstream.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1 text-[0.7rem] uppercase tracking-[0.12em]">
          <Button variant="link" className="h-auto px-2 text-muted-foreground" asChild>
            <Link href="/how-it-works">How it works</Link>
          </Button>
          <Separator orientation="vertical" className="hidden h-4 sm:block" />
          <Button variant="link" className="h-auto px-2 text-muted-foreground" asChild>
            <Link href="/architecture">Architecture</Link>
          </Button>
          <Separator orientation="vertical" className="hidden h-4 sm:block" />
          <Button variant="link" className="h-auto px-2 text-muted-foreground" asChild>
            <Link href="/docs">Docs</Link>
          </Button>
          <Separator orientation="vertical" className="hidden h-4 sm:block" />
          <Button variant="link" className="h-auto px-2 text-muted-foreground" asChild>
            <a
              href={siteConfig.githubUrl}
              rel="noopener noreferrer"
              target="_blank"
            >
              GitHub
            </a>
          </Button>
        </div>
      </div>
    </footer>
  );
}
