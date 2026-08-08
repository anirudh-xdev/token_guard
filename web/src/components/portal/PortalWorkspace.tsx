"use client";

import { SignInButton, useAuth, UserButton } from "@clerk/nextjs";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  asArray,
  normalizeMeResponse,
  normalizePortalOverview,
  tgPortalFetch,
  type MeResponse,
  type PortalOverview,
  type PortalTokenGetter,
  type TeamAssignment,
} from "@/lib/tokenguard-api";
import { Alert, Button, Skeleton } from "@/components/ui/PortalUI";

type PortalContextValue = {
  me: MeResponse;
  overview: PortalOverview | null;
  overviewLoading: boolean;
  selectedTeam: TeamAssignment | null;
  scopeID: string;
  setScopeID: (id: string) => void;
  refreshMe: () => Promise<void>;
  refreshOverview: () => Promise<void>;
  getToken: PortalTokenGetter;
  error: string;
  notice: string;
  setError: (message: string) => void;
  setNotice: (message: string) => void;
};

const PortalContext = createContext<PortalContextValue | null>(null);
const scopeStorageKey = "tokenguard_portal_scope";

export function usePortal() {
  const value = useContext(PortalContext);
  if (!value) throw new Error("usePortal must be used inside PortalWorkspace");
  return value;
}

const navItems = [
  { href: "/portal", label: "Overview" },
  { href: "/portal/usage", label: "Usage" },
  { href: "/portal/keys", label: "API keys" },
  { href: "/portal/teams", label: "Teams" },
  { href: "/portal/integrate", label: "Integrate" },
];

