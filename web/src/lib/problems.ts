export type Problem = {
  id: string;
  number: string;
  title: string;
  problem: string;
  when: string;
  solution: string;
  status: string;
  statusLabel: string;
};

export const problems: Problem[] = [
  {
    id: "runaway-spend",
    number: "01",
    title: "Runaway spend",
    problem:
      "LLM calls look cheap one-by-one, but an app or agent can burn hundreds of dollars before anyone notices.",
    when: "Retries, long contexts, high max_tokens, traffic spikes, or a bug that loops the API.",
    solution:
      "Before forwarding, TokenGuard estimates cost from the pricing catalog, reserves micro-USD against the user budget, and returns 402 if there isn’t enough. After the response it settles actual cost (or releases the reservation on failure).",
    status: "402",
    statusLabel: "budget",
  },
  {
    id: "agent-loops",
    number: "02",
    title: "Agent loops",
    problem:
      "Autonomous agents send the same prompt or tool payload over and over — and each attempt still costs tokens.",
    when: "Stuck planners, tool failures that retry the same step, identical session traffic past a threshold.",
    solution:
      "With X-TokenGuard-Session-ID, it hashes session + semantic payload in Redis. After the threshold (default 3), it returns 409 and does not call the provider.",
    status: "409",
    statusLabel: "loop",
  },
  {
    id: "multi-provider",
    number: "03",
    title: "Blind multi-provider cost",
    problem:
      "Teams use OpenAI + OpenRouter + Anthropic with separate dashboards and no single “who spent what” view.",
    when: "Multi-model apps, agencies, or route-this-request-to-provider-X setups.",
    solution:
      "One proxy host, X-TokenGuard-Provider routing, and one Turso ledger of usage events (tokens, cost, status) per TokenGuard user — across providers.",
    status: "ledger",
    statusLabel: "unified",
  },
  {
    id: "unpriced",
    number: "04",
    title: "Unknown / unpriced models",
    problem:
      "A new or mistyped model ID gets billed upstream with no local cost estimate, so the firewall can’t reserve safely.",
    when: "Model renames, OpenRouter IDs you never added, or deploy without updating prices.",
    solution:
      "Fail-closed: if the model isn’t in the pricing catalog, the request gets 400 pricing_not_configured and never hits the provider. Operators sync OpenRouter or upsert rates.",
    status: "400",
    statusLabel: "blocked",
  },
];
