"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import {
  apiBaseUrl,
  mgmtFetch,
  type MgmtPrice,
  type MgmtUsageEvent,
  type MgmtUser,
} from "@/lib/tokenguard-api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToneAlert } from "@/components/ui/tone-alert";
import { cn } from "@/lib/utils";
import { CopyIcon, LockIcon, RefreshCwIcon } from "lucide-react";

type View = "start" | "integrate" | "users" | "pricing" | "usage";
type SnippetLang = "curl" | "node" | "python";

const navItems: { id: View; label: string }[] = [
  { id: "start", label: "Start" },
  { id: "integrate", label: "Integrate" },
  { id: "users", label: "Users" },
  { id: "pricing", label: "Pricing" },
  { id: "usage", label: "Usage" },
];

function formatUSD(micro: number | undefined) {
  return `$${((Number(micro) || 0) / 1_000_000).toFixed(4)}`;
}

function perMillion(p: MgmtPrice, side: "input" | "output") {
  if (side === "input") {
    if (p.input_usd_per_million != null) return p.input_usd_per_million;
    if (p.input_cost_per_1k != null) return Number(p.input_cost_per_1k) / 1000;
  } else {
    if (p.output_usd_per_million != null) return p.output_usd_per_million;
    if (p.output_cost_per_1k != null) return Number(p.output_cost_per_1k) / 1000;
  }
  return 0;
}

