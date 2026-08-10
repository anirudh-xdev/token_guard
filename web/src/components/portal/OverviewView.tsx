"use client";

import Link from "next/link";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { EmptyState } from "@/components/ui/empty-state";
import { StatusBadge } from "@/components/ui/status-badge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
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

export function formatUSD(value: number | undefined | null) {
  const usd = typeof value === "number" && Number.isFinite(value) ? value : 0;
  if (usd > 0 && usd < 0.01) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}

export function formatMicroUSD(value: number | undefined | null) {
  const micro =
    typeof value === "number" && Number.isFinite(value) ? value : 0;
  return formatUSD(micro / 1_000_000);
}

export function formatNumber(value: number | undefined | null) {
  const n = typeof value === "number" && Number.isFinite(value) ? value : 0;
  return new Intl.NumberFormat().format(n);
}

export function OverviewView() {
  const { me, overview, overviewLoading, selectedTeam } = usePortal();
  if (overviewLoading || !overview) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-9 w-72" />
        <Skeleton className="h-36 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }

  const { budget, totals, scope } = overview;
  const utilization =
    budget.limit_microusd > 0
      ? (budget.spent_microusd / budget.limit_microusd) * 100
      : budget.spent_microusd > 0
        ? 100
        : 0;
  const roleLabel =
    scope.role === "owner"
      ? "Owner overview"
      : scope.role === "member"
        ? "Member overview"
        : "Personal overview";
  const maxDaily = Math.max(1, ...overview.daily.map((point) => point.cost_microusd));

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
            {roleLabel}
          </p>
          <h1 className="mt-1 font-display text-3xl font-bold tracking-tight text-text">
            {scope.name}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {scope.role === "owner"
              ? "Team-wide spend, utilization, and actions."
              : scope.role === "member"
                ? `Your activity in this team${scope.owner ? ` · owner ${scope.owner}` : ""}.`
                : "Your API spend and activity, separate from every team."}
          </p>
        </div>
        <Badge variant="secondary">Last {scope.days} days</Badge>
      </header>

      {utilization >= 100 ? (
        <ToneAlert tone="error" title="Over budget">
          This scope is over budget. New requests will be blocked until its limit
          is increased or spending is reset for a new period.
        </ToneAlert>
      ) : utilization >= 80 ? (
        <ToneAlert tone="warning" title="Near limit">
          {Math.round(utilization)}% of this budget is used. Review usage before
          the remaining allocation runs out.
        </ToneAlert>
      ) : null}

      {scope.role === "owner" && (overview.pending_invite_count || 0) > 0 ? (
        <ToneAlert tone="info" title="Pending invites">
          {overview.pending_invite_count} pending team{" "}
          {overview.pending_invite_count === 1 ? "invite needs" : "invites need"}{" "}
          attention.{" "}
          <Link href="/portal/teams" className="font-semibold">
            Review invites
          </Link>
        </ToneAlert>
      ) : null}

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-end justify-between gap-4">
          <div>
            <CardTitle className="font-display text-lg">
              {scope.role === "owner"
                ? "Team pool"
                : scope.role === "member"
                  ? "My team allowance"
                  : "Personal budget"}
            </CardTitle>
            <CardDescription>
              {formatMicroUSD(budget.available_microusd)} available
            </CardDescription>
          </div>
          <p className="font-mono text-2xl text-text">
            {formatMicroUSD(budget.spent_microusd)}
            <span className="text-sm text-muted-foreground">
              {" "}
              / {formatMicroUSD(budget.limit_microusd)}
            </span>
          </p>
        </CardHeader>
        <CardContent className="space-y-5">
          <div
            className="h-2 overflow-hidden rounded-full bg-muted"
            role="progressbar"
            aria-label="Budget utilization"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.min(100, Math.round(utilization))}
          >
            <div
              className={`h-full rounded-full ${
                utilization >= 100
                  ? "bg-danger"
                  : utilization >= 80
                    ? "bg-warn"
                    : "bg-signal"
              }`}
              style={{ width: `${Math.min(100, utilization)}%` }}
            />
          </div>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
            <Metric label="Requests" value={formatNumber(totals.requests)} />
            <Metric
              label="Tokens"
              value={formatNumber(totals.input_tokens + totals.output_tokens)}
            />
            <Metric label="Blocked" value={formatNumber(totals.blocked)} />
            <Metric
              label="Period cost"
              value={formatMicroUSD(totals.cost_microusd)}
            />
          </dl>
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(16rem,0.6fr)]">
        <Card>
          <CardHeader className="flex flex-row items-baseline justify-between gap-3">
            <CardTitle className="font-display text-lg">Spend trend</CardTitle>
            <span className="text-xs text-muted-foreground">Daily actual cost</span>
          </CardHeader>
          <CardContent>
            {overview.daily.length === 0 ? (
              <EmptyState
                title="No activity in this period"
                description={
                  scope.kind === "team"
                    ? "Send a request with this team’s X-TokenGuard-Team-ID to create team-attributed usage."
                    : "Requests made without a team header will appear here."
                }
              />
            ) : (
              <div
                className="flex h-52 items-end gap-2 border-b border-border pb-7"
                aria-label="Daily spend chart"
              >
                {overview.daily.map((point) => {
                  const height = Math.max(4, (point.cost_microusd / maxDaily) * 100);
                  return (
                    <div
                      key={point.date}
                      className="group relative flex h-full min-w-0 flex-1 items-end"
                      title={`${point.date}: ${formatMicroUSD(point.cost_microusd)}, ${point.requests} requests`}
                    >
                      <div
                        className="w-full rounded-t bg-signal/80 transition-colors group-hover:bg-signal"
                        style={{ height: `${height}%` }}
                      />
                      <span className="absolute -bottom-6 left-1/2 -translate-x-1/2 text-[0.62rem] text-muted-foreground">
                        {new Date(`${point.date}T00:00:00`).toLocaleDateString(
                          undefined,
                          { month: "short", day: "numeric" },
                        )}
                      </span>
                      <span className="sr-only">
                        {point.date}, {formatMicroUSD(point.cost_microusd)},{" "}
                        {point.requests} requests
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="font-display text-lg">Attention</CardTitle>
          </CardHeader>
          <CardContent className="space-y-0 px-0">
            <AttentionRow
              label="Budget status"
              status={
                utilization >= 100
                  ? "blocked"
                  : utilization >= 80
                    ? "near limit"
                    : "healthy"
              }
            />
            <Separator />
            <AttentionRow
              label="Provider errors"
              status={
                totals.provider_errors > 0
                  ? `${totals.provider_errors} errors`
                  : "none"
              }
            />
            {scope.kind === "personal" ? (
              <>
                <Separator />
                <AttentionRow
                  label="API keys"
                  status={`${me.user.active_key_count ?? 0} active`}
                />
              </>
            ) : null}
            {scope.role === "owner" ? (
              <>
                <Separator />
                <AttentionRow
                  label="Pending invites"
                  status={`${overview.pending_invite_count || 0} pending`}
                />
              </>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row flex-wrap items-baseline justify-between gap-3">
          <CardTitle className="font-display text-lg">Model breakdown</CardTitle>
          <Button variant="link" className="h-auto px-0 text-signal" asChild>
            <Link href="/portal/usage">View usage</Link>
          </Button>
        </CardHeader>
        <CardContent>
          {overview.breakdown.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No models used in this period.
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Model</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead className="text-right">Requests</TableHead>
                  <TableHead className="text-right">Tokens</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {overview.breakdown.map((item) => (
                  <TableRow key={`${item.provider}:${item.model}`}>
                    <TableCell className="font-mono font-medium">
                      {item.model}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {item.provider}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatNumber(item.requests)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatNumber(item.tokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono">
                      {formatMicroUSD(item.cost_microusd)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="font-display text-lg">Quick actions</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {scope.role === "owner" ? (
            <QuickLink href="/portal/teams">Manage team</QuickLink>
          ) : null}
          <QuickLink href="/portal/usage">Review usage</QuickLink>
          <QuickLink href="/portal/keys">Manage keys</QuickLink>
          <QuickLink href="/portal/integrate">
            {selectedTeam ? "Copy team integration" : "Integrate an app"}
          </QuickLink>
          <QuickLink href="/portal/faq">Setup FAQ</QuickLink>
        </CardContent>
      </Card>
    </>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-[0.08em] text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1 font-mono text-lg text-text">{value}</dd>
    </div>
  );
}

function AttentionRow({ label, status }: { label: string; status: string }) {
  return (
    <div className="flex items-center justify-between gap-3 px-(--card-spacing) py-3 text-sm">
      <span className="text-text-dim">{label}</span>
      <StatusBadge status={status} />
    </div>
  );
}

function QuickLink({
  href,
  children,
}: {
  href: string;
  children: React.ReactNode;
}) {
  return (
    <Button variant="outline" size="lg" asChild>
      <Link href={href}>{children}</Link>
    </Button>
  );
}
