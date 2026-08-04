"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  apiBaseUrl,
  mgmtFetch,
  type MgmtPrice,
  type MgmtUsageEvent,
  type MgmtUser,
} from "@/lib/tokenguard-api";

const SECRET_KEY = "tokenguard_admin_secret";
type View = "start" | "integrate" | "users" | "pricing" | "usage";
type SnippetLang = "curl" | "node" | "python";

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
  const [remember, setRemember] = useState(true);
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
      setInfoJson(`Failed to load discovery doc: ${e instanceof Error ? e.message : String(e)}`);
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

  const loadDashboardData = useCallback(
    async (secret: string) => {
      setGlobalErr("");
      const [uRes, rRes] = await Promise.all([
        mgmtFetch<{ users?: MgmtUser[] }>("/mgmt/users", secret),
        mgmtFetch<{ events?: MgmtUsageEvent[] }>("/mgmt/usage?limit=25", secret),
      ]);
      if (!uRes.ok) throw new Error(uRes.data.error || "Failed to list users");
      if (!rRes.ok) throw new Error(rRes.data.error || "Failed to list usage");
      setUsers(uRes.data.users || []);
      setUsage(rRes.data.events || []);
    },
    [],
  );

  const loadPricing = useCallback(async (secret: string) => {
    setGlobalErr("");
    const res = await mgmtFetch<{ prices?: MgmtPrice[] }>("/mgmt/pricing", secret);
    if (!res.ok) throw new Error(res.data.error || "Failed to list pricing");
    setPricing(res.data.prices || []);
  }, []);

  const unlock = useCallback(
    async (secret: string, persist: boolean) => {
      setUnlockErr("");
      if (secret.length < 16) {
        setUnlockErr("Admin secret must be at least 16 characters.");
        return;
      }
      try {
        const res = await mgmtFetch<{ users?: MgmtUser[] }>("/mgmt/users", secret);
        if (!res.ok) throw new Error(res.data.error || `HTTP ${res.status}`);
        if (persist) localStorage.setItem(SECRET_KEY, secret);
        else localStorage.removeItem(SECRET_KEY);
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
    const saved = localStorage.getItem(SECRET_KEY) || "";
    if (!saved) return;
    setAdminInput(saved);
    void unlock(saved, true);
    // Auto-unlock once on mount from remembered secret.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (unlocked && view === "pricing" && admin) {
      void loadPricing(admin).catch((e) =>
        setGlobalErr(e instanceof Error ? e.message : String(e)),
      );
    }
  }, [unlocked, view, admin, loadPricing]);

  function lockConsole() {
    setAdmin("");
    setUnlocked(false);
    localStorage.removeItem(SECRET_KEY);
    setAdminInput("");
    setView("start");
  }

  function nav(name: View | "lock") {
    if (name === "lock") {
      lockConsole();
      return;
    }
    setView(name);
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
    alert(
      `Imported ${res.data.imported || 0} price rows from live OpenRouter catalog. Catalog size: ${res.data.models_priced ?? "—"}`,
    );
  }

  if (!unlocked) {
    return (
      <div className="tg-console">
        <div className="unlock">
          <div className="unlock-card">
            <div className="brand">TokenGuard console</div>
            <h1>Developer access</h1>
            <p>
              Enter the server admin secret to manage users and copy integration
              snippets. This secret never goes to end users.
            </p>
            {unlockErr ? <div className="err">{unlockErr}</div> : null}
            <label htmlFor="admin-input">Admin secret</label>
            <input
              id="admin-input"
              type="password"
              autoComplete="current-password"
              placeholder="TOKENGUARD_ADMIN_SECRET"
              value={adminInput}
              onChange={(e) => setAdminInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void unlock(adminInput.trim(), remember);
              }}
            />
            <label
              style={{
                display: "flex",
                alignItems: "center",
                gap: "0.4rem",
                fontWeight: 500,
                marginBottom: "1rem",
              }}
            >
              <input
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
              />
              Remember on this device
            </label>
            <button
              className="btn"
              type="button"
              style={{ width: "100%" }}
              onClick={() => void unlock(adminInput.trim(), remember)}
            >
              Unlock console
            </button>
            <p className="kv" style={{ marginTop: "1rem" }}>
              Need the guide first?{" "}
              <a href={docsUrl} target="_blank" rel="noreferrer">
                Open /docs
              </a>{" "}
              (no secret required).
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="tg-console">
      <div className="layout">
        <aside className="sidebar">
          <div className="logo">TokenGuard</div>
          {(
            [
              ["start", "Start"],
              ["integrate", "Integrate"],
              ["users", "Users"],
              ["pricing", "Pricing"],
              ["usage", "Usage"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              className={`nav-btn${view === id ? " active" : ""}`}
              type="button"
              onClick={() => nav(id)}
            >
              {label}
            </button>
          ))}
          <button className="nav-btn" type="button" onClick={() => nav("lock")}>
            Lock console
          </button>
          <div className="foot">{apiBase}</div>
        </aside>

        <main className="main">
          {globalErr ? <div className="err">{globalErr}</div> : null}

          {view === "start" ? (
            <section>
              <div className="header">
                <div>
                  <h1>Start here</h1>
                  <p>Provision a key, then point any OpenAI-compatible SDK at this host.</p>
                </div>
                <button
                  className="btn"
                  type="button"
                  onClick={() => {
                    setProvisionOpen(true);
                    setProvisionDone(false);
                    setProvisionErr("");
                    setNewKey("");
                    setProvName("");
                    setProvEmail("");
                    setProvBudget("1");
                  }}
                >
                  + Provision user
                </button>
              </div>
              <div className="banner">
                Auth model: admin secret unlocks this console · <code>tg_</code> keys
                identify apps · provider keys still authenticate upstream. Nothing is
                “open.”
              </div>
              <div className="panel">
                <ol className="steps">
                  <li>
                    <strong>This host is your new base URL.</strong>
                    <div className="mono">{apiBase}</div>
                  </li>
                  <li>
                    <strong>Create a TokenGuard user key</strong> with Provision user
                    (or API).
                  </li>
                  <li>
                    <strong>Keep your provider API key</strong> (OpenAI / OpenRouter /
                    Anthropic).
                  </li>
                  <li>
                    <strong>Add headers</strong> <code>X-TokenGuard-API-Key</code>,
                    optional provider + session id.
                  </li>
                  <li>
                    <strong>Handle</strong> 401 / 400 / 402 / 409 in your client.
                  </li>
                </ol>
              </div>
              <div className="grid-2">
                <div className="panel">
                  <h2>Live service map</h2>
                  <pre>{infoJson}</pre>
                  <div className="toolbar">
                    <button className="btn secondary small" type="button" onClick={() => void loadInfo()}>
                      Refresh
                    </button>
                    <a className="btn ghost small" href={docsUrl} target="_blank" rel="noreferrer">
                      Public docs
                    </a>
                  </div>
                </div>
                <div className="panel">
                  <h2>Quick checks</h2>
                  <p className="kv">
                    <strong>Health:</strong> <span>{health}</span>
                  </p>
                  <p className="kv">
                    <strong>Default provider:</strong> <span>{provider}</span>
                  </p>
                  <p className="kv">
                    <strong>Priced models:</strong> <span>{modelsCount}</span>
                  </p>
                  <div className="toolbar">
                    <button
                      className="btn secondary small"
                      type="button"
                      onClick={() =>
                        void loadDashboardData(admin).catch((e) =>
                          setGlobalErr(e instanceof Error ? e.message : String(e)),
                        )
                      }
                    >
                      Refresh users & usage
                    </button>
                  </div>
                </div>
              </div>
            </section>
          ) : null}

          {view === "integrate" ? (
            <section>
              <div className="header">
                <div>
                  <h1>Integrate</h1>
                  <p>
                    Copy-paste snippets for this deployment. Paste your <code>tg_</code>{" "}
                    key where shown.
                  </p>
                </div>
              </div>
              <div className="panel">
                <label htmlFor="snippet-key">TokenGuard API key (optional — fills snippets)</label>
                <input
                  id="snippet-key"
                  type="text"
                  placeholder="tg_..."
                  value={snippetKey}
                  onChange={(e) => setSnippetKey(e.target.value)}
                />
                <label htmlFor="snippet-provider">Provider</label>
                <input
                  id="snippet-provider"
                  type="text"
                  value={snippetProvider}
                  onChange={(e) => setSnippetProvider(e.target.value)}
                />
              </div>
              <div className="panel">
                <div className="tabs">
                  {(["curl", "node", "python"] as const).map((lang) => (
                    <button
                      key={lang}
                      className={`tab${snippetLang === lang ? " active" : ""}`}
                      type="button"
                      onClick={() => setSnippetLang(lang)}
                    >
                      {lang === "node" ? "Node" : lang === "python" ? "Python" : "curl"}
                    </button>
                  ))}
                </div>
                <pre>{snippet}</pre>
                <div className="toolbar">
                  <button
                    className="btn secondary small"
                    type="button"
                    onClick={() => void navigator.clipboard.writeText(snippet)}
                  >
                    Copy
                  </button>
                </div>
              </div>
            </section>
          ) : null}

          {view === "users" ? (
            <section>
              <div className="header">
                <div>
                  <h1>Users</h1>
                  <p>
                    Budgets are in USD (stored as micro-USD server-side). Extend a limit
                    when a user hits the wall.
                  </p>
                </div>
                <button
                  className="btn"
                  type="button"
                  onClick={() => {
                    setProvisionOpen(true);
                    setProvisionDone(false);
                    setProvisionErr("");
                    setNewKey("");
                  }}
                >
                  + Provision user
                </button>
              </div>
              <div className="panel">
                <table>
                  <thead>
                    <tr>
                      <th>User</th>
                      <th>Email</th>
                      <th>Budget</th>
                      <th>Spent</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {users.length === 0 ? (
                      <tr>
                        <td colSpan={5} className="kv">
                          No users yet. Provision one to get a tg_ key.
                        </td>
                      </tr>
                    ) : (
                      users.map((u) => (
                        <tr key={u.user_id}>
                          <td>
                            <div>{u.name || "—"}</div>
                            <div className="mono kv">{u.user_id}</div>
                          </td>
                          <td>{u.email}</td>
                          <td>{formatUSD(u.limit_microusd)}</td>
                          <td>{formatUSD(u.spent_microusd)}</td>
                          <td>
                            <button
                              className="btn secondary small"
                              type="button"
                              onClick={() => {
                                setBudgetUserId(u.user_id);
                                setBudgetUsd(String(Number(u.limit_microusd) / 1_000_000));
                                setBudgetReset(false);
                                setBudgetErr("");
                                setBudgetOpen(true);
                              }}
                            >
                              Edit
                            </button>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          {view === "pricing" ? (
            <section>
              <div className="header">
                <div>
                  <h1>Pricing</h1>
                  <p>
                    Live catalog in Turso. Enter rates as <strong>$ per 1M tokens</strong>.
                    Sync OpenRouter for real market prices.
                  </p>
                </div>
                <div className="toolbar" style={{ marginTop: 0 }}>
                  <button className="btn secondary" type="button" onClick={() => void syncOpenRouter()}>
                    Sync OpenRouter
                  </button>
                  <button
                    className="btn"
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
                  </button>
                </div>
              </div>
              <div className="panel">
                <label htmlFor="price-search">Search</label>
                <input
                  id="price-search"
                  type="text"
                  placeholder="gpt-4o, openrouter/…"
                  value={priceSearch}
                  onChange={(e) => setPriceSearch(e.target.value)}
                />
                <table>
                  <thead>
                    <tr>
                      <th>Model key</th>
                      <th>Input $/1M</th>
                      <th>Output $/1M</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {filteredPrices.length === 0 ? (
                      <tr>
                        <td colSpan={4} className="kv">
                          No prices yet. Add one or Sync OpenRouter.
                        </td>
                      </tr>
                    ) : (
                      filteredPrices.map((p) => (
                        <tr key={p.model_key}>
                          <td className="mono">{p.model_key}</td>
                          <td>${perMillion(p, "input").toFixed(4)}</td>
                          <td>${perMillion(p, "output").toFixed(4)}</td>
                          <td>
                            <button
                              className="btn secondary small"
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
                            </button>{" "}
                            <button
                              className="btn ghost small"
                              type="button"
                              onClick={() => void deletePrice(p.model_key)}
                            >
                              Delete
                            </button>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          {view === "usage" ? (
            <section>
              <div className="header">
                <div>
                  <h1>Usage</h1>
                  <p>Recent proxy events across users.</p>
                </div>
                <button
                  className="btn secondary"
                  type="button"
                  onClick={() =>
                    void loadDashboardData(admin).catch((e) =>
                      setGlobalErr(e instanceof Error ? e.message : String(e)),
                    )
                  }
                >
                  Refresh
                </button>
              </div>
              <div className="panel">
                <table>
                  <thead>
                    <tr>
                      <th>Model</th>
                      <th>User</th>
                      <th>Tokens in/out</th>
                      <th>Cost</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {usage.length === 0 ? (
                      <tr>
                        <td colSpan={5} className="kv">
                          No usage yet.
                        </td>
                      </tr>
                    ) : (
                      usage.map((e, i) => (
                        <tr key={`${e.user_id}-${e.model}-${i}`}>
                          <td>
                            <strong>{e.model || "—"}</strong>
                          </td>
                          <td className="mono">{e.user_id || "—"}</td>
                          <td>
                            {e.input_tokens ?? 0} / {e.output_tokens ?? 0}
                          </td>
                          <td>{formatUSD(e.actual_cost_microusd)}</td>
                          <td>
                            <span className={`badge ${e.status === "completed" ? "ok" : "bad"}`}>
                              {e.status}
                            </span>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          ) : null}

          <p className="kv" style={{ marginTop: "2rem" }}>
            Product portal: <Link href="/portal">/portal</Link>
          </p>
        </main>
      </div>

      {provisionOpen ? (
        <div className="backdrop">
          <div className="modal">
            <h2>Provision user</h2>
            {provisionErr ? <div className="err">{provisionErr}</div> : null}
            {provisionDone ? (
              <>
                <div className="success-box">
                  <strong>Save this key now — it is shown once.</strong>
                  <code>{newKey}</code>
                  <div className="toolbar">
                    <button
                      className="btn secondary small"
                      type="button"
                      onClick={() => void navigator.clipboard.writeText(newKey)}
                    >
                      Copy key
                    </button>
                    <button
                      className="btn secondary small"
                      type="button"
                      onClick={() => {
                        setSnippetKey(newKey);
                        setProvisionOpen(false);
                        setView("integrate");
                      }}
                    >
                      Use in Integrate tab
                    </button>
                  </div>
                </div>
                <div className="modal-actions">
                  <button
                    className="btn"
                    type="button"
                    onClick={() => {
                      setProvisionOpen(false);
                      void loadDashboardData(admin);
                    }}
                  >
                    Done
                  </button>
                </div>
              </>
            ) : (
              <>
                <label htmlFor="input-name">Name</label>
                <input
                  id="input-name"
                  type="text"
                  placeholder="Acme Agent"
                  value={provName}
                  onChange={(e) => setProvName(e.target.value)}
                />
                <label htmlFor="input-email">Email</label>
                <input
                  id="input-email"
                  type="email"
                  placeholder="dev@acme.com"
                  value={provEmail}
                  onChange={(e) => setProvEmail(e.target.value)}
                />
                <label htmlFor="input-budget">Budget (USD)</label>
                <input
                  id="input-budget"
                  type="number"
                  min={0.01}
                  step={0.01}
                  value={provBudget}
                  onChange={(e) => setProvBudget(e.target.value)}
                />
                <div className="modal-actions">
                  <button className="btn ghost" type="button" onClick={() => setProvisionOpen(false)}>
                    Cancel
                  </button>
                  <button className="btn" type="button" onClick={() => void submitProvision()}>
                    Create user & key
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      ) : null}

      {budgetOpen ? (
        <div className="backdrop">
          <div className="modal">
            <h2>Edit budget</h2>
            {budgetErr ? <div className="err">{budgetErr}</div> : null}
            <p className="kv mono">{budgetUserId}</p>
            <label htmlFor="budget-usd">New limit (USD)</label>
            <input
              id="budget-usd"
              type="number"
              min={0.01}
              step={0.01}
              value={budgetUsd}
              onChange={(e) => setBudgetUsd(e.target.value)}
            />
            <label
              style={{
                display: "flex",
                alignItems: "center",
                gap: "0.4rem",
                fontWeight: 500,
                marginBottom: "1rem",
              }}
            >
              <input
                type="checkbox"
                checked={budgetReset}
                onChange={(e) => setBudgetReset(e.target.checked)}
              />
              Also reset spent to $0 (fresh period)
            </label>
            <div className="modal-actions">
              <button className="btn ghost" type="button" onClick={() => setBudgetOpen(false)}>
                Cancel
              </button>
              <button className="btn" type="button" onClick={() => void submitBudget()}>
                Save budget
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {priceOpen ? (
        <div className="backdrop">
          <div className="modal">
            <h2>Model price</h2>
            {priceErr ? <div className="err">{priceErr}</div> : null}
            <label htmlFor="price-key">Model key</label>
            <input
              id="price-key"
              type="text"
              placeholder="gpt-4o-mini or openrouter/openai/gpt-4o-mini"
              value={priceKey}
              onChange={(e) => setPriceKey(e.target.value)}
            />
            <label htmlFor="price-in">Input ($ per 1M tokens)</label>
            <input
              id="price-in"
              type="number"
              min={0}
              step={0.0001}
              value={priceIn}
              onChange={(e) => setPriceIn(e.target.value)}
            />
            <label htmlFor="price-out">Output ($ per 1M tokens)</label>
            <input
              id="price-out"
              type="number"
              min={0}
              step={0.0001}
              value={priceOut}
              onChange={(e) => setPriceOut(e.target.value)}
            />
            <p className="kv">
              Example: gpt-4o-mini is $0.15 input / $0.60 output per 1M tokens.
            </p>
            <div className="modal-actions">
              <button className="btn ghost" type="button" onClick={() => setPriceOpen(false)}>
                Cancel
              </button>
              <button className="btn" type="button" onClick={() => void submitPrice()}>
                Save
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
