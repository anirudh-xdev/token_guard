"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { apiBaseUrl } from "@/lib/tokenguard-api";
import { githubDoc } from "@/lib/site";

export function IntegratorDocs() {
  const apiBase = apiBaseUrl();
  const [copied, setCopied] = useState<string | null>(null);

  const curl = useMemo(
    () => `curl -s ${apiBase}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_PROVIDER_KEY" \\
  -H "X-TokenGuard-API-Key: tg_YOUR_KEY" \\
  -H "X-TokenGuard-Provider: openrouter" \\
  -H "X-TokenGuard-Session-ID: dev-session-1" \\
  -d '{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"max_tokens":64}'`,
    [apiBase],
  );

  const node = useMemo(
    () => `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.OPENROUTER_API_KEY,
  baseURL: "${apiBase}/v1",
  defaultHeaders: {
    "X-TokenGuard-API-Key": process.env.TOKENGUARD_API_KEY,
    "X-TokenGuard-Provider": "openrouter",
    "X-TokenGuard-Session-ID": "my-app-1",
  },
});`,
    [apiBase],
  );

  async function copy(label: string, text: string) {
    await navigator.clipboard.writeText(text);
    setCopied(label);
    setTimeout(() => setCopied(null), 1500);
  }

  return (
    <div className="mx-auto max-w-3xl px-5 py-16 sm:px-8 sm:py-20">
      <p className="font-mono text-[0.7rem] uppercase tracking-[0.14em] text-signal">
        TokenGuard · Developer guide
      </p>
      <h1 className="font-display mt-3 text-4xl font-bold tracking-tight text-text sm:text-5xl">
        Use TokenGuard in 5 minutes
      </h1>
      <p className="mt-4 max-w-xl text-sm leading-relaxed text-muted sm:text-base">
        TokenGuard sits between your app and LLM providers. It enforces budgets
        and stops agent loops before money is spent. Auth stays strong: admin
        secret for management, <code className="text-text">tg_</code> keys for
        apps, provider keys for upstream.
      </p>

      <div className="mt-8 flex flex-wrap gap-3">
        <Link
          href="/dashboard"
          className="rounded-md bg-signal px-4 py-2.5 text-sm font-semibold text-on-signal"
        >
          Open developer console
        </Link>
        <a
          href={`${apiBase}/v1/tokenguard.json`}
          className="rounded-md border border-line bg-panel px-4 py-2.5 text-sm font-semibold text-text"
          target="_blank"
          rel="noreferrer"
        >
          Machine-readable API map
        </a>
        <a
          href={`${apiBase}/healthz`}
          className="rounded-md border border-line bg-panel px-4 py-2.5 text-sm font-semibold text-text"
          target="_blank"
          rel="noreferrer"
        >
          Health check
        </a>
      </div>

      <section className="mt-10 border border-line bg-panel p-5 sm:p-6">
        <h2 className="font-display text-lg font-bold text-text">1. What changes in your app</h2>
        <pre className="mt-3 overflow-x-auto bg-[var(--text)] p-4 font-mono text-[0.78rem] text-[#e8fff8]">
{`BEFORE  App ──► OpenAI / OpenRouter / Anthropic
AFTER   App ──► TokenGuard ──► provider`}
        </pre>
        <p className="mt-3 text-sm text-muted">
          Keep your provider API key. Add a TokenGuard user key. Point the SDK{" "}
          <strong className="text-text">base URL</strong> at the API host (
          <code className="text-text">{apiBase}</code>).
        </p>
      </section>

      <section className="mt-4 border border-line bg-panel p-5 sm:p-6">
        <h2 className="font-display text-lg font-bold text-text">2. Create a user key</h2>
        <ol className="mt-3 list-decimal space-y-2 pl-5 text-sm text-muted">
          <li>
            Open the{" "}
            <Link href="/dashboard" className="text-signal underline-offset-2 hover:underline">
              developer console
            </Link>{" "}
            or{" "}
            <Link href="/portal" className="text-signal underline-offset-2 hover:underline">
              product portal
            </Link>
            .
          </li>
          <li>
            Console: unlock with <code className="text-text">TOKENGUARD_ADMIN_SECRET</code>.
            Portal: sign in with Clerk.
          </li>
          <li>
            Provision / create a key → copy the one-time <code className="text-text">tg_...</code>{" "}
            key.
          </li>
        </ol>
        <p className="mt-3 text-sm text-muted">
          Or <code className="text-text">POST /mgmt/provision</code> with header{" "}
          <code className="text-text">X-TokenGuard-Admin-Secret</code>.
        </p>
      </section>

      <section className="mt-4 border border-line bg-panel p-5 sm:p-6">
        <h2 className="font-display text-lg font-bold text-text">3. Call the proxy</h2>
        <p className="mt-2 text-sm text-muted">Required on every LLM request:</p>
        <div className="mt-3 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-line text-[0.7rem] uppercase tracking-wide text-muted">
                <th className="py-2 pr-3">Header</th>
                <th className="py-2">Value</th>
              </tr>
            </thead>
            <tbody className="text-text-dim">
              <tr className="border-b border-line">
                <td className="py-2 pr-3 font-mono text-xs">X-TokenGuard-API-Key</td>
                <td className="py-2">Your <code>tg_...</code> key</td>
              </tr>
              <tr className="border-b border-line">
                <td className="py-2 pr-3 font-mono text-xs">Authorization / x-api-key</td>
                <td className="py-2">Real provider key (passed through)</td>
              </tr>
              <tr className="border-b border-line">
                <td className="py-2 pr-3 font-mono text-xs">X-TokenGuard-Provider</td>
                <td className="py-2">openai · openrouter · anthropic</td>
              </tr>
              <tr>
                <td className="py-2 pr-3 font-mono text-xs">X-TokenGuard-Session-ID</td>
                <td className="py-2">Stable id per agent run (loop protection)</td>
              </tr>
            </tbody>
          </table>
        </div>
        <pre className="mt-4 overflow-x-auto bg-[var(--text)] p-4 font-mono text-[0.78rem] text-[#e8fff8] whitespace-pre-wrap">
          {curl}
        </pre>
        <button
          type="button"
          className="mt-3 rounded-md border border-line px-3 py-1.5 text-xs font-semibold text-text"
          onClick={() => void copy("curl", curl)}
        >
          {copied === "curl" ? "Copied" : "Copy curl"}
        </button>
      </section>

      <section className="mt-4 border border-line bg-panel p-5 sm:p-6">
        <h2 className="font-display text-lg font-bold text-text">4. SDK pattern (OpenAI-compatible)</h2>
        <pre className="mt-3 overflow-x-auto bg-[var(--text)] p-4 font-mono text-[0.78rem] text-[#e8fff8] whitespace-pre-wrap">
          {node}
        </pre>
        <button
          type="button"
          className="mt-3 rounded-md border border-line px-3 py-1.5 text-xs font-semibold text-text"
          onClick={() => void copy("node", node)}
        >
          {copied === "node" ? "Copied" : "Copy Node"}
        </button>
      </section>

      <section className="mt-4 border border-line bg-panel p-5 sm:p-6">
        <h2 className="font-display text-lg font-bold text-text">5. Status codes you should handle</h2>
        <div className="mt-3 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-line text-[0.7rem] uppercase tracking-wide text-muted">
                <th className="py-2 pr-3">Code</th>
                <th className="py-2">Meaning</th>
              </tr>
            </thead>
            <tbody className="text-text-dim">
              <tr className="border-b border-line"><td className="py-2 pr-3">401</td><td className="py-2">Missing/invalid <code>tg_</code> key</td></tr>
              <tr className="border-b border-line"><td className="py-2 pr-3">400</td><td className="py-2">Bad body or model not in pricing</td></tr>
              <tr className="border-b border-line"><td className="py-2 pr-3">402</td><td className="py-2">Budget exceeded</td></tr>
              <tr className="border-b border-line"><td className="py-2 pr-3">409</td><td className="py-2">Agent loop detected</td></tr>
              <tr><td className="py-2 pr-3">503</td><td className="py-2">Billing/Redis unavailable</td></tr>
            </tbody>
          </table>
        </div>
        <p className="mt-3 text-sm text-muted">
          JSON errors include a machine <code className="text-text">code</code> field (e.g.{" "}
          <code className="text-text">budget_exceeded</code>).
        </p>
      </section>

      <section className="mt-4 border border-line bg-panel p-5 sm:p-6">
        <h2 className="font-display text-lg font-bold text-text">Useful URLs</h2>
        <div className="mt-3 flex flex-wrap gap-2 font-mono text-[0.7rem] text-muted">
          <span className="border border-line px-2 py-1">{apiBase}</span>
          <span className="border border-line px-2 py-1">/docs (this page)</span>
          <span className="border border-line px-2 py-1">/dashboard</span>
          <span className="border border-line px-2 py-1">/portal</span>
          <span className="border border-line px-2 py-1">/v1/tokenguard.json</span>
          <span className="border border-line px-2 py-1">/healthz</span>
          <span className="border border-line px-2 py-1">/mgmt/*</span>
        </div>
      </section>

      <section className="mt-10 border-t border-line pt-8">
        <h2 className="font-display text-lg font-bold text-text">Deeper docs in the repo</h2>
        <ul className="mt-4 space-y-2 text-sm text-muted">
          {(
            [
              ["howToUse", "How to use"],
              ["api", "HTTP API"],
              ["architecture", "Architecture"],
              ["design", "Design invariants"],
              ["deploy", "Deploy"],
              ["integration", "Integration"],
            ] as const
          ).map(([key, label]) => (
            <li key={key}>
              <a
                href={githubDoc(key)}
                className="text-signal underline-offset-2 hover:underline"
                target="_blank"
                rel="noreferrer"
              >
                {label}
              </a>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
