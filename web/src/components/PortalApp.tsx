"use client";

import { useAuth } from "@clerk/nextjs";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { apiBaseUrl, tgFetch, type MeResponse } from "@/lib/tokenguard-api";

function money(n: number | undefined) {
  if (typeof n !== "number") return "—";
  return `$${n.toFixed(2)}`;
}

export function PortalApp() {
  const { getToken, isLoaded, isSignedIn } = useAuth();
  const [me, setMe] = useState<MeResponse | null>(null);
  const [error, setError] = useState("");
  const [newKey, setNewKey] = useState("");
  const [teamName, setTeamName] = useState("");
  const [teamBudget, setTeamBudget] = useState("2000");
  const [selectedTeam, setSelectedTeam] = useState<{
    id: string;
    name: string;
    isOwner: boolean;
    poolUsd: number;
  } | null>(null);
  const [members, setMembers] = useState<
    Array<{
      user_id: string;
      email: string;
      role: string;
      cap_usd: number;
      spent_usd: number;
    }>
  >([]);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteCap, setInviteCap] = useState("200");
  const [poolEdit, setPoolEdit] = useState("");
  const [capEdits, setCapEdits] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);

  const withToken = useCallback(async () => {
    const token = await getToken();
    return token;
  }, [getToken]);

  const loadMe = useCallback(async () => {
    setError("");
    const token = await withToken();
    const { ok, data } = await tgFetch<MeResponse>("/portal/api/me", token);
    if (!ok) {
      setError(data.error || "Failed to load account");
      setMe(null);
      return;
    }
    setMe(data);
  }, [withToken]);

  useEffect(() => {
    if (isLoaded && isSignedIn) {
      void loadMe();
    }
  }, [isLoaded, isSignedIn, loadMe]);

  async function createKey() {
    setBusy(true);
    setError("");
    try {
      const token = await withToken();
      const { ok, data } = await tgFetch<{ api_key?: string; error?: string }>(
        "/portal/api/keys",
        token,
        { method: "POST", body: JSON.stringify({ name: "default" }) },
      );
      if (!ok) {
        setError(data.error || "Could not create key");
        return;
      }
      setNewKey(data.api_key || "");
      await loadMe();
    } finally {
      setBusy(false);
    }
  }

  async function revokeKey(id: string) {
    if (!confirm("Revoke this API key?")) return;
    const token = await withToken();
    const { ok, data } = await tgFetch<{ error?: string }>(
      "/portal/api/keys/revoke",
      token,
      { method: "POST", body: JSON.stringify({ key_id: id }) },
    );
    if (!ok) {
      setError(data.error || "Revoke failed");
      return;
    }
    await loadMe();
  }

  async function createTeam() {
    setBusy(true);
    setError("");
    try {
      const token = await withToken();
      const { ok, data } = await tgFetch<{ error?: string }>(
        "/portal/api/teams",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            name: teamName.trim(),
            budget_usd: Number(teamBudget),
          }),
        },
      );
      if (!ok) {
        setError(data.error || "Create team failed");
        return;
      }
      setTeamName("");
      await loadMe();
    } finally {
      setBusy(false);
    }
  }

  async function openTeam(
    id: string,
    name: string,
    isOwner: boolean,
    poolUsd: number,
  ) {
    setSelectedTeam({ id, name, isOwner, poolUsd });
    setPoolEdit(String(poolUsd));
    const token = await withToken();
    const { ok, data } = await tgFetch<{
      members?: Array<{
        user_id: string;
        email: string;
        role: string;
        cap_usd: number;
        spent_usd: number;
      }>;
      error?: string;
    }>(`/portal/api/teams/members?team_id=${encodeURIComponent(id)}`, token);
    if (!ok) {
      setError(data.error || "Failed to load members");
      return;
    }
    const list = data.members || [];
    setMembers(list);
    const edits: Record<string, string> = {};
    for (const m of list) {
      edits[m.user_id] = String(m.cap_usd);
    }
    setCapEdits(edits);
  }

  async function inviteMember() {
    if (!selectedTeam) return;
    setBusy(true);
    setError("");
    try {
      const token = await withToken();
      const { ok, data } = await tgFetch<{ error?: string }>(
        "/portal/api/teams/members",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            email: inviteEmail.trim(),
            cap_usd: Number(inviteCap),
          }),
        },
      );
      if (!ok) {
        setError(data.error || "Invite failed");
        return;
      }
      setInviteEmail("");
      await openTeam(
        selectedTeam.id,
        selectedTeam.name,
        selectedTeam.isOwner,
        selectedTeam.poolUsd,
      );
      await loadMe();
    } finally {
      setBusy(false);
    }
  }

  async function saveTeamPool() {
    if (!selectedTeam?.isOwner) return;
    setBusy(true);
    setError("");
    try {
      const token = await withToken();
      const { ok, data } = await tgFetch<{ budget_usd?: number; error?: string }>(
        "/portal/api/teams/budget",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            budget_usd: Number(poolEdit),
          }),
        },
      );
      if (!ok) {
        setError(data.error || "Update pool failed");
        return;
      }
      const nextPool = data.budget_usd ?? Number(poolEdit);
      await openTeam(selectedTeam.id, selectedTeam.name, true, nextPool);
      await loadMe();
    } finally {
      setBusy(false);
    }
  }

  async function saveMemberCap(userId: string) {
    if (!selectedTeam?.isOwner) return;
    setBusy(true);
    setError("");
    try {
      const token = await withToken();
      const { ok, data } = await tgFetch<{ error?: string }>(
        "/portal/api/teams/members/cap",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            user_id: userId,
            cap_usd: Number(capEdits[userId] ?? "0"),
          }),
        },
      );
      if (!ok) {
        setError(data.error || "Update cap failed");
        return;
      }
      await openTeam(
        selectedTeam.id,
        selectedTeam.name,
        true,
        selectedTeam.poolUsd,
      );
      await loadMe();
    } finally {
      setBusy(false);
    }
  }

  async function removeMember(userId: string, email: string) {
    if (!selectedTeam?.isOwner) return;
    if (!confirm(`Remove ${email} from the team?`)) return;
    setBusy(true);
    setError("");
    try {
      const token = await withToken();
      const { ok, data } = await tgFetch<{ error?: string }>(
        "/portal/api/teams/members/remove",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            user_id: userId,
          }),
        },
      );
      if (!ok) {
        setError(data.error || "Remove failed");
        return;
      }
      await openTeam(
        selectedTeam.id,
        selectedTeam.name,
        true,
        selectedTeam.poolUsd,
      );
      await loadMe();
    } finally {
      setBusy(false);
    }
  }

  if (!isLoaded) {
    return <p className="text-muted">Loading…</p>;
  }

  if (!isSignedIn) {
    return null;
  }

  const u = me?.user;
  const integration = me?.integration;

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6 px-5 py-10 sm:px-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="font-mono text-[0.7rem] uppercase tracking-[0.14em] text-signal">
            Product portal
          </p>
          <h1 className="mt-2 font-display text-3xl font-bold text-text">
            Your API access
          </h1>
          <p className="mt-2 max-w-xl text-sm text-muted">
            Auth is here (Next.js + Clerk). Budgets, keys, and teams live on the
            TokenGuard API at{" "}
            <code className="font-mono text-text-dim">{apiBaseUrl()}</code>.
          </p>
        </div>
      </div>

      {error ? (
        <p className="rounded-md border border-danger/30 bg-danger-dim px-3 py-2 text-sm text-danger">
          {error}
        </p>
      ) : null}

      {newKey ? (
        <div className="rounded-md border border-signal/30 bg-signal-dim px-4 py-3 text-sm">
          <p className="font-semibold text-text">Copy your API key now</p>
          <code className="mt-2 block break-all font-mono text-text-dim">
            {newKey}
          </code>
          <button
            type="button"
            className="btn-ghost mt-3 px-3 py-1.5 text-xs"
            onClick={() => void navigator.clipboard.writeText(newKey)}
          >
            Copy
          </button>
        </div>
      ) : null}

      <section className="rounded-xl border border-line bg-panel p-5 shadow-[var(--shadow-sm)]">
        <div className="flex items-center justify-between">
          <h2 className="font-display text-lg font-semibold text-text">
            Account
          </h2>
          <button
            type="button"
            className="btn-ghost px-3 py-1.5 text-xs"
            onClick={() => void loadMe()}
            disabled={busy}
          >
            Refresh
          </button>
        </div>
        <p className="mt-1 text-sm text-muted">
          {u ? `${u.name || u.email} · ${u.email}` : "Loading account…"}
        </p>
        <div className="mt-4 grid gap-3 sm:grid-cols-3">
          {[
            ["Budget", money(u?.budget_usd)],
            ["Spent", money(u?.spent_usd)],
            ["Available", money(u?.available_usd)],
          ].map(([label, value]) => (
            <div
              key={label}
              className="rounded-lg border border-line bg-ink px-3 py-3"
            >
              <p className="text-[0.65rem] uppercase tracking-[0.12em] text-muted">
                {label}
              </p>
              <p className="mt-1 font-mono text-lg text-text">{value}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="rounded-xl border border-line bg-panel p-5 shadow-[var(--shadow-sm)]">
        <div className="flex items-center justify-between gap-3">
          <h2 className="font-display text-lg font-semibold text-text">
            API keys
          </h2>
          <button
            type="button"
            className="rounded-md bg-signal px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.08em] text-on-signal disabled:opacity-50"
            onClick={() => void createKey()}
            disabled={busy || me?.limits.can_create_key === false}
          >
            Create key
          </button>
        </div>
        <ul className="mt-4 divide-y divide-line">
          {(u?.keys || []).length === 0 ? (
            <li className="py-3 text-sm text-muted">No keys yet.</li>
          ) : (
            (u?.keys || []).map((k) => (
              <li
                key={k.id}
                className="flex items-center justify-between gap-3 py-3 text-sm"
              >
                <span>
                  <code className="font-mono text-text-dim">
                    {k.key_prefix}…
                  </code>{" "}
                  <span className="text-muted">
                    {k.name} · {k.status}
                  </span>
                </span>
                {k.status === "active" ? (
                  <button
                    type="button"
                    className="btn-ghost px-2 py-1 text-xs"
                    onClick={() => void revokeKey(k.id)}
                  >
                    Revoke
                  </button>
                ) : null}
              </li>
            ))
          )}
        </ul>
      </section>

      <section className="rounded-xl border border-line bg-panel p-5 shadow-[var(--shadow-sm)]">
        <h2 className="font-display text-lg font-semibold text-text">Teams</h2>
        <p className="mt-1 text-sm text-muted">
          Owner sets a pool. Each member gets a cap inside that pool.
        </p>
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <label className="text-sm">
            <span className="mb-1 block font-semibold text-text-dim">
              Team name
            </span>
            <input
              className="w-full rounded-md border border-line bg-ink px-3 py-2"
              value={teamName}
              onChange={(e) => setTeamName(e.target.value)}
              placeholder="Acme AI"
            />
          </label>
          <label className="text-sm">
            <span className="mb-1 block font-semibold text-text-dim">
              Pool (USD)
            </span>
            <input
              type="number"
              min={0}
              className="w-full rounded-md border border-line bg-ink px-3 py-2"
              value={teamBudget}
              onChange={(e) => setTeamBudget(e.target.value)}
            />
          </label>
        </div>
        <button
          type="button"
          className="mt-3 rounded-md bg-signal px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.08em] text-on-signal disabled:opacity-50"
          onClick={() => void createTeam()}
          disabled={busy || !teamName.trim()}
        >
          Create team
        </button>
        <ul className="mt-4 divide-y divide-line">
          {(u?.teams || []).length === 0 ? (
            <li className="py-3 text-sm text-muted">No teams yet.</li>
          ) : (
            (u?.teams || []).map((t) => (
              <li
                key={t.id}
                className="flex items-center justify-between gap-3 py-3 text-sm"
              >
                <span>
                  <strong className="text-text">{t.name}</strong>{" "}
                  <span className="text-muted">
                    pool {money(t.budget_usd)} · my cap {money(t.my_cap_usd)} ·{" "}
                    {t.my_role}
                  </span>
                </span>
                <button
                  type="button"
                  className="btn-ghost px-2 py-1 text-xs"
                  onClick={() =>
                    void openTeam(
                      t.id,
                      t.name,
                      t.my_role === "owner",
                      t.budget_usd,
                    )
                  }
                >
                  Manage
                </button>
              </li>
            ))
          )}
        </ul>

        {selectedTeam ? (
          <div className="mt-5 border-t border-line pt-5">
            <h3 className="font-semibold text-text">
              {selectedTeam.name} — members
            </h3>
            {selectedTeam.isOwner ? (
              <>
                <div className="mt-3 flex flex-wrap items-end gap-2">
                  <label className="text-sm">
                    <span className="mb-1 block font-semibold text-text-dim">
                      Pool (USD)
                    </span>
                    <input
                      type="number"
                      min={0}
                      className="w-40 rounded-md border border-line bg-ink px-3 py-2 text-sm"
                      value={poolEdit}
                      onChange={(e) => setPoolEdit(e.target.value)}
                    />
                  </label>
                  <button
                    type="button"
                    className="rounded-md border border-line px-3 py-2 text-xs font-semibold text-text"
                    onClick={() => void saveTeamPool()}
                    disabled={busy}
                  >
                    Save pool
                  </button>
                </div>
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                  <input
                    className="rounded-md border border-line bg-ink px-3 py-2 text-sm"
                    placeholder="member@company.com"
                    value={inviteEmail}
                    onChange={(e) => setInviteEmail(e.target.value)}
                  />
                  <div className="flex gap-2">
                    <input
                      type="number"
                      min={0}
                      className="w-full rounded-md border border-line bg-ink px-3 py-2 text-sm"
                      value={inviteCap}
                      onChange={(e) => setInviteCap(e.target.value)}
                    />
                    <button
                      type="button"
                      className="rounded-md bg-signal px-3 py-1.5 text-xs font-semibold text-on-signal"
                      onClick={() => void inviteMember()}
                      disabled={busy}
                    >
                      Invite
                    </button>
                  </div>
                </div>
              </>
            ) : null}
            <ul className="mt-3 divide-y divide-line">
              {members.map((m) => (
                <li
                  key={m.user_id}
                  className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm"
                >
                  <span>
                    {m.email}{" "}
                    <span className="text-muted">
                      {m.role} · spent {money(m.spent_usd)}
                    </span>
                  </span>
                  {selectedTeam.isOwner && m.role !== "owner" ? (
                    <span className="flex flex-wrap items-center gap-2">
                      <input
                        type="number"
                        min={0}
                        className="w-24 rounded-md border border-line bg-ink px-2 py-1 text-xs"
                        value={capEdits[m.user_id] ?? String(m.cap_usd)}
                        onChange={(e) =>
                          setCapEdits((prev) => ({
                            ...prev,
                            [m.user_id]: e.target.value,
                          }))
                        }
                      />
                      <button
                        type="button"
                        className="btn-ghost px-2 py-1 text-xs"
                        onClick={() => void saveMemberCap(m.user_id)}
                        disabled={busy}
                      >
                        Save cap
                      </button>
                      <button
                        type="button"
                        className="btn-ghost px-2 py-1 text-xs text-danger"
                        onClick={() => void removeMember(m.user_id, m.email)}
                        disabled={busy}
                      >
                        Remove
                      </button>
                    </span>
                  ) : (
                    <span className="text-muted">cap {money(m.cap_usd)}</span>
                  )}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </section>

      <section className="rounded-xl border border-line bg-panel p-5 shadow-[var(--shadow-sm)]">
        <h2 className="font-display text-lg font-semibold text-text">
          Integrate
        </h2>
        <p className="mt-2 text-sm text-muted">
          Base URL{" "}
          <code className="font-mono text-text-dim">
            {integration?.proxy_base_url || `${apiBaseUrl()}/v1`}
          </code>
        </p>
        <pre className="mt-3 overflow-x-auto rounded-lg bg-[#071410] p-4 font-mono text-xs leading-relaxed text-[#e8f5f1]">
          {`curl -X POST ${integration?.proxy_url || `${apiBaseUrl()}/v1/chat/completions`} \\
  -H "Authorization: Bearer $OPENAI_API_KEY" \\
  -H "X-TokenGuard-API-Key: tg_your_key" \\
  -H "Content-Type: application/json" \\
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"max_tokens":50}'`}
        </pre>
        <p className="mt-3 text-sm text-muted">
          <Link href="/docs" className="text-signal hover:underline">
            Docs
          </Link>{" "}
          · Operator console at{" "}
          <Link href="/dashboard" className="text-signal hover:underline">
            /dashboard
          </Link>
          .
        </p>
      </section>
    </div>
  );
}
