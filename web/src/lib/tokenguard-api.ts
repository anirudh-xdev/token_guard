/** Browser → Go TokenGuard API base (no trailing slash). */
export function apiBaseUrl(): string {
  const raw =
    process.env.NEXT_PUBLIC_TOKENGUARD_API_URL?.trim() ||
    process.env.NEXT_PUBLIC_DEMO_URL?.trim() ||
    "http://127.0.0.1:8080";
  return raw.replace(/\/+$/, "");
}

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
  teams: Array<{
    id: string;
    name: string;
    budget_usd: number;
    spent_usd: number;
    available_usd: number;
    my_role: string;
    my_cap_usd: number;
    my_spent_usd: number;
  }>;
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
