/** Dominant hero visual: request path through the firewall — not abstract blobs. */
export function HeroVisual() {
  return (
    <div
      aria-hidden
      className="anim-fade-in relative h-full min-h-[280px] w-full overflow-hidden border-l border-line bg-panel site-grid sm:min-h-0"
      style={{ boxShadow: "inset 1px 0 0 var(--line), var(--shadow-lg)" }}
    >
      <div className="absolute inset-0 bg-gradient-to-br from-signal-dim via-transparent to-ink-2/80" />
      <div className="pointer-events-none absolute -right-16 top-10 h-56 w-56 rounded-full bg-signal/10 blur-3xl" />

      <div className="relative flex h-full flex-col justify-center gap-0 p-6 sm:p-8 lg:p-10">
        <FlowRow delay="0s" label="App" detail="POST /v1/chat/completions" tone="muted" />
        <Connector />
        <FlowRow
          delay="0.15s"
          label="TokenGuard"
          detail="reserve · loop · catalog"
          tone="signal"
          pulse
        />
        <Connector branched />
        <div className="grid gap-3 sm:grid-cols-2">
          <FlowRow
            delay="0.3s"
            label="Block"
            detail="402 · 409 · 400 · 503"
            tone="danger"
            compact
          />
          <FlowRow
            delay="0.38s"
            label="Upstream"
            detail="strip X-TokenGuard-* → settle"
            tone="info"
            compact
          />
        </div>

        <pre
          className="anim-fade-up-delay-3 mt-8 overflow-x-auto border border-line bg-ink-2 p-4 text-[0.65rem] leading-relaxed text-text-dim sm:text-[0.7rem]"
          style={{ boxShadow: "var(--shadow-sm)" }}
        >
          <code>{`{
  "error": "TokenGuard: budget exceeded",
  "available_microusd": 1200,
  "estimated_cost_microusd": 5000,
  "model": "gpt-4o-mini"
}`}</code>
        </pre>
      </div>
    </div>
  );
}

function Connector({ branched = false }: { branched?: boolean }) {
  return (
    <div
      className={`anim-pulse ml-4 h-6 w-px bg-signal/45 ${branched ? "mb-1" : ""}`}
    />
  );
}

function FlowRow({
  label,
  detail,
  tone,
  delay,
  pulse,
  compact,
}: {
  label: string;
  detail: string;
  tone: "muted" | "signal" | "danger" | "info";
  delay: string;
  pulse?: boolean;
  compact?: boolean;
}) {
  const toneClass =
    tone === "signal"
      ? "border-signal/35 text-signal bg-signal-dim"
      : tone === "danger"
        ? "border-danger/30 text-danger bg-danger-dim"
        : tone === "info"
          ? "border-info/30 text-info bg-info-dim"
          : "border-line text-muted bg-panel";

  return (
    <div
      className="anim-step flex items-start gap-3"
      style={{ animationDelay: delay }}
    >
      <span
        className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${
          pulse
            ? "bg-signal shadow-[0_0_10px_rgba(6,122,82,0.45)]"
            : "bg-current opacity-50"
        } ${toneClass}`}
      />
      <div
        className={`flex-1 border ${toneClass} ${compact ? "px-3 py-2" : "px-4 py-3"}`}
        style={{ boxShadow: "var(--shadow-sm)" }}
      >
        <p className="font-display text-sm font-semibold tracking-tight text-text sm:text-base">
          {label}
        </p>
        <p className="mt-0.5 text-[0.65rem] uppercase tracking-[0.1em] opacity-90">
          {detail}
        </p>
      </div>
    </div>
  );
}
