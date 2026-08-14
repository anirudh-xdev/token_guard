import { problems } from "@/lib/problems";

export function ProblemsSection() {
  return (
    <section id="problems" className="border-t border-border bg-card">
      <div className="mx-auto max-w-6xl px-5 py-20 sm:px-8 sm:py-28">
        <div className="max-w-2xl">
          <p className="text-[0.7rem] uppercase tracking-[0.18em] text-signal">
            What it catches
          </p>
          <h2 className="font-display mt-3 text-3xl font-bold tracking-tight text-text sm:text-4xl">
            Four ways LLM spend goes blind — and how TokenGuard stops them.
          </h2>
          <p className="mt-4 text-sm leading-relaxed text-muted-foreground sm:text-base">
            One proxy. Fail closed. Money as micro-USD integers. No guessing
            model prices.
          </p>
        </div>

        <ol className="mt-16 space-y-0 divide-y divide-line border-y border-line">
          {problems.map((p) => (
            <li
              key={p.id}
              id={p.id}
              className="grid gap-6 py-10 sm:grid-cols-[5rem_1fr] lg:grid-cols-[5rem_1fr_12rem] lg:gap-10"
            >
              <span className="font-display text-3xl font-bold text-signal/40">
                {p.number}
              </span>
              <div>
                <h3 className="font-display text-xl font-semibold tracking-tight text-text sm:text-2xl">
                  {p.title}
                </h3>
                <p className="mt-3 max-w-xl text-sm leading-relaxed text-text-dim">
                  {p.problem}
                </p>
                <p className="mt-2 max-w-xl text-xs text-muted-foreground">
                  <span className="font-medium text-signal">When:</span> {p.when}
                </p>
                <p className="mt-4 max-w-xl text-sm leading-relaxed text-text">
                  <span className="font-medium text-signal">How:</span>{" "}
                  {p.solution}
                </p>
              </div>
              <div className="flex items-start lg:justify-end">
                <StatusBadge code={p.status} label={p.statusLabel} />
              </div>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}

function StatusBadge({ code, label }: { code: string; label: string }) {
  const color =
    code === "402"
      ? "text-warn border-warn/25 bg-warn-dim"
      : code === "409"
        ? "text-danger border-danger/25 bg-danger-dim"
        : code === "400"
          ? "text-info border-info/25 bg-info-dim"
          : "text-signal border-signal/25 bg-signal-dim";

  return (
    <div
      className={`inline-flex flex-col border px-4 py-3 font-mono text-xs uppercase tracking-[0.14em] ${color}`}
      style={{ boxShadow: "var(--shadow-sm)" }}
    >
      <span className="text-lg font-semibold tracking-normal normal-case">
        {code}
      </span>
      <span className="mt-1 opacity-80">{label}</span>
    </div>
  );
}
