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
  budget_usd: number;
  spent_usd: number;
  available_usd: number;
  my_role: string;
  my_cap_usd: number;
  my_spent_usd: number;
  my_available_usd?: number;
  owner_email?: string;
  owner_name?: string;
  invited_by_email?: string;
  invited_by_name?: string;
  invited_at?: string;
};

export type AccountView = {
  user_id: string;
  email: string;
  name: string;
  budget_usd: number;
  spent_usd: number;
  available_usd: number;
  keys: Array<{
    id: string;
    name: string;
    key_prefix: string;
    status: string;
    created_at: string;
  }>;
  active_key_count: number;
  teams: TeamAssignment[];
};

export type TeamMember = {
  user_id: string;
  email: string;
  role: string;
  cap_usd: number;
  spent_usd: number;
  invited_by_email?: string;
  invited_at?: string;
};

export type TeamInvite = {
  id: string;
  team_id: string;
  team_name: string;
  email: string;
  cap_usd: number;
  invited_by_email: string;
  status: string;
  created_at?: string;
};

export type PortalUsageEvent = {
  id: string;
  user_id: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  actual_cost_microusd: number;
  status: string;
  created_at?: string;
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

export async function tgFetch<T>(
  path: string,
  token: string | null,
  init?: RequestInit,
): Promise<{ ok: boolean; status: number; data: T & { error?: string } }> {
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
  let data = {} as T & { error?: string };
  try {
    data = text ? (JSON.parse(text) as T & { error?: string }) : ({} as T & { error?: string });
  } catch {
    data = { error: text } as T & { error?: string };
  }
  return { ok: res.ok, status: res.status, data };
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