export function DashboardApp() {
  const apiBase = apiBaseUrl();
  const docsUrl = `${apiBase}/docs`;

  const [unlocked, setUnlocked] = useState(false);
  const [admin, setAdmin] = useState("");
  const [adminInput, setAdminInput] = useState("");
  const [unlockErr, setUnlockErr] = useState("");
  const [globalErr, setGlobalErr] = useState("");
  const [view, setView] = useState<View>("start");

  const [health, setHealth] = useState("…");
  const [provider, setProvider] = useState("…");
  const [modelsCount, setModelsCount] = useState("…");
  const [infoJson, setInfoJson] = useState("Loading /v1/tokenguard.json…");

  const [users, setUsers] = useState<MgmtUser[]>([]);
  const [usage, setUsage] = useState<MgmtUsageEvent[]>([]);
  const [pricing, setPricing] = useState<MgmtPrice[]>([]);
  const [priceSearch, setPriceSearch] = useState("");

  const [snippetKey, setSnippetKey] = useState("");
  const [snippetProvider, setSnippetProvider] = useState("openrouter");
  const [snippetLang, setSnippetLang] = useState<SnippetLang>("curl");

  const [provisionOpen, setProvisionOpen] = useState(false);
  const [provisionDone, setProvisionDone] = useState(false);
  const [provisionErr, setProvisionErr] = useState("");
  const [newKey, setNewKey] = useState("");
  const [provName, setProvName] = useState("");
  const [provEmail, setProvEmail] = useState("");
  const [provBudget, setProvBudget] = useState("1");

  const [budgetOpen, setBudgetOpen] = useState(false);
  const [budgetUserId, setBudgetUserId] = useState("");
  const [budgetUsd, setBudgetUsd] = useState("1");
  const [budgetReset, setBudgetReset] = useState(false);
  const [budgetErr, setBudgetErr] = useState("");

  const [priceOpen, setPriceOpen] = useState(false);
  const [priceErr, setPriceErr] = useState("");
  const [priceKey, setPriceKey] = useState("");
  const [priceIn, setPriceIn] = useState("");
  const [priceOut, setPriceOut] = useState("");

  const loadInfo = useCallback(async () => {
    try {
      const res = await fetch(`${apiBase}/v1/tokenguard.json`);
      const data = await res.json();
      setInfoJson(JSON.stringify(data, null, 2));
      setProvider(data.default_provider || "—");
      setModelsCount(`${(data.models_priced || []).length} configured`);
    } catch (e) {
      setInfoJson(
        `Failed to load discovery doc: ${e instanceof Error ? e.message : String(e)}`,
      );
    }
  }, [apiBase]);

  const checkHealth = useCallback(async () => {
    try {
      const res = await fetch(`${apiBase}/v1/status`);
      const data = await res.json();
      setHealth(data.status === "ok" ? "ok" : JSON.stringify(data));
    } catch {
      setHealth("unreachable");
    }
  }, [apiBase]);

  const loadDashboardData = useCallback(async (secret: string) => {
    setGlobalErr("");
    const [uRes, rRes] = await Promise.all([
      mgmtFetch<{ users?: MgmtUser[] }>("/mgmt/users", secret),
      mgmtFetch<{ events?: MgmtUsageEvent[] }>("/mgmt/usage?limit=25", secret),
    ]);
    if (!uRes.ok) throw new Error(uRes.data.error || "Failed to list users");
    if (!rRes.ok) throw new Error(rRes.data.error || "Failed to list usage");
    setUsers(uRes.data.users || []);
    setUsage(rRes.data.events || []);
  }, []);

  const loadPricing = useCallback(async (secret: string) => {
    setGlobalErr("");
    const res = await mgmtFetch<{ prices?: MgmtPrice[] }>("/mgmt/pricing", secret);
    if (!res.ok) throw new Error(res.data.error || "Failed to list pricing");
    setPricing(res.data.prices || []);
  }, []);

  const unlock = useCallback(
    async (secret: string) => {
      setUnlockErr("");
      if (secret.length < 16) {
        setUnlockErr("Admin secret must be at least 16 characters.");
        return;
      }
      try {
        const res = await mgmtFetch<{ users?: MgmtUser[] }>("/mgmt/users", secret);
        if (!res.ok) throw new Error(res.data.error || `HTTP ${res.status}`);
        setAdmin(secret);
        setUnlocked(true);
        await Promise.all([loadInfo(), loadDashboardData(secret), checkHealth()]);
      } catch (e) {
        setAdmin("");
        setUnlocked(false);
        setUnlockErr(e instanceof Error ? e.message : "Unlock failed");
      }
    },
    [checkHealth, loadDashboardData, loadInfo],
  );

  useEffect(() => {
    if (!unlocked || view !== "pricing" || !admin) return;
    const timer = window.setTimeout(() => {
      void loadPricing(admin).catch((e) =>
        setGlobalErr(e instanceof Error ? e.message : String(e)),
      );
    }, 0);
    return () => window.clearTimeout(timer);
  }, [unlocked, view, admin, loadPricing]);

  function lockConsole() {
    setAdmin("");
    setUnlocked(false);
    setAdminInput("");
    setView("start");
  }

  function openProvision() {
    setProvisionOpen(true);
    setProvisionDone(false);
    setProvisionErr("");
    setNewKey("");
    setProvName("");
    setProvEmail("");
    setProvBudget("1");
  }

  const snippet = useMemo(() => {
    const key = snippetKey.trim() || "tg_YOUR_KEY";
    const providerName = snippetProvider.trim() || "openrouter";
    const model = providerName === "openai" ? "gpt-4o-mini" : "openai/gpt-4o-mini";
    const snippets: Record<SnippetLang, string> = {
      curl: `curl -s ${apiBase}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer YOUR_PROVIDER_KEY" \\
  -H "X-TokenGuard-API-Key: ${key}" \\
  -H "X-TokenGuard-Provider: ${providerName}" \\
  -H "X-TokenGuard-Session-ID: my-session-1" \\
  -d '{"model":"${model}","messages":[{"role":"user","content":"Hello"}],"max_tokens":64}'`,
      node: `import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.PROVIDER_API_KEY,
  baseURL: "${apiBase}/v1",
  defaultHeaders: {
    "X-TokenGuard-API-Key": "${key}",
    "X-TokenGuard-Provider": "${providerName}",
    "X-TokenGuard-Session-ID": "my-app-1",
  },
});

const res = await client.chat.completions.create({
  model: "${model}",
  messages: [{ role: "user", content: "Hello" }],
  max_tokens: 64,
});`,
      python: `from openai import OpenAI
import os

client = OpenAI(
    api_key=os.environ["PROVIDER_API_KEY"],
    base_url="${apiBase}/v1",
    default_headers={
        "X-TokenGuard-API-Key": "${key}",
        "X-TokenGuard-Provider": "${providerName}",
        "X-TokenGuard-Session-ID": "my-app-1",
    },
)

res = client.chat.completions.create(
    model="${model}",
    messages=[{"role":"user","content":"Hello"}],
    max_tokens=64,
)`,
    };
    return snippets[snippetLang];
  }, [apiBase, snippetKey, snippetLang, snippetProvider]);

  const filteredPrices = useMemo(() => {
    const q = priceSearch.trim().toLowerCase();
    if (!q) return pricing;
    return pricing.filter((p) => (p.model_key || "").toLowerCase().includes(q));
  }, [pricing, priceSearch]);

  async function submitProvision() {
    setProvisionErr("");
    const email = provEmail.trim();
    const budgetUsd = Number(provBudget);
    if (!email) {
      setProvisionErr("Email is required.");
      return;
    }
    if (!(budgetUsd > 0)) {
      setProvisionErr("Budget must be greater than 0.");
      return;
    }
    const res = await mgmtFetch<{ api_key?: string }>("/mgmt/provision", admin, {
      method: "POST",
      body: JSON.stringify({ email, name: provName.trim(), budget_usd: budgetUsd }),
    });
    if (!res.ok) {
      setProvisionErr(res.data.error || "Provision failed");
      return;
    }
    setNewKey(res.data.api_key || "");
    setProvisionDone(true);
  }

  async function submitBudget() {
    setBudgetErr("");
    const usd = Number(budgetUsd);
    if (!(usd > 0)) {
      setBudgetErr("Budget must be greater than 0.");
      return;
    }
    const res = await mgmtFetch("/mgmt/budget", admin, {
      method: "PATCH",
      body: JSON.stringify({
        user_id: budgetUserId,
        budget_usd: usd,
        reset_spent: budgetReset,
      }),
    });
    if (!res.ok) {
      setBudgetErr(res.data.error || "Update failed");
      return;
    }
    setBudgetOpen(false);
    await loadDashboardData(admin);
    toast.success("Budget updated.");
  }

  async function submitPrice() {
    setPriceErr("");
    const model_key = priceKey.trim();
    const input_usd_per_million = Number(priceIn);
    const output_usd_per_million = Number(priceOut);
    if (!model_key) {
      setPriceErr("Model key is required.");
      return;
    }
    if (!(input_usd_per_million >= 0) || !(output_usd_per_million >= 0)) {
      setPriceErr("Costs must be >= 0 ($ per 1M tokens).");
      return;
    }
    const res = await mgmtFetch("/mgmt/pricing/upsert", admin, {
      method: "POST",
      body: JSON.stringify({ model_key, input_usd_per_million, output_usd_per_million }),
    });
    if (!res.ok) {
      setPriceErr(res.data.error || "Upsert failed");
      return;
    }
    setPriceOpen(false);
    await loadPricing(admin);
    await loadInfo();
    toast.success("Price saved.");
  }

  async function deletePrice(modelKey: string) {
    if (!confirm(`Delete price for ${modelKey}?`)) return;
    const res = await mgmtFetch("/mgmt/pricing/delete", admin, {
      method: "POST",
      body: JSON.stringify({ model_key: modelKey }),
    });
    if (!res.ok) {
      setGlobalErr(res.data.error || "Delete failed");
      return;
    }
    await loadPricing(admin);
    await loadInfo();
    toast.success(`Deleted ${modelKey}.`);
  }

  async function syncOpenRouter() {
    setGlobalErr("");
    if (
      !confirm(
        "Import/update live prices from OpenRouter (real USD rates)? This overwrites matching model keys.",
      )
    ) {
      return;
    }
    const res = await mgmtFetch<{ imported?: number; models_priced?: number }>(
      "/mgmt/pricing/sync/openrouter",
      admin,
      { method: "POST" },
    );
    if (!res.ok) {
      setGlobalErr(res.data.error || "Sync failed");
      return;
    }
    await loadPricing(admin);
    await loadInfo();
    toast.success(
      `Imported ${res.data.imported || 0} rows. Catalog: ${res.data.models_priced ?? "—"}`,
    );
  }

  if (!unlocked) {
    return (
      <div className="atmosphere grid min-h-screen place-items-center px-5 py-16">
        <Card className="w-full max-w-md">
          <CardHeader>
            <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
              TokenGuard console
            </p>
            <CardTitle className="font-display text-2xl">Developer access</CardTitle>
            <CardDescription>
              Enter the server admin secret to manage users and copy integration
              snippets. This secret never goes to end users.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {unlockErr ? (
              <ToneAlert tone="error" title="Unlock failed">
                {unlockErr}
              </ToneAlert>
            ) : null}
            <div className="grid gap-1.5">
              <Label htmlFor="admin-input">Admin secret</Label>
              <Input
                id="admin-input"
                type="password"
                autoComplete="current-password"
                placeholder="TOKENGUARD_ADMIN_SECRET"
                className="h-10"
                value={adminInput}
                onChange={(e) => setAdminInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void unlock(adminInput.trim());
                }}
              />
            </div>
            <p className="text-xs leading-5 text-muted-foreground">
              The admin secret is kept in memory only and is cleared when this tab
              is refreshed or closed. Do not use this console on a shared device.
            </p>
            <Button
              size="lg"
              className="w-full"
              type="button"
              onClick={() => void unlock(adminInput.trim())}
            >
              Unlock console
            </Button>
            <p className="text-xs text-muted-foreground">
              Need the guide first?{" "}
              <a
                href={docsUrl}
                target="_blank"
                rel="noreferrer"
                className="font-semibold text-signal underline"
              >
                Open /docs
              </a>{" "}
              (no secret required).
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-ink">
      <div className="mx-auto grid max-w-7xl lg:grid-cols-[14rem_minmax(0,1fr)]">
        <aside className="border-b border-border bg-card px-4 py-5 lg:min-h-screen lg:border-b-0 lg:border-r">
          <p className="font-display text-lg font-bold text-text">TokenGuard</p>
          <p className="text-xs text-muted-foreground">Operator console</p>
          <nav className="mt-5 flex gap-1 overflow-x-auto lg:flex-col" aria-label="Console">
            {navItems.map((item) => (
              <button
                key={item.id}
                type="button"
                onClick={() => setView(item.id)}
                className={cn(
                  "min-h-10 shrink-0 rounded-md px-3 py-2 text-left text-sm font-semibold",
                  view === item.id
                    ? "bg-signal-dim text-signal"
                    : "text-text-dim hover:bg-muted hover:text-text",
                )}
              >
                {item.label}
              </button>
            ))}
            <Button
              variant="outline"
              size="sm"
              className="mt-2 justify-start"
              type="button"
              onClick={lockConsole}
            >
              <LockIcon data-icon="inline-start" />
              Lock console
            </Button>
          </nav>
          <p className="mt-6 hidden break-all font-mono text-[0.65rem] text-muted-foreground lg:block">
            {apiBase}
          </p>
        </aside>

        <main className="min-w-0 px-5 py-7 sm:px-8 lg:px-10">
          <div className="mx-auto max-w-5xl space-y-5">
            {globalErr ? (
              <ToneAlert tone="error" title="Request failed">
                {globalErr}
              </ToneAlert>
            ) : null}

            {view === "start" ? (
              <section className="space-y-5">
                <header className="flex flex-wrap items-end justify-between gap-4">
                  <div>
                    <h1 className="font-display text-3xl font-bold tracking-tight">
                      Start here
                    </h1>
                    <p className="mt-2 text-sm text-muted-foreground">
                      Provision a key, then point any OpenAI-compatible SDK at this host.
                    </p>
                  </div>
                  <Button size="lg" type="button" onClick={openProvision}>
                    + Provision user
                  </Button>
                </header>

                <ToneAlert tone="info" title="Auth model">
                  Admin secret unlocks this console · <code>tg_</code> keys identify
                  apps · provider keys still authenticate upstream. Nothing is “open.”
                </ToneAlert>

                <Card>
                  <CardHeader>
                    <CardTitle className="font-display text-lg">Setup steps</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <ol className="list-decimal space-y-3 pl-5 text-sm leading-6 text-muted-foreground">
                      <li>
                        <strong className="text-text">This host is your new base URL.</strong>
                        <div className="mt-1 font-mono text-xs text-text">{apiBase}</div>
                      </li>
                      <li>
                        <strong className="text-text">Create a TokenGuard user key</strong> with
                        Provision user (or API).
                      </li>
                      <li>
                        <strong className="text-text">Keep your provider API key</strong> (OpenAI
                        / OpenRouter / Anthropic).
                      </li>
                      <li>
                        <strong className="text-text">Add headers</strong>{" "}
                        <code>X-TokenGuard-API-Key</code>, optional provider + session id.
                      </li>
                      <li>
                        <strong className="text-text">Handle</strong> 401 / 400 / 402 / 409 in
                        your client.
                      </li>
                    </ol>
                  </CardContent>
                </Card>

                <div className="grid gap-4 xl:grid-cols-2">
                  <Card>
                    <CardHeader className="flex flex-row items-center justify-between gap-3">
                      <CardTitle className="font-display text-lg">Live service map</CardTitle>
                      <div className="flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          type="button"
                          onClick={() => void loadInfo()}
                        >
                          <RefreshCwIcon data-icon="inline-start" />
                          Refresh
                        </Button>
                        <Button variant="outline" size="sm" asChild>
                          <a href={docsUrl} target="_blank" rel="noreferrer">
                            Public docs
                          </a>
                        </Button>
                      </div>
                    </CardHeader>
                    <CardContent>
                      <pre className="max-h-80 overflow-auto rounded-lg bg-code p-4 text-xs leading-5 text-code-text">
                        {infoJson}
                      </pre>
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader>
                      <CardTitle className="font-display text-lg">Quick checks</CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-3 text-sm">
                      <p>
                        <strong>Health:</strong>{" "}
                        <span className="font-mono">{health}</span>
                      </p>
                      <p>
                        <strong>Default provider:</strong>{" "}
                        <span className="font-mono">{provider}</span>
                      </p>
                      <p>
                        <strong>Priced models:</strong>{" "}
                        <span className="font-mono">{modelsCount}</span>
                      </p>
                      <Button
                        variant="outline"
                        size="sm"
                        type="button"
                        onClick={() =>
                          void loadDashboardData(admin).catch((e) =>
                            setGlobalErr(e instanceof Error ? e.message : String(e)),
                          )
                        }
                      >
                        Refresh users & usage
                      </Button>
                    </CardContent>
                  </Card>
                </div>
              </section>
            ) : null}

            {view === "integrate" ? (
              <section className="space-y-5">
                <header>
                  <h1 className="font-display text-3xl font-bold tracking-tight">
                    Integrate
                  </h1>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Copy-paste snippets for this deployment. Paste your{" "}
                    <code>tg_</code> key where shown.
                  </p>
                </header>

                <Card>
                  <CardHeader>
                    <CardTitle>Snippet inputs</CardTitle>
                  </CardHeader>
                  <CardContent className="grid gap-4 sm:grid-cols-2">
                    <div className="grid gap-1.5">
                      <Label htmlFor="snippet-key">
                        TokenGuard API key (optional — fills snippets)
                      </Label>
                      <Input
                        id="snippet-key"
                        type="text"
                        placeholder="tg_..."
                        className="h-10 font-mono"
                        value={snippetKey}
                        onChange={(e) => setSnippetKey(e.target.value)}
                      />
                    </div>
                    <div className="grid gap-1.5">
                      <Label htmlFor="snippet-provider">Provider</Label>
                      <Input
                        id="snippet-provider"
                        type="text"
                        className="h-10"
                        value={snippetProvider}
                        onChange={(e) => setSnippetProvider(e.target.value)}
                      />
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Request example</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <Tabs
                      value={snippetLang}
                      onValueChange={(value) => setSnippetLang(value as SnippetLang)}
                    >
                      <TabsList>
                        <TabsTrigger value="curl">curl</TabsTrigger>
                        <TabsTrigger value="node">Node</TabsTrigger>
                        <TabsTrigger value="python">Python</TabsTrigger>
                      </TabsList>
                      <TabsContent value={snippetLang} className="mt-4">
                        <pre className="overflow-x-auto rounded-lg bg-code p-5 text-xs leading-6 text-code-text">
                          <code>{snippet}</code>
                        </pre>
                      </TabsContent>
                    </Tabs>
                    <Button
                      variant="outline"
                      type="button"
                      onClick={() => {
                        void navigator.clipboard.writeText(snippet);
                        toast.success("Snippet copied.");
                      }}
                    >
                      <CopyIcon data-icon="inline-start" />
                      Copy
                    </Button>
                  </CardContent>
                </Card>
              </section>
            ) : null}

            {view === "users" ? (
              <section className="space-y-5">
                <header className="flex flex-wrap items-end justify-between gap-4">
                  <div>
                    <h1 className="font-display text-3xl font-bold tracking-tight">
                      Users
                    </h1>
                    <p className="mt-2 text-sm text-muted-foreground">
                      Budgets are in USD (stored as micro-USD server-side). Extend a
                      limit when a user hits the wall.
                    </p>
                  </div>
                  <Button size="lg" type="button" onClick={openProvision}>
                    + Provision user
                  </Button>
                </header>

                <Card>
                  <CardContent className="pt-(--card-spacing)">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>User</TableHead>
                          <TableHead>Email</TableHead>
                          <TableHead>Budget</TableHead>
                          <TableHead>Spent</TableHead>
                          <TableHead className="text-right">Actions</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {users.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={5} className="text-muted-foreground">
                              No users yet. Provision one to get a tg_ key.
                            </TableCell>
                          </TableRow>
                        ) : (
                          users.map((u) => (
                            <TableRow key={u.user_id}>
                              <TableCell>
                                <div className="font-medium">{u.name || "—"}</div>
                                <div className="font-mono text-xs text-muted-foreground">
                                  {u.user_id}
                                </div>
                              </TableCell>
                              <TableCell>{u.email}</TableCell>
                              <TableCell className="font-mono">
                                {formatUSD(u.limit_microusd)}
                              </TableCell>
                              <TableCell className="font-mono">
                                {formatUSD(u.spent_microusd)}
                              </TableCell>
                              <TableCell className="text-right">
                                <Button
                                  variant="outline"
                                  size="sm"
                                  type="button"
                                  onClick={() => {
                                    setBudgetUserId(u.user_id);
                                    setBudgetUsd(
                                      String(Number(u.limit_microusd) / 1_000_000),
                                    );
                                    setBudgetReset(false);
                                    setBudgetErr("");
                                    setBudgetOpen(true);
                                  }}
                                >
                                  Edit
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </section>
            ) : null}

            {view === "pricing" ? (
              <section className="space-y-5">
                <header className="flex flex-wrap items-end justify-between gap-4">
                  <div>
                    <h1 className="font-display text-3xl font-bold tracking-tight">
                      Pricing
                    </h1>
                    <p className="mt-2 text-sm text-muted-foreground">
                      Live catalog in Turso. Enter rates as{" "}
                      <strong>$ per 1M tokens</strong>. Sync OpenRouter for real market
                      prices.
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      type="button"
                      onClick={() => void syncOpenRouter()}
                    >
                      Sync OpenRouter
                    </Button>
                    <Button
                      type="button"
                      onClick={() => {
                        setPriceKey("");
                        setPriceIn("");
                        setPriceOut("");
                        setPriceErr("");
                        setPriceOpen(true);
                      }}
                    >
                      + Add / edit model
                    </Button>
                  </div>
                </header>

                <Card>
                  <CardHeader>
                    <div className="grid max-w-md gap-1.5">
                      <Label htmlFor="price-search">Search</Label>
                      <Input
                        id="price-search"
                        type="text"
                        placeholder="gpt-4o, openrouter/…"
                        className="h-10"
                        value={priceSearch}
                        onChange={(e) => setPriceSearch(e.target.value)}
                      />
                    </div>
                  </CardHeader>
                  <CardContent>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Model key</TableHead>
                          <TableHead>Input $/1M</TableHead>
                          <TableHead>Output $/1M</TableHead>
                          <TableHead className="text-right">Actions</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {filteredPrices.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={4} className="text-muted-foreground">
                              No prices yet. Add one or Sync OpenRouter.
                            </TableCell>
                          </TableRow>
                        ) : (
                          filteredPrices.map((p) => (
                            <TableRow key={p.model_key}>
                              <TableCell className="font-mono">{p.model_key}</TableCell>
                              <TableCell className="font-mono">
                                ${perMillion(p, "input").toFixed(4)}
                              </TableCell>
                              <TableCell className="font-mono">
                                ${perMillion(p, "output").toFixed(4)}
                              </TableCell>
                              <TableCell className="text-right">
                                <span className="inline-flex gap-2">
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    type="button"
                                    onClick={() => {
                                      setPriceKey(p.model_key);
                                      setPriceIn(String(perMillion(p, "input")));
                                      setPriceOut(String(perMillion(p, "output")));
                                      setPriceErr("");
                                      setPriceOpen(true);
                                    }}
                                  >
                                    Edit
                                  </Button>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    type="button"
                                    onClick={() => void deletePrice(p.model_key)}
                                  >
                                    Delete
                                  </Button>
                                </span>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </section>
            ) : null}

            {view === "usage" ? (
              <section className="space-y-5">
                <header className="flex flex-wrap items-end justify-between gap-4">
                  <div>
                    <h1 className="font-display text-3xl font-bold tracking-tight">
                      Usage
                    </h1>
                    <p className="mt-2 text-sm text-muted-foreground">
                      Recent proxy events across users.
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    type="button"
                    onClick={() =>
                      void loadDashboardData(admin).catch((e) =>
                        setGlobalErr(e instanceof Error ? e.message : String(e)),
                      )
                    }
                  >
                    <RefreshCwIcon data-icon="inline-start" />
                    Refresh
                  </Button>
                </header>

                <Card>
                  <CardContent className="pt-(--card-spacing)">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Model</TableHead>
                          <TableHead>User</TableHead>
                          <TableHead>Tokens in/out</TableHead>
                          <TableHead>Cost</TableHead>
                          <TableHead>Status</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {usage.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={5} className="text-muted-foreground">
                              No usage yet.
                            </TableCell>
                          </TableRow>
                        ) : (
                          usage.map((e, i) => (
                            <TableRow key={`${e.user_id}-${e.model}-${i}`}>
                              <TableCell className="font-medium">
                                {e.model || "—"}
                              </TableCell>
                              <TableCell className="font-mono text-xs">
                                {e.user_id || "—"}
                              </TableCell>
                              <TableCell className="font-mono">
                                {e.input_tokens ?? 0} / {e.output_tokens ?? 0}
                              </TableCell>
                              <TableCell className="font-mono">
                                {formatUSD(e.actual_cost_microusd)}
                              </TableCell>
                              <TableCell>
                                <Badge
                                  variant={
                                    e.status === "completed" ? "default" : "destructive"
                                  }
                                >
                                  {e.status}
                                </Badge>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </section>
            ) : null}

            <p className="text-xs text-muted-foreground">
              Product portal:{" "}
              <Link href="/portal" className="font-semibold text-signal underline">
                /portal
              </Link>
            </p>
          </div>
        </main>
      </div>

      <Dialog open={provisionOpen} onOpenChange={setProvisionOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Provision user</DialogTitle>
            <DialogDescription>
              Creates a user and returns a plaintext <code>tg_</code> key once.
            </DialogDescription>
          </DialogHeader>
          {provisionErr ? (
            <ToneAlert tone="error">{provisionErr}</ToneAlert>
          ) : null}
          {provisionDone ? (
            <div className="space-y-3">
              <ToneAlert tone="success" title="Save this key now">
                It is shown once.
                <code className="mt-2 block break-all rounded-md bg-muted p-3 font-mono text-xs text-text">
                  {newKey}
                </code>
              </ToneAlert>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onClick={() => {
                    void navigator.clipboard.writeText(newKey);
                    toast.success("Key copied.");
                  }}
                >
                  <CopyIcon data-icon="inline-start" />
                  Copy key
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onClick={() => {
                    setSnippetKey(newKey);
                    setProvisionOpen(false);
                    setView("integrate");
                  }}
                >
                  Use in Integrate tab
                </Button>
              </div>
              <DialogFooter>
                <Button
                  type="button"
                  onClick={() => {
                    setProvisionOpen(false);
                    void loadDashboardData(admin);
                  }}
                >
                  Done
                </Button>
              </DialogFooter>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="grid gap-1.5">
                <Label htmlFor="input-name">Name</Label>
                <Input
                  id="input-name"
                  type="text"
                  placeholder="Acme Agent"
                  className="h-10"
                  value={provName}
                  onChange={(e) => setProvName(e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="input-email">Email</Label>
                <Input
                  id="input-email"
                  type="email"
                  placeholder="dev@acme.com"
                  className="h-10"
                  value={provEmail}
                  onChange={(e) => setProvEmail(e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="input-budget">Budget (USD)</Label>
                <Input
                  id="input-budget"
                  type="number"
                  min={0.01}
                  step={0.01}
                  className="h-10"
                  value={provBudget}
                  onChange={(e) => setProvBudget(e.target.value)}
                />
              </div>
              <DialogFooter>
                <Button
                  variant="outline"
                  type="button"
                  onClick={() => setProvisionOpen(false)}
                >
                  Cancel
                </Button>
                <Button type="button" onClick={() => void submitProvision()}>
                  Create user & key
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={budgetOpen} onOpenChange={setBudgetOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Edit budget</DialogTitle>
            <DialogDescription className="font-mono text-xs">
              {budgetUserId}
            </DialogDescription>
          </DialogHeader>
          {budgetErr ? <ToneAlert tone="error">{budgetErr}</ToneAlert> : null}
          <div className="space-y-4">
            <div className="grid gap-1.5">
              <Label htmlFor="budget-usd">New limit (USD)</Label>
              <Input
                id="budget-usd"
                type="number"
                min={0.01}
                step={0.01}
                className="h-10"
                value={budgetUsd}
                onChange={(e) => setBudgetUsd(e.target.value)}
              />
            </div>
            <label className="flex items-center gap-2 text-sm font-medium">
              <Checkbox
                checked={budgetReset}
                onCheckedChange={(checked) => setBudgetReset(checked === true)}
              />
              Also reset spent to $0 (fresh period)
            </label>
          </div>
          <DialogFooter>
            <Button variant="outline" type="button" onClick={() => setBudgetOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={() => void submitBudget()}>
              Save budget
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={priceOpen} onOpenChange={setPriceOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Model price</DialogTitle>
            <DialogDescription>
              Example: gpt-4o-mini is $0.15 input / $0.60 output per 1M tokens.
            </DialogDescription>
          </DialogHeader>
          {priceErr ? <ToneAlert tone="error">{priceErr}</ToneAlert> : null}
          <div className="space-y-4">
            <div className="grid gap-1.5">
              <Label htmlFor="price-key">Model key</Label>
              <Input
                id="price-key"
                type="text"
                placeholder="gpt-4o-mini or openrouter/openai/gpt-4o-mini"
                className="h-10 font-mono"
                value={priceKey}
                onChange={(e) => setPriceKey(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="price-in">Input ($ per 1M tokens)</Label>
              <Input
                id="price-in"
                type="number"
                min={0}
                step={0.0001}
                className="h-10"
                value={priceIn}
                onChange={(e) => setPriceIn(e.target.value)}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="price-out">Output ($ per 1M tokens)</Label>
              <Input
                id="price-out"
                type="number"
                min={0}
                step={0.0001}
                className="h-10"
                value={priceOut}
                onChange={(e) => setPriceOut(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" type="button" onClick={() => setPriceOpen(false)}>
              Cancel
            </Button>
            <Button type="button" onClick={() => void submitPrice()}>
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
