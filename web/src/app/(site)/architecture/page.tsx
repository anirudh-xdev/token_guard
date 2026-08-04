import type { Metadata } from "next";
import Link from "next/link";
import { githubDoc } from "@/lib/site";

export const metadata: Metadata = {
  title: "Architecture",
  description:
    "Turso billing ledger, Upstash loop breaker, pricing catalog, and single-binary Go proxy layout.",
};

const stores = [
  {
    name: "Turso (libSQL)",
    role: "Billing ledger",
    owns: "users, api_keys, user_budgets, usage_events, model_prices",
  },
  {
    name: "Upstash Redis REST",
    role: "Loop state",
    owns: "Short-TTL counters keyed by session + payload hash",
  },
  {
    name: "Pricing catalog",
    role: "Cost estimate",
    owns: "Turso model_prices + OpenRouter sync; optional pricing.json bootstrap",
  },
];

export default function ArchitecturePage() {
  return (
    <div className="pt-14 sm:pt-16">
      <div className="border-b border-line">
        <div className="mx-auto max-w-6xl px-5 py-16 sm:px-8 sm:py-20">
          <p className="text-[0.7rem] uppercase tracking-[0.18em] text-signal">
            System shape
          </p>
          <h1 className="font-display mt-3 max-w-2xl text-4xl font-bold tracking-tight sm:text-5xl">
            Single binary. Three stores. Fail closed.
          </h1>
          <p className="mt-5 max-w-xl text-sm leading-relaxed text-muted sm:text-base">
            TokenGuard is a Go HTTP service. The entry point wires config,
            optional guard dependencies, and an http.ServeMux. Ops UI stays
            embedded at /dashboard — this page is the map.
          </p>
        </div>
      </div>

      <div className="mx-auto max-w-6xl px-5 py-16 sm:px-8">
        {/* Diagram-led flow */}
        <div
          className="overflow-x-auto border border-line bg-panel p-6 site-grid sm:p-10"
          style={{ boxShadow: "var(--shadow-md)" }}
        >
          <div className="flex min-w-[640px] items-center justify-between gap-3 text-center text-[0.65rem] uppercase tracking-[0.1em]">
            <Node label="Client app" />
            <Arrow />
            <Node label="TokenGuard" accent />
            <Arrow />
            <div className="flex flex-col gap-3">
              <Node label="LLM provider" />
              <Node label="402 / 409 / 400" tone="danger" />
            </div>
            <Arrow />
            <div className="flex flex-col gap-3">
              <Node label="Turso" tone="info" />
              <Node label="Redis" tone="info" />
            </div>
          </div>
          <p className="mt-8 text-center text-xs text-muted">
            Guard on → budget + loop preflight → upstream or block → settle usage
            into Turso
          </p>
        </div>

        <h2 className="font-display mt-16 text-2xl font-bold tracking-tight">
          Data stores
        </h2>
        <ul className="mt-8 divide-y divide-line border-y border-line">
          {stores.map((s) => (
            <li
              key={s.name}
              className="grid gap-2 py-6 sm:grid-cols-[14rem_10rem_1fr] sm:gap-8"
            >
              <span className="font-display font-semibold text-text">
                {s.name}
              </span>
              <span className="text-[0.7rem] uppercase tracking-[0.12em] text-signal">
                {s.role}
              </span>
              <span className="text-sm text-muted">{s.owns}</span>
            </li>
          ))}
        </ul>

        <h2 className="font-display mt-16 text-2xl font-bold tracking-tight">
          Packages
        </h2>
        <ul className="mt-6 space-y-3 text-sm text-text-dim">
          <li>
            <code className="text-signal">cmd/tokenguard</code> — bootstrap,
            routes, shutdown
          </li>
          <li>
            <code className="text-signal">internal/proxy</code> — reverse proxy,
            guard, providers, mgmt
          </li>
          <li>
            <code className="text-signal">internal/billing</code> — Turso store,
            budgets, keys, usage
          </li>
          <li>
            <code className="text-signal">internal/cache</code> — Upstash + loop
            circuit breaker
          </li>
          <li>
            <code className="text-signal">internal/models</code> — pricing engine
            + OpenRouter sync
          </li>
          <li>
            <code className="text-signal">web/</code> — product portal, operator
            console, and integrator docs (Next.js)
          </li>
        </ul>

        <p className="mt-12 text-sm text-muted">
          Full write-up:{" "}
          <a
            href={githubDoc("architecture")}
            className="text-signal underline-offset-4 hover:underline"
            rel="noopener noreferrer"
            target="_blank"
          >
            docs/ARCHITECTURE.md
          </a>
          {" · "}
          <Link href="/how-it-works" className="text-signal underline-offset-4 hover:underline">
            How it works
          </Link>
        </p>
      </div>
    </div>
  );
}

function Node({
  label,
  accent,
  tone,
}: {
  label: string;
  accent?: boolean;
  tone?: "danger" | "info";
}) {
  const cls = accent
    ? "border-signal/40 text-signal bg-signal-dim"
    : tone === "danger"
      ? "border-danger/30 text-danger bg-danger-dim"
      : tone === "info"
        ? "border-info/30 text-info bg-info-dim"
        : "border-line text-muted bg-ink";

  return (
    <div
      className={`min-w-[7rem] border px-3 py-3 ${cls}`}
      style={{ boxShadow: "var(--shadow-sm)" }}
    >
      <span className="font-display text-[0.7rem] font-semibold normal-case tracking-tight">
        {label}
      </span>
    </div>
  );
}

function Arrow() {
  return <span className="text-signal/60">→</span>;
}
