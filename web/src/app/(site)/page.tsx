import { CtaGroup } from "@/components/CtaGroup";
import { HeroVisual } from "@/components/HeroVisual";
import { ProblemsSection } from "@/components/ProblemsSection";
import { Button } from "@/components/ui/button";
import Link from "next/link";

export default function HomePage() {
  return (
    <>
      <section className="relative flex min-h-[100svh] flex-col pt-14 sm:pt-16">
        <div className="grid flex-1 lg:grid-cols-2">
          <div className="flex flex-col justify-center px-5 py-12 sm:px-8 sm:py-16 lg:py-20">
            <p className="anim-fade-up text-[0.7rem] uppercase tracking-[0.2em] text-signal">
              Go reverse proxy · financial firewall
            </p>
            <h1 className="anim-fade-up-delay-1 font-display mt-4 text-[clamp(2.75rem,8vw,5rem)] font-extrabold leading-[0.95] tracking-tight text-text">
              Token
              <span className="text-signal">Guard</span>
            </h1>
            <p className="anim-fade-up-delay-2 mt-5 max-w-md text-sm leading-relaxed text-text-dim sm:text-base">
              Sit between your app and LLM providers. Reserve budget, trip agent
              loops, block unpriced models — then forward. Or return before money
              leaves.
            </p>
            <CtaGroup className="anim-fade-up-delay-3 mt-8" />
          </div>

          <div className="min-h-[320px] lg:min-h-0">
            <HeroVisual />
          </div>
        </div>
      </section>

      <ProblemsSection />

      <section className="border-t border-border bg-ink-2">
        <div className="mx-auto grid max-w-6xl gap-10 px-5 py-20 sm:px-8 lg:grid-cols-2 lg:gap-16">
          <div>
            <p className="text-[0.7rem] uppercase tracking-[0.18em] text-signal">
              Under the hood
            </p>
            <h2 className="font-display mt-3 text-3xl font-bold tracking-tight">
              One request path. Fail closed.
            </h2>
            <p className="mt-4 text-sm leading-relaxed text-muted-foreground">
              Budget reserve in Turso, loop counters in Upstash Redis, live
              pricing from OpenRouter sync into the catalog. Management dashboard
              stays on the Go binary — this site tells the story.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <Button size="lg" className="uppercase tracking-[0.12em] text-white!" asChild>
                <Link href="/how-it-works">How it works</Link>
              </Button>
              <Button
                size="lg"
                variant="outline"
                className="uppercase tracking-[0.12em]"
                asChild
              >
                <Link href="/architecture">Architecture</Link>
              </Button>
            </div>
          </div>
          <ul className="space-y-4 text-sm text-text-dim">
            {[
              ["Micro-USD integers", "no float ledger math."],
              ["Strip X-TokenGuard-*", "before upstream; provider auth passes through."],
              ["Never invent prices", "unknown models get 400, not a guess."],
              ["503 when stores are down", "in guarded mode — never soft-fail open."],
            ].map(([title, rest]) => (
              <li
                key={title}
                className="border-l-[3px] border-signal bg-card py-3 pl-4 pr-3 ring-1 ring-foreground/5"
              >
                <span className="font-medium text-text">{title}</span> — {rest}
              </li>
            ))}
          </ul>
        </div>
      </section>

      <section className="relative overflow-hidden border-t border-border">
        <div className="pointer-events-none absolute inset-0 bg-gradient-to-b from-signal-dim/80 via-transparent to-transparent" />
        <div className="relative mx-auto max-w-6xl px-5 py-20 text-center sm:px-8 sm:py-24">
          <h2 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">
            Put a firewall in front of your tokens.
          </h2>
          <p className="mx-auto mt-4 max-w-lg text-sm text-muted-foreground">
            Open source Go proxy. Docs and dashboard ship with the binary;
            operators provision users and sync pricing when guard is enabled.
          </p>
          <CtaGroup className="mt-8 justify-center" primaryLabel="Star on GitHub" />
        </div>
      </section>
    </>
  );
}
