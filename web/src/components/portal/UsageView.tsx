"use client";

import { useCallback, useEffect, useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { formatMicroUSD, formatNumber } from "@/components/portal/OverviewView";
import {
  Alert,
  Button,
  EmptyState,
  Skeleton,
  StatusBadge,
} from "@/components/ui/PortalUI";
import {
  asArray,
  tgPortalFetch,
  type PortalUsageEvent,
} from "@/lib/tokenguard-api";

export function UsageView() {
  const { scopeID, selectedTeam, getToken } = usePortal();
  const [events, setEvents] = useState<PortalUsageEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const query = scopeID ? `?team_id=${encodeURIComponent(scopeID)}` : "";
      const { ok, data } = await tgPortalFetch<{ events?: PortalUsageEvent[] }>(
        `/portal/api/usage${query}`,
        getToken,
      );
      if (!ok) throw new Error(data.error || "Could not load usage");
      setEvents(asArray(data.events));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load usage");
    } finally {
      setLoading(false);
    }
  }, [getToken, scopeID]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
            {selectedTeam ? `${selectedTeam.name} scope` : "Personal scope"}
          </p>
          <h1 className="mt-1 font-display text-3xl font-bold">Usage</h1>
          <p className="mt-2 text-sm text-muted">
            {selectedTeam?.my_role === "owner"
              ? "Every request charged to this team pool."
              : selectedTeam
                ? "Only your requests charged to this team."
                : "Only requests charged to your personal budget."}
          </p>
        </div>
        <Button variant="secondary" onClick={() => void load()} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </header>

      {error ? <Alert tone="error">{error}</Alert> : null}

      {loading ? (
        <div aria-label="Loading usage" className="space-y-3">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : events.length === 0 ? (
        <EmptyState
          title="No usage in this scope"
          description={
            selectedTeam
              ? "Use this team’s X-TokenGuard-Team-ID in an API request. Personal calls will not appear here."
              : "Calls made without X-TokenGuard-Team-ID will appear here after they complete or are blocked."
          }
        />
      ) : (
        <section aria-label="Recent usage events" className="overflow-x-auto">
          <table className="w-full min-w-[48rem] border-collapse text-left text-sm">
            <thead>
              <tr className="border-b border-line text-xs uppercase tracking-[0.08em] text-muted">
                <th scope="col" className="py-3 pr-4">Status</th>
                <th scope="col" className="py-3 pr-4">Model</th>
                <th scope="col" className="py-3 pr-4">Provider</th>
                {selectedTeam?.my_role === "owner" ? (
                  <th scope="col" className="py-3 pr-4">Member</th>
                ) : null}
                <th scope="col" className="py-3 pr-4 text-right">Tokens in / out</th>
                <th scope="col" className="py-3 pr-4 text-right">Cost</th>
                <th scope="col" className="py-3 text-right">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {events.map((event) => (
                <tr key={event.id}>
                  <td className="py-3 pr-4"><StatusBadge status={event.status} /></td>
                  <th scope="row" className="py-3 pr-4 font-mono font-medium">{event.model}</th>
                  <td className="py-3 pr-4 text-muted">{event.provider}</td>
                  {selectedTeam?.my_role === "owner" ? (
                    <td className="max-w-44 truncate py-3 pr-4 font-mono text-xs text-muted" title={event.user_id}>
                      {event.user_id}
                    </td>
                  ) : null}
                  <td className="py-3 pr-4 text-right font-mono">
                    {formatNumber(event.input_tokens)} / {formatNumber(event.output_tokens)}
                  </td>
                  <td className="py-3 pr-4 text-right font-mono">
                    {formatMicroUSD(event.actual_cost_microusd)}
                  </td>
                  <td className="py-3 text-right text-muted">
                    {event.created_at ? new Date(event.created_at).toLocaleString() : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      <p className="text-xs leading-5 text-muted">
        Showing the 25 most recent events. Historical rows created before team
        attribution was introduced may remain in personal history because they
        cannot be reassigned safely.
      </p>
    </>
  );
}
