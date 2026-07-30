import { CtaGroup } from "@/components/CtaGroup";

const steps = [
  {
    n: "1",
    title: "Resolve provider",
    body: "From X-TokenGuard-Provider, path heuristics, or TOKENGUARD_DEFAULT_PROVIDER.",
  },
  {
    n: "2",
    title: "Authenticate key",
    body: "Require X-TokenGuard-API-Key. Plaintext tg_ only at provision; hashed at rest.",
  },
  {
    n: "3",
    title: "Analyze body",
    body: "Model, tiktoken input estimate, session id, semantic payload for the loop breaker.",
  },
  {
    n: "4",
    title: "Budget + loop in parallel",
    body: "Reserve micro-USD in Turso. Redis INCR on session + payload hash. Trip at threshold.",
  },
  {
    n: "5",
    title: "Fail closed or forward",
    body: "402 budget · 409 loop · 400 unpriced · 503 store down — or strip X-TokenGuard-* and proxy.",
  },
  {
    n: "6",
    title: "Settle usage",
    body: "Count output (incl. SSE). Settle actual cost, release unused reservation, log the ledger event.",
  },
];

export function RequestPath() {
  return (
    <div className="relative">
      <ol className="space-y-0">
        {steps.map((step, i) => (
          <li
            key={step.n}
            className="anim-step relative grid gap-4 border-l border-line py-6 pl-8 sm:grid-cols-[3rem_1fr] sm:gap-8"
            style={{ animationDelay: `${i * 0.08}s` }}
          >
            <span className="absolute -left-[5px] top-8 h-2.5 w-2.5 rounded-full bg-signal" />
            <span className="font-display text-2xl font-bold text-signal/50">
              {step.n}
            </span>
            <div>
              <h3 className="font-display text-lg font-semibold text-text">
                {step.title}
              </h3>
              <p className="mt-2 max-w-2xl text-sm leading-relaxed text-muted">
                {step.body}
              </p>
            </div>
          </li>
        ))}
      </ol>

      <div className="mt-12 border border-line bg-panel p-6 sm:p-8">
        <p className="text-[0.7rem] uppercase tracking-[0.16em] text-signal">
          Try it
        </p>
        <p className="font-display mt-2 text-xl font-semibold text-text">
          Clone, configure Turso + Upstash, provision a key.
        </p>
        <CtaGroup className="mt-6" />
      </div>
    </div>
  );
}
