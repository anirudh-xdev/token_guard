"use client";

import Link from "next/link";
import { useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { Alert, Button } from "@/components/ui/PortalUI";
import { apiBaseUrl } from "@/lib/tokenguard-api";

export function IntegrateView() {
  const { selectedTeam } = usePortal();
  const [language, setLanguage] = useState<"curl" | "node">("curl");
  const [copied, setCopied] = useState(false);
  const teamLine = selectedTeam
    ? language === "curl"
      ? `  -H "X-TokenGuard-Team-ID: ${selectedTeam.id}" \\\n`
      : `      "X-TokenGuard-Team-ID": "${selectedTeam.id}",\n`
    : "";
  const snippet =
    language === "curl"
      ? `# Use the SAME session id for every call in one agent run.
# Change it only when the user starts a brand-new run.
SESSION_ID="agent-run-1"

curl "${apiBaseUrl()}/v1/chat/completions" \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer $PROVIDER_API_KEY" \\
  -H "X-TokenGuard-API-Key: $TOKENGUARD_API_KEY" \\
  -H "X-TokenGuard-Session-ID: $SESSION_ID" \\
${teamLine}  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'

# Later steps in the same run keep $SESSION_ID unchanged.
# Next independent run: SESSION_ID="agent-run-2" (or a new UUID).`
      : `// WHERE to create the id:
// at the start of an agent run / chat thread in YOUR app
// (handler, job, LangGraph thread, OpenAI Agents run, etc.)
async function runAgentTask(userGoal) {
  // Create ONCE for this run. Store it in a local variable,
  // request context, or your agent-state object.
  const sessionId = \`agent-run-\${crypto.randomUUID()}\`;

  // Helper that always reuses that same sessionId.
  async function callModel(messages) {
    return fetch("${apiBaseUrl()}/v1/chat/completions", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": \`Bearer \${process.env.PROVIDER_API_KEY}\`,
        "X-TokenGuard-API-Key": process.env.TOKENGUARD_API_KEY,
        "X-TokenGuard-Session-ID": sessionId, // same value every step
${teamLine}      },
      body: JSON.stringify({ model: "gpt-4o-mini", messages }),
    });
  }

  // Step 1 and Step 2 share sessionId → TokenGuard can detect loops.
  await callModel([{ role: "user", content: userGoal }]);
  await callModel([
    { role: "user", content: userGoal },
    { role: "assistant", content: "..." },
    { role: "user", content: "continue" },
  ]);
}

// New user task / new conversation → call runAgentTask() again.
// That creates a NEW sessionId automatically.`;

  async function copy() {
    await navigator.clipboard.writeText(snippet);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <>
      <header>
        <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
          Integration
        </p>
        <h1 className="mt-1 font-display text-3xl font-bold">
          Route an LLM call through TokenGuard
        </h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-muted">
          Replace the provider base URL, keep the provider credential, and add
          your personal TokenGuard key. For agents, also send a session id.
        </p>
      </header>

      {selectedTeam ? (
        <Alert tone="info">
          This snippet charges <strong>{selectedTeam.name}</strong> and includes{" "}
          <code>X-TokenGuard-Team-ID: {selectedTeam.id}</code>. Selecting a scope
          in the portal does not change your running application.
        </Alert>
      ) : (
        <Alert tone="info">
          This snippet charges your personal budget because it has no team
          header. Select a team scope to generate a team-attributed example.
        </Alert>
      )}

      <section aria-labelledby="steps-heading">
        <h2 id="steps-heading" className="font-display text-lg font-semibold">
          Before you send a request
        </h2>
        <ol className="mt-4 grid gap-px overflow-hidden rounded-lg border border-line bg-line md:grid-cols-4">
          <Step number="1" title="Create a key">
            Generate a TokenGuard API key on the API keys page.
          </Step>
          <Step number="2" title="Set two secrets">
            Store provider and TokenGuard keys in server-side environment variables.
          </Step>
          <Step number="3" title="Call the proxy">
            Use TokenGuard’s base URL and a model in the configured pricing catalog.
          </Step>
          <Step number="4" title="Add a session id">
            For agents, send <code>X-TokenGuard-Session-ID</code> so loop
            protection can trip before spend runs away.
          </Step>
        </ol>
      </section>

      <section aria-labelledby="snippet-heading">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <h2 id="snippet-heading" className="font-display text-lg font-semibold">
            Request example
          </h2>
          <div className="flex gap-2">
            <label className="sr-only" htmlFor="snippet-language">
              Snippet language
            </label>
            <select
              id="snippet-language"
              value={language}
              onChange={(event) => setLanguage(event.target.value as "curl" | "node")}
              className="min-h-11 rounded-md border border-line bg-panel px-3 text-sm"
            >
              <option value="curl">cURL</option>
              <option value="node">Node.js</option>
            </select>
            <Button variant="secondary" onClick={() => void copy()}>
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
        </div>
        <pre className="mt-4 overflow-x-auto rounded-lg bg-code p-5 text-xs leading-6 text-code-text">
          <code>{snippet}</code>
        </pre>
      </section>

      <section aria-labelledby="security-heading" className="border-t border-line pt-6">
        <h2 id="security-heading" className="font-display text-lg font-semibold">
          Security checks
        </h2>
        <ul className="mt-3 list-disc space-y-2 pl-5 text-sm leading-6 text-muted">
          <li>Never put either key in browser code or commit it to source control.</li>
          <li>TokenGuard strips its internal headers before forwarding upstream.</li>
          <li>Unknown models are blocked; add pricing through the operator catalog first.</li>
        </ul>
      </section>

      <section aria-labelledby="faq-link-heading" className="border-t border-line pt-6">
        <h2 id="faq-link-heading" className="font-display text-lg font-semibold">
          Still stuck?
        </h2>
        <p className="mt-2 text-sm leading-6 text-muted">
          See the plain-language{" "}
          <Link href="/portal/faq" className="font-semibold text-signal underline">
            FAQ
          </Link>{" "}
          for session id, team id, provider keys, 402/409 errors, and a
          non-developer first checklist.
        </p>
      </section>
    </>
  );
}

function Step({
  number,
  title,
  children,
}: {
  number: string;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <li className="bg-panel p-5">
      <span className="font-mono text-sm font-semibold text-signal">{number}</span>
      <h3 className="mt-2 font-semibold">{title}</h3>
      <p className="mt-1 text-sm leading-6 text-muted">{children}</p>
    </li>
  );
}
