"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { apiBaseUrl } from "@/lib/tokenguard-api";
import { githubDoc } from "@/lib/site";
import { CopyIcon } from "lucide-react";

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
    toast.success("Copied to clipboard");
    setTimeout(() => setCopied(null), 1500);
  }

  return (
    <div className="mx-auto max-w-3xl space-y-5 px-5 py-16 sm:px-8 sm:py-20">
      <header>
        <p className="font-mono text-[0.7rem] uppercase tracking-[0.14em] text-signal">
          TokenGuard · Developer guide
        </p>
        <h1 className="font-display mt-3 text-4xl font-bold tracking-tight text-text sm:text-5xl">
          Use TokenGuard in 5 minutes
        </h1>
        <p className="mt-4 max-w-xl text-sm leading-relaxed text-muted-foreground sm:text-base">
          TokenGuard sits between your app and LLM providers. It enforces budgets
          and stops agent loops before money is spent. Auth stays strong: admin
          secret for management, <code className="text-text">tg_</code> keys for
          apps, provider keys for upstream.
        </p>
        <div className="mt-8 flex flex-wrap gap-2">
          <Button size="lg" className="text-white!" asChild>
            <Link href="/dashboard">Open developer console</Link>
          </Button>
          <Button variant="outline" size="lg" asChild>
            <a
              href={`${apiBase}/v1/tokenguard.json`}
              target="_blank"
              rel="noreferrer"
            >
              Machine-readable API map
            </a>
          </Button>
          <Button variant="outline" size="lg" asChild>
            <a href={`${apiBase}/v1/status`} target="_blank" rel="noreferrer">
              Health check
            </a>
          </Button>
        </div>
      </header>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">
            1. What changes in your app
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <pre className="overflow-x-auto rounded-lg bg-code p-4 font-mono text-[0.78rem] text-code-text">
{`BEFORE  App ──► OpenAI / OpenRouter / Anthropic
AFTER   App ──► TokenGuard ──► provider`}
          </pre>
          <p className="text-sm text-muted-foreground">
            Keep your provider API key. Add a TokenGuard user key. Point the SDK{" "}
            <strong className="text-text">base URL</strong> at the API host (
            <code className="text-text">{apiBase}</code>).
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">2. Create a user key</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <ol className="list-decimal space-y-2 pl-5 text-sm text-muted-foreground">
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
              Provision / create a key → copy the one-time{" "}
              <code className="text-text">tg_...</code> key.
            </li>
          </ol>
          <p className="text-sm text-muted-foreground">
            Or <code className="text-text">POST /mgmt/provision</code> with header{" "}
            <code className="text-text">X-TokenGuard-Admin-Secret</code>.
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">3. Call the proxy</CardTitle>
          <CardDescription>Required on every LLM request:</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Header</TableHead>
                <TableHead>Value</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell className="font-mono text-xs">
                  X-TokenGuard-API-Key
                </TableCell>
                <TableCell>
                  Your <code>tg_...</code> key
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell className="font-mono text-xs">
                  Authorization / x-api-key
                </TableCell>
                <TableCell>Real provider key (passed through)</TableCell>
              </TableRow>
              <TableRow>
                <TableCell className="font-mono text-xs">
                  X-TokenGuard-Provider
                </TableCell>
                <TableCell>openai · openrouter · anthropic</TableCell>
              </TableRow>
              <TableRow>
                <TableCell className="font-mono text-xs">
                  X-TokenGuard-Session-ID
                </TableCell>
                <TableCell>Stable id per agent run (loop protection)</TableCell>
              </TableRow>
            </TableBody>
          </Table>
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-code p-4 font-mono text-[0.78rem] text-code-text">
            {curl}
          </pre>
          <Button variant="outline" size="sm" type="button" onClick={() => void copy("curl", curl)}>
            <CopyIcon data-icon="inline-start" />
            {copied === "curl" ? "Copied" : "Copy curl"}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">
            4. SDK pattern (OpenAI-compatible)
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-code p-4 font-mono text-[0.78rem] text-code-text">
            {node}
          </pre>
          <Button variant="outline" size="sm" type="button" onClick={() => void copy("node", node)}>
            <CopyIcon data-icon="inline-start" />
            {copied === "node" ? "Copied" : "Copy Node"}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">
            5. Status codes you should handle
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Code</TableHead>
                <TableHead>Meaning</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>401</TableCell>
                <TableCell>
                  Missing/invalid <code>tg_</code> key
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>400</TableCell>
                <TableCell>Bad body or model not in pricing</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>402</TableCell>
                <TableCell>Budget exceeded</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>409</TableCell>
                <TableCell>Agent loop detected</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>503</TableCell>
                <TableCell>Billing/Redis unavailable</TableCell>
              </TableRow>
            </TableBody>
          </Table>
          <p className="text-sm text-muted-foreground">
            JSON errors include a machine <code className="text-text">code</code>{" "}
            field (e.g. <code className="text-text">budget_exceeded</code>).
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">Useful URLs</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-2 font-mono text-[0.7rem] text-muted-foreground">
            {[
              apiBase,
              "/docs (this page)",
              "/dashboard",
              "/portal",
              "/v1/tokenguard.json",
              "/v1/status",
              "/mgmt/*",
            ].map((item) => (
              <span
                key={item}
                className="rounded-md border border-border bg-muted/40 px-2 py-1"
              >
                {item}
              </span>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">Deeper docs in the repo</CardTitle>
        </CardHeader>
        <CardContent>
          <ul className="space-y-2 text-sm text-muted-foreground">
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
        </CardContent>
      </Card>
    </div>
  );
}
