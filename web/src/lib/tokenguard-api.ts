/** Browser → Go TokenGuard API base (no trailing slash). */
export function apiBaseUrl(): string {
  const raw = process.env.NEXT_PUBLIC_TOKENGUARD_API_URL?.trim();
  if (raw) {
    return raw.replace(/\/+$/, "");
  }
  // Never throw: static prerender / CI builds run with NODE_ENV=production and
  // may not have this env at build time. Set NEXT_PUBLIC_TOKENGUARD_API_URL in
  // the host (Vercel/etc.) for real deploys.
  return "http://127.0.0.1:8080";
}

export type TeamAssignment = {
  id: string;
  name: string;
  budget_usd?: number;
  spent_usd?: number;
  available_usd?: number;
  my_role: string;
  my_cap_usd?: number;
  my_spent_usd?: number;
  my_available_usd?: number;
  owner_email?: string;
  owner_name?: string;
  invited_by_email?: string;
  invited_by_name?: string;
  invited_at?: string;
};

export type PortalAPIKey = {
  id: string;
  name: string;
  key_prefix: string;
  status: string;
  created_at: string;
};

export type AccountView = {
  user_id: string;
  email: string;
  name: string;
  budget_usd: number;
  spent_usd: number;
  available_usd: number;
  keys: PortalAPIKey[];
  active_key_count: number;
  teams: TeamAssignment[];
};

export type TeamMember = {
  user_id: string;
  email: string;
  name?: string;
  role: string;
  cap_usd?: number;
  spent_usd?: number;
  invited_by_email?: string;
  invited_at?: string;
};

export type TeamInvite = {
  id: string;
  team_id: string;
  team_name: string;
  email: string;
  cap_usd?: number;
  invited_by_email: string;
  status: string;
  created_at?: string;
};

export type PortalUsageEvent = {
  id: string;
  user_id: string;
  team_id?: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  actual_cost_microusd: number;
  status: string;
  created_at?: string;
};

export type PortalOverview = {
  scope: {
    kind: "personal" | "team";
    id?: string;
    name: string;
    role: "personal" | "owner" | "member";
    owner?: string;
    days: number;
  };
  budget: {
    limit_microusd: number;
    spent_microusd: number;
    reserved_microusd: number;
    available_microusd: number;
  };
  totals: {
    requests: number;
    completed: number;
    blocked: number;
    provider_errors: number;
    input_tokens: number;
    output_tokens: number;
    cost_microusd: number;
  };
  daily: Array<{
    date: string;
    requests: number;
    blocked: number;
    input_tokens: number;
    output_tokens: number;
    cost_microusd: number;
  }>;
  breakdown: Array<{
    provider: string;
    model: string;
    requests: number;
    tokens: number;
    cost_microusd: number;
  }>;
  pending_invite_count?: number;
};

export type MeResponse = {
  user: AccountView;
  integration: {
    proxy_base_url: string;
    proxy_url: string;
    docs_url: string;
    discovery_url: string;
    api_key_header: string;
  };
  limits: {
    max_keys: number;
    default_budget_usd: number;
    can_create_key: boolean;
  };
};

/** Raw /portal/api/me payload before nil-slice normalization. */
type RawMeResponse = {
  user?: Partial<Omit<AccountView, "keys" | "teams">> & {
    keys?: PortalAPIKey[] | null;
    teams?: TeamAssignment[] | null;
  };
  integration?: Partial<MeResponse["integration"]>;
  limits?: Partial<MeResponse["limits"]>;
  error?: string;
};

/** Go nil slices encode as JSON null — normalize before UI use. */
export function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

export function normalizeMeResponse(raw: RawMeResponse | MeResponse): MeResponse {
  const user = raw?.user ?? {};
  const limits = raw?.limits ?? {};
  const integration = raw?.integration ?? {};
  return {
    user: {
      user_id: user.user_id || "",
      email: user.email || "",
      name: user.name || "",
      budget_usd: user.budget_usd ?? 0,
      spent_usd: user.spent_usd ?? 0,
      available_usd: user.available_usd ?? 0,
      active_key_count: user.active_key_count ?? 0,
      keys: asArray(user.keys),
      teams: asArray(user.teams),
    },
    integration: {
      proxy_base_url: integration.proxy_base_url || "",
      proxy_url: integration.proxy_url || "",
      docs_url: integration.docs_url || "",
      discovery_url: integration.discovery_url || "",
      api_key_header: integration.api_key_header || "X-TokenGuard-API-Key",
    },
    limits: {
      max_keys: limits.max_keys ?? 0,
      default_budget_usd: limits.default_budget_usd ?? 0,
      can_create_key: Boolean(limits.can_create_key),
    },
  };
}

