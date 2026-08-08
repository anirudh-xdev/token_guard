"use client";

import { useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import {
  Alert,
  Button,
  ConfirmDialog,
  EmptyState,
  StatusBadge,
} from "@/components/ui/PortalUI";
import { tgFetch } from "@/lib/tokenguard-api";

export function KeysView() {
  const { me, getToken, refreshMe, setError, setNotice } = usePortal();
  const [busy, setBusy] = useState("");
  const [newKey, setNewKey] = useState("");
  const [copied, setCopied] = useState(false);
  const [revoke, setRevoke] = useState<{ id: string; name: string } | null>(null);
  const canCreate = me.limits.can_create_key;

  async function createKey() {
    setBusy("create");
    setError("");
    setNotice("");
    try {
      const token = await getToken();
      const { ok, data } = await tgFetch<{ api_key?: string }>(
        "/portal/api/keys",
        token,
        { method: "POST", body: JSON.stringify({ name: "default" }) },
      );
      if (!ok) throw new Error(data.error || "Could not create key");
      setNewKey(data.api_key || "");
      await refreshMe();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not create key");
    } finally {
      setBusy("");
    }
  }

  async function revokeKey() {
    if (!revoke) return;
    setBusy(revoke.id);
    setError("");
    try {
      const token = await getToken();
      const { ok, data } = await tgFetch(
        "/portal/api/keys/revoke",
        token,
        { method: "POST", body: JSON.stringify({ key_id: revoke.id }) },
      );
      if (!ok) throw new Error(data.error || "Could not revoke key");
      setRevoke(null);
      setNotice(`Revoked ${revoke.name}. Requests using it will now return 401.`);
      await refreshMe();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not revoke key");
    } finally {
      setBusy("");
    }
  }

  async function copyKey() {
    await navigator.clipboard.writeText(newKey);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
            Personal access
          </p>
          <h1 className="mt-1 font-display text-3xl font-bold">API keys</h1>
          <p className="mt-2 text-sm text-muted">
            Keys belong to you. A team is selected per request with{" "}
            <code>X-TokenGuard-Team-ID</code>.
          </p>
        </div>
        <Button onClick={() => void createKey()} disabled={!canCreate || busy === "create"}>
          {busy === "create" ? "Creating…" : "Create key"}
        </Button>
      </header>

      {!canCreate ? (
        <Alert tone="warning">
          You reached the limit of {me.limits.max_keys} active keys. Revoke an
          unused key before creating another.
        </Alert>
      ) : null}

      {newKey ? (
        <Alert tone="success">
          <p className="font-semibold">Copy this key now. It will not be shown again.</p>
          <code className="mt-2 block break-all rounded bg-panel/70 p-3 text-xs">
            {newKey}
          </code>
          <Button variant="secondary" className="mt-3" onClick={() => void copyKey()}>
            {copied ? "Copied" : "Copy key"}
          </Button>
        </Alert>
      ) : null}

      {me.user.keys.length === 0 ? (
        <EmptyState
          title="Create your first API key"
          description="Your application sends this key to TokenGuard. Provider credentials remain separate and are forwarded upstream."
          action={
            <Button onClick={() => void createKey()} disabled={!canCreate || busy === "create"}>
              Create key
            </Button>
          }
        />
      ) : (
        <section aria-labelledby="key-list-heading">
          <h2 id="key-list-heading" className="font-display text-lg font-semibold">
            Your keys
          </h2>
          <ul className="mt-4 divide-y divide-line border-y border-line">
            {me.user.keys.map((key) => (
              <li key={key.id} className="flex flex-wrap items-center justify-between gap-4 py-4">
                <div>
                  <p className="font-semibold text-text">{key.name}</p>
                  <p className="mt-1 font-mono text-xs text-muted">
                    {key.key_prefix}… · created{" "}
                    {new Date(key.created_at).toLocaleDateString()}
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  <StatusBadge status={key.status} />
                  {key.status === "active" ? (
                    <Button
                      variant="danger"
                      onClick={() => setRevoke({ id: key.id, name: key.name })}
                    >
                      Revoke
                    </Button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      <ConfirmDialog
        open={Boolean(revoke)}
        title="Revoke API key?"
        description={`Applications using ${revoke?.name || "this key"} will immediately fail authentication. This cannot be undone.`}
        confirmLabel="Revoke key"
        busy={Boolean(revoke && busy === revoke.id)}
        onClose={() => setRevoke(null)}
        onConfirm={() => void revokeKey()}
      />
    </>
  );
}
