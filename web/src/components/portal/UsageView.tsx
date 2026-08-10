"use client";

import { useCallback, useEffect, useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { formatMicroUSD, formatNumber } from "@/components/portal/OverviewView";
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ToneAlert } from "@/components/ui/tone-alert";
import {
  asArray,
  tgPortalFetch,
  type PortalUsageEvent,
} from "@/lib/tokenguard-api";
import { RefreshCwIcon } from "lucide-react";

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
          <h1 className="mt-1 font-display text-3xl font-bold tracking-tight">
            Usage
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {selectedTeam?.my_role === "owner"
              ? "Every request charged to this team pool."
              : selectedTeam
                ? "Only your requests charged to this team."
                : "Only requests charged to your personal budget."}
          </p>
        </div>
        <Button
          variant="outline"
          size="lg"
          onClick={() => void load()}
          disabled={loading}
        >
          <RefreshCwIcon
            data-icon="inline-start"
            className={loading ? "animate-spin" : undefined}
          />
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </header>

      {error ? (
        <ToneAlert tone="error" title="Could not load usage">
          {error}
        </ToneAlert>
      ) : null}

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
        <Card>
          <CardHeader>
            <CardTitle className="font-display text-lg">Recent events</CardTitle>
            <CardDescription>
              Latest 25 events for the selected spend scope.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Status</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead>Provider</TableHead>
                  {selectedTeam?.my_role === "owner" ? (
                    <TableHead>Member</TableHead>
                  ) : null}
                  <TableHead className="text-right">Tokens in / out</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                  <TableHead className="text-right">Time</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell>
                      <StatusBadge status={event.status} />
                    </TableCell>
                    <TableCell className="font-mono font-medium">
                      {event.model}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {event.provider}
                    </TableCell>
                    {selectedTeam?.my_role === "owner" ? (
                      <TableCell
                        className="max-w-44 truncate font-mono text-xs text-muted-foreground"
                        title={event.user_id}
                      >
                        {event.user_id}
                      </TableCell>
                    ) : null}
                    <TableCell className="text-right font-mono">
                      {formatNumber(event.input_tokens)} /{" "}
                      {formatNumber(event.output_tokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {formatMicroUSD(event.actual_cost_microusd)}
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">
                      {event.created_at
                        ? new Date(event.created_at).toLocaleString()
                        : "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <p className="text-xs leading-5 text-muted-foreground">
        Showing the 25 most recent events. Historical rows created before team
        attribution was introduced may remain in personal history because they
        cannot be reassigned safely.
      </p>
    </>
  );
}