export function normalizePortalOverview(
  raw: Partial<PortalOverview> | PortalOverview,
): PortalOverview {
  return {
    scope: {
      kind: raw?.scope?.kind === "team" ? "team" : "personal",
      id: raw?.scope?.id,
      name: raw?.scope?.name || "Personal",
      role: raw?.scope?.role || "personal",
      owner: raw?.scope?.owner,
      days: raw?.scope?.days || 30,
    },
    budget: {
      limit_microusd: raw?.budget?.limit_microusd ?? 0,
      spent_microusd: raw?.budget?.spent_microusd ?? 0,
      reserved_microusd: raw?.budget?.reserved_microusd ?? 0,
      available_microusd: raw?.budget?.available_microusd ?? 0,
    },
    totals: {
      requests: raw?.totals?.requests ?? 0,
      completed: raw?.totals?.completed ?? 0,
      blocked: raw?.totals?.blocked ?? 0,
      provider_errors: raw?.totals?.provider_errors ?? 0,
      input_tokens: raw?.totals?.input_tokens ?? 0,
      output_tokens: raw?.totals?.output_tokens ?? 0,
      cost_microusd: raw?.totals?.cost_microusd ?? 0,
    },
    daily: asArray(raw?.daily),
    breakdown: asArray(raw?.breakdown),
    pending_invite_count: raw?.pending_invite_count ?? 0,
  };
}

export type PortalTokenGetter = (options?: {
  skipCache?: boolean;
}) => Promise<string | null>;

/** Clerk session JWTs are short-lived (~60s). Prefer a fresh token when needed. */
export async function getPortalToken(
  getToken: PortalTokenGetter,
  opts?: { skipCache?: boolean },
): Promise<string> {
  const token = await getToken(opts?.skipCache ? { skipCache: true } : undefined);
  if (!token) {
    throw new Error("Not signed in. Sign in again to continue.");
  }
  return token;
}

export async function tgFetch<T>(
  path: string,
  token: string | null,
  init?: RequestInit,
): Promise<{ ok: boolean; status: number; data: T & { error?: string; code?: string } }> {
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const res = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers,
  });
  const text = await res.text();
  let data = {} as T & { error?: string; code?: string };
  try {
    data = text
      ? (JSON.parse(text) as T & { error?: string; code?: string })
      : ({} as T & { error?: string; code?: string });
  } catch {
    data = { error: text } as T & { error?: string; code?: string };
  }
  return { ok: res.ok, status: res.status, data };
}

/**
 * Portal API helper that refreshes the Clerk token once on 401.
 * This covers expired ~60s session JWTs without forcing a full page reload.
 */
export async function tgPortalFetch<T>(
  path: string,
  getToken: PortalTokenGetter,
  init?: RequestInit,
): Promise<{ ok: boolean; status: number; data: T & { error?: string; code?: string } }> {
  let token = await getPortalToken(getToken);
  let result = await tgFetch<T>(path, token, init);
  if (
    result.status === 401 &&
    (result.data.code === "unauthorized" ||
      result.data.code === "session_expired" ||
      /clerk session|expired|unauthorized/i.test(result.data.error || ""))
  ) {
    token = await getPortalToken(getToken, { skipCache: true });
    result = await tgFetch<T>(path, token, init);
  }
  return result;
}

/** Operator console → Go /mgmt/* with admin secret. */
export async function mgmtFetch<T>(
  path: string,
  adminSecret: string,
  init?: RequestInit,
): Promise<{ ok: boolean; status: number; data: T & { error?: string } }> {
  const headers = new Headers(init?.headers);
  headers.set("Content-Type", "application/json");
  headers.set("X-TokenGuard-Admin-Secret", adminSecret);
  const res = await fetch(`${apiBaseUrl()}${path}`, {
    ...init,
    headers,
  });
  const text = await res.text();
  let data = {} as T & { error?: string };
  try {
    data = text ? (JSON.parse(text) as T & { error?: string }) : ({} as T & { error?: string });
  } catch {
    data = { error: text } as T & { error?: string };
  }
  return { ok: res.ok, status: res.status, data };
}

export type MgmtUser = {
  user_id: string;
  email: string;
  name?: string;
  limit_microusd: number;
  spent_microusd: number;
};

export type MgmtUsageEvent = {
  model?: string;
  user_id?: string;
  input_tokens?: number;
  output_tokens?: number;
  actual_cost_microusd?: number;
  status?: string;
};

export type MgmtPrice = {
  model_key: string;
  input_usd_per_million?: number;
  output_usd_per_million?: number;
  input_cost_per_1k?: number;
  output_cost_per_1k?: number;
};
