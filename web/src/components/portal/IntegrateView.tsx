"use client";

import Link from "next/link";
import { useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToneAlert } from "@/components/ui/tone-alert";
import { apiBaseUrl } from "@/lib/tokenguard-api";
import { CopyIcon } from "lucide-react";

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
        <h1 className="mt-1 font-display text-3xl font-bold tracking-tight">
          Route an LLM call through TokenGuard
        </h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
          Replace the provider base URL, keep the provider credential, and add
          your personal TokenGuard key. For agents, also send a session id.
        </p>
      </header>

      {selectedTeam ? (
        <ToneAlert tone="info" title="Team-attributed snippet">
          This example charges <strong>{selectedTeam.name}</strong> and includes{" "}
          <code>X-TokenGuard-Team-ID: {selectedTeam.id}</code>. Selecting a scope
          in the portal does not change your running application.
        </ToneAlert>
      ) : (
        <ToneAlert tone="info" title="Personal budget snippet">
          This example charges your personal budget because it has no team
          header. Select a team scope to generate a team-attributed example.
        </ToneAlert>
      )}

      <section aria-labelledby="steps-heading" className="space-y-4">
        <h2 id="steps-heading" className="font-display text-lg font-semibold">
          Before you send a request
        </h2>
        <ol className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
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

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-4">
          <div>
            <CardTitle
              id="snippet-heading"
              className="font-display text-lg"
            >
              Request example
            </CardTitle>
            <CardDescription>
              Copy into your server or agent runtime — never into browser code.
            </CardDescription>
          </div>
          <div className="flex gap-2">
            <label className="sr-only" htmlFor="snippet-language">
              Snippet language
            </label>
            <Select
              value={language}
              onValueChange={(value) => setLanguage(value as "curl" | "node")}
            >
              <SelectTrigger id="snippet-language" className="h-10 w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="curl">cURL</SelectItem>
                <SelectItem value="node">Node.js</SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" size="lg" onClick={() => void copy()}>
              <CopyIcon data-icon="inline-start" />
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <pre className="overflow-x-auto rounded-lg bg-code p-5 text-xs leading-6 text-code-text">
            <code>{snippet}</code>
          </pre>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">Security checks</CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="list-disc space-y-2 pl-5 text-sm leading-6 text-muted-foreground">
            <li>
              Never put either key in browser code or commit it to source control.
            </li>
            <li>
              TokenGuard strips its internal headers before forwarding upstream.
            </li>
            <li>
              Unknown models are blocked; add pricing through the operator catalog
              first.
            </li>
          </ul>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">Still stuck?</CardTitle>
          <CardDescription>
            See the plain-language{" "}
            <Link href="/portal/faq" className="font-semibold text-signal underline">
              FAQ
            </Link>{" "}
            for session id, team id, provider keys, 402/409 errors, and a
            non-developer first checklist.
          </CardDescription>
        </CardHeader>
      </Card>
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
    <li>
      <Card size="sm" className="h-full">
        <CardHeader>
          <span className="font-mono text-sm font-semibold text-signal">
            {number}
          </span>
          <CardTitle className="mt-1">{title}</CardTitle>
          <CardDescription className="text-sm leading-6">{children}</CardDescription>
        </CardHeader>
      </Card>
    </li>
  );
}
