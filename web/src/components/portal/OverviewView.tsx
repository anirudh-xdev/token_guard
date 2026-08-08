"use client";

import Link from "next/link";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { Alert, EmptyState, Skeleton, StatusBadge } from "@/components/ui/PortalUI";

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
      <>
        <Skeleton className="h-9 w-72" />
        <Skeleton className="h-36 w-full" />
        <Skeleton className="h-72 w-full" />
      </>
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
          <h1 className="mt-1 font-display text-3xl font-bold text-text">
            {scope.name}
          </h1>
          <p className="mt-2 text-sm text-muted">
            {scope.role === "owner"
              ? "Team-wide spend, utilization, and actions."
              : scope.role === "member"
                ? `Your activity in this team${scope.owner ? ` · owner ${scope.owner}` : ""}.`
                : "Your API spend and activity, separate from every team."}
          </p>
        </div>
        <span className="rounded-full bg-ink-2 px-3 py-1.5 text-xs font-semibold text-text-dim">
          Last {scope.days} days
        </span>
      </header>

      {utilization >= 100 ? (
        <Alert tone="error">
          This scope is over budget. New requests will be blocked until its limit
          is increased or spending is reset for a new period.
        </Alert>
      ) : utilization >= 80 ? (
        <Alert tone="warning">
          {Math.round(utilization)}% of this budget is used. Review usage before
          the remaining allocation runs out.
        </Alert>
      ) : null}

      {scope.role === "owner" && (overview.pending_invite_count || 0) > 0 ? (
        <Alert tone="info">
          {overview.pending_invite_count} pending team{" "}
          {overview.pending_invite_count === 1 ? "invite needs" : "invites need"} attention.{" "}
          <Link href="/portal/teams" className="font-semibold underline">
            Review invites
          </Link>
        </Alert>
      ) : null}

      <section aria-labelledby="budget-heading" className="border-y border-line py-6">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2 id="budget-heading" className="font-display text-lg font-semibold">
              {scope.role === "owner"
                ? "Team pool"
                : scope.role === "member"
                  ? "My team allowance"
                  : "Personal budget"}
            </h2>
            <p className="mt-1 text-sm text-muted">
              {formatMicroUSD(budget.available_microusd)} available
            </p>
          </div>
          <p className="font-mono text-2xl text-text">
            {formatMicroUSD(budget.spent_microusd)}
            <span className="text-sm text-muted">
              {" "}
              / {formatMicroUSD(budget.limit_microusd)}
            </span>
          </p>
        </div>
        <div
          className="mt-4 h-2 overflow-hidden rounded-full bg-ink-2"
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
        <dl className="mt-5 grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
          <Metric label="Requests" value={formatNumber(totals.requests)} />
          <Metric label="Tokens" value={formatNumber(totals.input_tokens + totals.output_tokens)} />
          <Metric label="Blocked" value={formatNumber(totals.blocked)} />
          <Metric label="Period cost" value={formatMicroUSD(totals.cost_microusd)} />
        </dl>
      </section>

      <div className="grid gap-8 xl:grid-cols-[minmax(0,1.4fr)_minmax(16rem,0.6fr)]">
        <section aria-labelledby="trend-heading">
          <div className="flex items-baseline justify-between gap-3">
            <h2 id="trend-heading" className="font-display text-lg font-semibold">
              Spend trend
            </h2>
            <span className="text-xs text-muted">Daily actual cost</span>
          </div>
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
              className="mt-5 flex h-52 items-end gap-2 border-b border-line pb-7"
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
                    <span className="absolute -bottom-6 left-1/2 -translate-x-1/2 text-[0.62rem] text-muted">
                      {new Date(`${point.date}T00:00:00`).toLocaleDateString(undefined, {
                        month: "short",
                        day: "numeric",
                      })}
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
        </section>

        <section aria-labelledby="attention-heading">
          <h2 id="attention-heading" className="font-display text-lg font-semibold">
            Attention
          </h2>
          <ul className="mt-4 divide-y divide-line border-y border-line">
            <AttentionRow
              label="Budget status"
              status={utilization >= 100 ? "blocked" : utilization >= 80 ? "near limit" : "healthy"}
            />
            <AttentionRow
              label="Provider errors"
              status={totals.provider_errors > 0 ? `${totals.provider_errors} errors` : "none"}
            />
            {scope.kind === "personal" ? (
              <AttentionRow
                label="API keys"
                status={`${me.user.active_key_count ?? 0} active`}
              />
            ) : null}
            {scope.role === "owner" ? (
              <AttentionRow
                label="Pending invites"
                status={`${overview.pending_invite_count || 0} pending`}
              />
            ) : null}
          </ul>
        </section>
      </div>

      <section aria-labelledby="breakdown-heading">
        <div className="flex flex-wrap items-baseline justify-between gap-3">
          <h2 id="breakdown-heading" className="font-display text-lg font-semibold">
            Model breakdown
          </h2>
          <Link href="/portal/usage" className="text-sm font-semibold text-signal hover:underline">
            View usage
          </Link>
        </div>
        {overview.breakdown.length === 0 ? (
          <p className="mt-4 text-sm text-muted">No models used in this period.</p>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full min-w-[34rem] border-collapse text-left text-sm">
              <thead>
                <tr className="border-b border-line text-xs uppercase tracking-[0.08em] text-muted">
                  <th scope="col" className="py-3 pr-4">Model</th>
                  <th scope="col" className="py-3 pr-4">Provider</th>
                  <th scope="col" className="py-3 pr-4 text-right">Requests</th>
                  <th scope="col" className="py-3 pr-4 text-right">Tokens</th>
                  <th scope="col" className="py-3 text-right">Cost</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {overview.breakdown.map((item) => (
                  <tr key={`${item.provider}:${item.model}`}>
                    <th scope="row" className="py-3 pr-4 font-mono font-medium">{item.model}</th>
                    <td className="py-3 pr-4 text-muted">{item.provider}</td>
                    <td className="py-3 pr-4 text-right">{formatNumber(item.requests)}</td>
                    <td className="py-3 pr-4 text-right">{formatNumber(item.tokens)}</td>
                    <td className="py-3 text-right font-mono">{formatMicroUSD(item.cost_microusd)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section aria-labelledby="quick-heading" className="border-t border-line pt-6">
        <h2 id="quick-heading" className="font-display text-lg font-semibold">
          Quick actions
        </h2>
        <div className="mt-4 flex flex-wrap gap-3">
          {scope.role === "owner" ? (
            <QuickLink href="/portal/teams">Manage team</QuickLink>
          ) : null}
          <QuickLink href="/portal/usage">Review usage</QuickLink>
          <QuickLink href="/portal/keys">Manage keys</QuickLink>
          <QuickLink href="/portal/integrate">
            {selectedTeam ? "Copy team integration" : "Integrate an app"}
          </QuickLink>
        </div>
      </section>
    </>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-[0.08em] text-muted">{label}</dt>
      <dd className="mt-1 font-mono text-lg text-text">{value}</dd>
    </div>
  );
}

function AttentionRow({ label, status }: { label: string; status: string }) {
  return (
    <li className="flex items-center justify-between gap-3 py-3 text-sm">
      <span className="text-text-dim">{label}</span>
      <StatusBadge status={status} />
    </li>
  );
}

function QuickLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link
      href={href}
      className="inline-flex min-h-11 items-center rounded-md border border-line bg-panel px-4 py-2 text-sm font-semibold text-text hover:border-signal hover:text-signal"
    >
      {children}
    </Link>
  );
}
