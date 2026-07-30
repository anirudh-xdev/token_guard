import { CtaGroup } from "@/components/CtaGroup";
import { HeroVisual } from "@/components/HeroVisual";
import { ProblemsSection } from "@/components/ProblemsSection";
import Link from "next/link";

export default function HomePage() {
  return (
    <>
      {/* Full-bleed hero: header overlays; content fills first viewport */}
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

      <section className="border-t border-line bg-ink-2">
        <div className="mx-auto grid max-w-6xl gap-10 px-5 py-20 sm:px-8 lg:grid-cols-2 lg:gap-16">
          <div>
            <p className="text-[0.7rem] uppercase tracking-[0.18em] text-signal">
              Under the hood
            </p>
            <h2 className="font-display mt-3 text-3xl font-bold tracking-tight">
              One request path. Fail closed.
            </h2>
            <p className="mt-4 text-sm leading-relaxed text-muted">
              Budget reserve in Turso, loop counters in Upstash Redis, live
              pricing from OpenRouter sync into the catalog. Management dashboard
              stays on the Go binary — this site tells the story.
            </p>
            <div className="mt-8 flex flex-wrap gap-4 text-[0.7rem] uppercase tracking-[0.12em]">
              <Link
                href="/how-it-works"
                className="border border-signal bg-signal/10 px-4 py-2 text-signal transition hover:bg-signal hover:text-on-signal"
              >
                How it works
              </Link>
              <Link
                href="/architecture"
                className="border border-line px-4 py-2 text-muted transition hover:border-signal hover:text-signal"
              >
                Architecture
              </Link>
            </div>
          </div>
          <ul className="space-y-5 text-sm text-text-dim">
            <li className="border-l-2 border-signal pl-4">
              <span className="text-text">Micro-USD integers</span> — no float
              ledger math.
            </li>
            <li className="border-l-2 border-signal pl-4">
              <span className="text-text">Strip X-TokenGuard-*</span> before
              upstream; provider auth passes through.
            </li>
            <li className="border-l-2 border-signal pl-4">
              <span className="text-text">Never invent prices</span> — unknown
              models get 400, not a guess.
            </li>
            <li className="border-l-2 border-signal pl-4">
              <span className="text-text">503 when stores are down</span> in
              guarded mode — never soft-fail open.
            </li>
          </ul>
        </div>
      </section>

      <section className="border-t border-line">
        <div className="mx-auto max-w-6xl px-5 py-20 text-center sm:px-8 sm:py-24">
          <h2 className="font-display text-3xl font-bold tracking-tight sm:text-4xl">
            Put a firewall in front of your tokens.
          </h2>
          <p className="mx-auto mt-4 max-w-lg text-sm text-muted">
            Open source Go proxy. Docs and dashboard ship with the binary;
            operators provision users and sync pricing when guard is enabled.
          </p>
          <CtaGroup className="mt-8 justify-center" primaryLabel="Star on GitHub" />
        </div>
      </section>
    </>
  );
}