export function PortalWorkspace({ children }: { children: ReactNode }) {
  const { getToken, isLoaded, isSignedIn } = useAuth();
  const pathname = usePathname();
  const [me, setMe] = useState<MeResponse | null>(null);
  const [overview, setOverview] = useState<PortalOverview | null>(null);
  const [loading, setLoading] = useState(true);
  const [overviewLoading, setOverviewLoading] = useState(false);
  const [scopeID, setScopeIDState] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const selectedTeam = useMemo(
    () => asArray(me?.user.teams).find((team) => team.id === scopeID) ?? null,
    [me, scopeID],
  );

  const setScopeID = useCallback((id: string) => {
    setScopeIDState(id);
    sessionStorage.setItem(scopeStorageKey, id);
    setError("");
    setNotice("");
  }, []);

  const refreshMe = useCallback(async () => {
    const { ok, data } = await tgPortalFetch<MeResponse>("/portal/api/me", getToken);
    if (!ok) throw new Error(data.error || "Could not load your account");
    const next = normalizeMeResponse(data);
    setMe(next);
    setError("");
    const stored = sessionStorage.getItem(scopeStorageKey) || "";
    if (stored && !next.user.teams.some((team) => team.id === stored)) {
      sessionStorage.removeItem(scopeStorageKey);
      setScopeIDState("");
    }
  }, [getToken]);

  const refreshOverview = useCallback(async () => {
    if (!me) return;
    setOverviewLoading(true);
    try {
      const query = scopeID
        ? `?team_id=${encodeURIComponent(scopeID)}&days=30`
        : "?days=30";
      const { ok, data } = await tgPortalFetch<PortalOverview>(
        `/portal/api/overview${query}`,
        getToken,
      );
      if (!ok) throw new Error(data.error || "Could not load this scope");
      setOverview(normalizePortalOverview(data));
      setError((current) =>
        /clerk session|expired|unauthorized|not signed in/i.test(current)
          ? ""
          : current,
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not load overview");
      setOverview(null);
    } finally {
      setOverviewLoading(false);
    }
  }, [getToken, me, scopeID]);

  useEffect(() => {
    if (!isLoaded || !isSignedIn) return;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setLoading(true);
      const stored = sessionStorage.getItem(scopeStorageKey) || "";
      setScopeIDState(stored);
      void refreshMe()
        .catch((cause) => {
          if (!cancelled) {
            setError(
              cause instanceof Error ? cause.message : "Could not load account",
            );
          }
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [isLoaded, isSignedIn, refreshMe]);

  useEffect(() => {
    if (!me) return;
    let cancelled = false;
    const timer = window.setTimeout(() => {
      if (!cancelled) void refreshOverview();
    }, 0);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [me, refreshOverview]);

  if (!isLoaded || (isSignedIn && loading)) {
    return (
      <main className="mx-auto max-w-6xl px-5 py-12 sm:px-8">
        <span className="sr-only">Loading your TokenGuard workspace</span>
        <Skeleton className="h-8 w-56" />
        <Skeleton className="mt-8 h-40 w-full" />
        <Skeleton className="mt-4 h-64 w-full" />
      </main>
    );
  }

  if (!isSignedIn) {
    return (
      <main className="grid min-h-screen place-items-center px-5 py-16">
        <div className="max-w-md text-center">
          <p className="text-xs font-semibold uppercase tracking-[0.14em] text-signal">
            TokenGuard workspace
          </p>
          <h1 className="mt-3 font-display text-3xl font-bold text-text">
            Control LLM spend before it happens
          </h1>
          <p className="mt-3 text-sm leading-6 text-muted">
            Sign in to manage personal API access, team budgets, and usage.
          </p>
          <SignInButton mode="modal">
            <Button className="mt-7">Sign in</Button>
          </SignInButton>
        </div>
      </main>
    );
  }

  if (!me) {
    const authProblem = /clerk session|expired|not signed in|unauthorized/i.test(
      error,
    );
    return (
      <main className="mx-auto max-w-xl px-5 py-16">
        <Alert tone="error">
          {error || "Your workspace could not be loaded. Refresh and try again."}
        </Alert>
        <div className="mt-5 flex flex-wrap gap-3">
          <Button
            onClick={() => {
              setLoading(true);
              void refreshMe()
                .catch((cause) =>
                  setError(
                    cause instanceof Error
                      ? cause.message
                      : "Could not load account",
                  ),
                )
                .finally(() => setLoading(false));
            }}
          >
            Retry
          </Button>
          {authProblem ? (
            <SignInButton mode="modal">
              <Button variant="secondary">Sign in again</Button>
            </SignInButton>
          ) : null}
        </div>
      </main>
    );
  }

  const context: PortalContextValue = {
    me,
    overview,
    overviewLoading,
    selectedTeam,
    scopeID,
    setScopeID,
    refreshMe,
    refreshOverview,
    getToken,
    error,
    notice,
    setError,
    setNotice,
  };

  return (
    <PortalContext.Provider value={context}>
      <div className="min-h-screen bg-ink">
        <header className="border-b border-line bg-panel">
          <div className="mx-auto flex max-w-7xl items-center justify-between gap-4 px-5 py-4 sm:px-8">
            <div>
              <Link href="/portal" className="font-display text-lg font-bold text-text">
                TokenGuard
              </Link>
              <p className="text-xs text-muted">Spend workspace</p>
            </div>
            <div className="flex items-center gap-3">
              <span className="hidden text-right text-xs text-muted sm:block">
                {me.user.name || me.user.email}
                <br />
                {selectedTeam
                  ? `${selectedTeam.name} · ${selectedTeam.my_role}`
                  : "Personal scope"}
              </span>
              <UserButton />
            </div>
          </div>
        </header>

        <div className="mx-auto grid max-w-7xl lg:grid-cols-[14rem_minmax(0,1fr)]">
          <aside className="border-b border-line bg-panel px-5 py-4 lg:min-h-[calc(100vh-73px)] lg:border-b-0 lg:border-r lg:px-4 lg:py-6">
            <label className="block text-xs font-semibold uppercase tracking-[0.1em] text-muted">
              Spend scope
              <select
                value={scopeID}
                onChange={(event) => setScopeID(event.target.value)}
                className="mt-2 min-h-11 w-full rounded-md border border-line bg-ink px-3 text-sm text-text"
              >
                <option value="">Personal</option>
                {asArray(me.user.teams).map((team) => (
                  <option key={team.id} value={team.id}>
                    {team.name} · {team.my_role}
                  </option>
                ))}
              </select>
            </label>
            <nav aria-label="Portal navigation" className="mt-4 flex gap-1 overflow-x-auto lg:flex-col">
              {navItems.map((item) => {
                const active =
                  pathname === item.href ||
                  (item.href !== "/portal" && pathname.startsWith(`${item.href}/`));
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={`min-h-11 shrink-0 rounded-md px-3 py-2.5 text-sm font-semibold ${
                      active
                        ? "bg-signal-dim text-signal"
                        : "text-text-dim hover:bg-ink-2 hover:text-text"
                    }`}
                  >
                    {item.label}
                  </Link>
                );
              })}
            </nav>
            <p className="mt-5 hidden text-xs leading-5 text-muted lg:block">
              Team scope changes this view only. Your app must send{" "}
              <code>X-TokenGuard-Team-ID</code> to charge a team.
            </p>
          </aside>

          <main className="min-w-0 px-5 py-7 sm:px-8 lg:px-10 lg:py-9">
            <div className="mx-auto max-w-5xl space-y-5">
              {error ? (
                <Alert tone="error">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <span>{error}</span>
                    <Button
                      variant="secondary"
                      className="min-h-9 px-3"
                      onClick={() => {
                        setError("");
                        void refreshMe().catch((cause) =>
                          setError(
                            cause instanceof Error
                              ? cause.message
                              : "Could not refresh account",
                          ),
                        );
                      }}
                    >
                      Retry
                    </Button>
                  </div>
                </Alert>
              ) : null}
              {notice ? <Alert tone="success">{notice}</Alert> : null}
              {children}
            </div>
          </main>
        </div>
      </div>
    </PortalContext.Provider>
  );
}
