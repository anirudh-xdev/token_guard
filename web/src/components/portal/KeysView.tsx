"use client";

import { useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { EmptyState } from "@/components/ui/empty-state";
import { StatusBadge } from "@/components/ui/status-badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { ToneAlert } from "@/components/ui/tone-alert";
import { tgPortalFetch } from "@/lib/tokenguard-api";
import { CopyIcon, KeyRoundIcon } from "lucide-react";

export function KeysView() {
  const { me, getToken, refreshMe, setError, setNotice } = usePortal();
  const [busy, setBusy] = useState("");
  const [newKey, setNewKey] = useState("");
  const [copied, setCopied] = useState(false);
  const [revoke, setRevoke] = useState<{ id: string; name: string } | null>(null);
  const keys = me.user.keys ?? [];
  const canCreate = me.limits?.can_create_key ?? false;
  const maxKeys = me.limits?.max_keys ?? 0;

  async function createKey() {
    setBusy("create");
    setError("");
    setNotice("");
    try {
      const { ok, data } = await tgPortalFetch<{ api_key?: string }>(
        "/portal/api/keys",
        getToken,
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
      const { ok, data } = await tgPortalFetch(
        "/portal/api/keys/revoke",
        getToken,
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
          <h1 className="mt-1 font-display text-3xl font-bold tracking-tight">
            API keys
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Keys belong to you. A team is selected per request with{" "}
            <code>X-TokenGuard-Team-ID</code>.
          </p>
        </div>
        <Button
          size="lg"
          onClick={() => void createKey()}
          disabled={!canCreate || busy === "create"}
        >
          <KeyRoundIcon data-icon="inline-start" />
          {busy === "create" ? "Creating…" : "Create key"}
        </Button>
      </header>

      {!canCreate ? (
        <ToneAlert tone="warning" title="Key limit reached">
          You reached the limit of {maxKeys} active keys. Revoke an unused key
          before creating another.
        </ToneAlert>
      ) : null}

      {newKey ? (
        <ToneAlert tone="success" title="Copy this key now">
          <p className="mb-2">It will not be shown again.</p>
          <code className="mt-1 block break-all rounded-md bg-panel/80 p-3 font-mono text-xs text-text">
            {newKey}
          </code>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => void copyKey()}
          >
            <CopyIcon data-icon="inline-start" />
            {copied ? "Copied" : "Copy key"}
          </Button>
        </ToneAlert>
      ) : null}

      {keys.length === 0 ? (
        <EmptyState
          title="Create your first API key"
          description="Your application sends this key to TokenGuard. Provider credentials remain separate and are forwarded upstream."
          action={
            <Button
              size="lg"
              onClick={() => void createKey()}
              disabled={!canCreate || busy === "create"}
            >
              Create key
            </Button>
          }
        />
      ) : (
        <Card>
          <CardHeader>
            <CardTitle className="font-display text-lg">Your keys</CardTitle>
            <CardDescription>
              Prefix only is stored in the UI after creation. Revoking is immediate.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-0 px-0">
            {keys.map((key, index) => (
              <div key={key.id}>
                {index > 0 ? <Separator /> : null}
                <div className="flex flex-wrap items-center justify-between gap-4 px-(--card-spacing) py-4">
                  <div>
                    <p className="font-semibold text-text">{key.name || "API key"}</p>
                    <p className="mt-1 font-mono text-xs text-muted-foreground">
                      {key.key_prefix || "tg_"}… · created{" "}
                      {key.created_at
                        ? new Date(key.created_at).toLocaleDateString()
                        : "—"}
                    </p>
                  </div>
                  <div className="flex items-center gap-3">
                    <StatusBadge status={key.status || "unknown"} />
                    {key.status === "active" ? (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() =>
                          setRevoke({ id: key.id, name: key.name || "API key" })
                        }
                      >
                        Revoke
                      </Button>
                    ) : null}
                  </div>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <Dialog
        open={Boolean(revoke)}
        onOpenChange={(open) => !open && setRevoke(null)}
      >
        <DialogContent showCloseButton={!Boolean(revoke && busy === revoke.id)}>
          <DialogHeader>
            <DialogTitle>Revoke API key?</DialogTitle>
            <DialogDescription>
              Applications using {revoke?.name || "this key"} will immediately fail
              authentication. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setRevoke(null)}
              disabled={Boolean(revoke && busy === revoke.id)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => void revokeKey()}
              disabled={Boolean(revoke && busy === revoke.id)}
            >
              {revoke && busy === revoke.id ? "Working…" : "Revoke key"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
