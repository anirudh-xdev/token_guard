"use client";

import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiBaseUrl } from "@/lib/tokenguard-api";
import { cn } from "@/lib/utils";

type FAQItem = {
  id: string;
  category: string;
  question: string;
  answer: ReactNode;
};

const categories = [
  "All",
  "Getting started",
  "Headers",
  "Errors",
  "Portal",
] as const;

const answerClassName =
  "faq-answer max-w-3xl text-sm leading-7 text-muted-foreground [&_a]:font-semibold [&_a]:text-signal [&_a]:underline [&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.8rem] [&_code]:text-text [&_li]:mt-2 [&_ol]:mt-4 [&_ol]:list-decimal [&_ol]:pl-5 [&_p+p]:mt-3 [&_strong]:font-semibold [&_strong]:text-text [&_ul]:mt-4 [&_ul]:list-disc [&_ul]:pl-5";

export function FAQView() {
  const { selectedTeam } = usePortal();
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState<(typeof categories)[number]>("All");
  const [openId, setOpenId] = useState<string>("two-keys");

  const items = useMemo<FAQItem[]>(
    () => [
      {
        id: "not-a-developer",
        category: "Getting started",
        question: "I’m not a developer. What should I do first?",
        answer: (
          <>
            <ol>
              <li>
                Create an API key on <Link href="/portal/keys">API keys</Link>{" "}
                and copy it somewhere safe.
              </li>
              <li>
                Open <Link href="/portal/integrate">Integrate</Link> and copy
                the example.
              </li>
              <li>
                Give that example to whoever builds your app / agent — or paste
                it into an AI coding assistant with your two keys kept secret.
              </li>
              <li>
                If you are on a team, ask the owner for the Team ID before
                charging the shared budget.
              </li>
            </ol>
            <p>
              You do not need session or team headers for a first smoke test.
              Add those later for agents and shared budgets.
            </p>
          </>
        ),
      },
      {
        id: "two-keys",
        category: "Getting started",
        question: "Why do I need two keys?",
        answer: (
          <>
            <p>
              TokenGuard uses <strong>two different secrets</strong> on every
              request:
            </p>
            <ul>
              <li>
                <code>X-TokenGuard-API-Key</code> — your TokenGuard key starting
                with <code>tg_</code>. Create it on{" "}
                <Link href="/portal/keys">API keys</Link>. This identifies you
                and which budget to charge.
              </li>
              <li>
                <code>Authorization: Bearer …</code> — your real provider key
                (OpenAI, OpenRouter, Anthropic, etc.). TokenGuard forwards this
                upstream so the model can run.
              </li>
            </ul>
            <p>
              If either is missing, the request fails. The TokenGuard key is not
              a replacement for the provider key.
            </p>
          </>
        ),
      },
      {
        id: "base-url",
        category: "Getting started",
        question: "What URL should my app call?",
        answer: (
          <>
            <p>
              Point your SDK or HTTP client at TokenGuard instead of the
              provider directly:
            </p>
            <p>
              <code>{apiBaseUrl()}/v1</code>
            </p>
            <p>
              Example chat path: <code>{apiBaseUrl()}/v1/chat/completions</code>
            </p>
            <p>
              Copy a ready-made example from{" "}
              <Link href="/portal/integrate">Integrate</Link>.
            </p>
          </>
        ),
      },
      {
        id: "model",
        category: "Getting started",
        question: "Why was my model rejected?",
        answer: (
          <>
            <p>
              TokenGuard only allows models that exist in its pricing catalog.
              Unknown models return <code>400</code> — it will not guess a
              price.
            </p>
            <p>
              Use a model your operator has priced, or ask them to sync /
              upsert pricing. Check Integrate for a known-good model name.
            </p>
          </>
        ),
      },
      {
        id: "session-id",
        category: "Headers",
        question: "Where does the session id come from?",
        answer: (
          <>
            <p>
              <strong>You create it in your app.</strong> TokenGuard does not
              generate or rotate <code>X-TokenGuard-Session-ID</code> for you.
            </p>
            <ul>
              <li>
                When a user starts one agent task / conversation, create{" "}
                <code>sessionId = crypto.randomUUID()</code> and store it on
                that run’s state.
              </li>
              <li>
                Send that same value on every model call during that run.
              </li>
              <li>
                Only create a new id when the user starts a brand-new
                independent run.
              </li>
            </ul>
            <p>
              <strong>Common mistake:</strong> calling{" "}
              <code>crypto.randomUUID()</code> inside every request helper.
              Then every call looks like a new session and loop protection
              never works.
            </p>
          </>
        ),
      },
      {
        id: "team-id",
        category: "Headers",
        question: "What is Team ID and when do I send it?",
        answer: (
          <>
            <p>
              A Team ID is the id of a shared budget pool (example:{" "}
              <code>team_…</code>). Find it under{" "}
              <Link href="/portal/teams">Teams</Link>, or select that team in
              the scope switcher and open Integrate — the snippet will include
              it.
            </p>
            {selectedTeam ? (
              <p>
                Currently selected team: <strong>{selectedTeam.name}</strong> →{" "}
                <code>{selectedTeam.id}</code>
              </p>
            ) : null}
            <ul>
              <li>
                <strong>No team header</strong> → charges your personal budget.
              </li>
              <li>
                <strong>
                  <code>X-TokenGuard-Team-ID</code>
                </strong>{" "}
                → charges that team pool (and your member cap).
              </li>
            </ul>
            <p>
              Selecting a team in the portal only changes what you see. Your
              app must still send the header to charge the team.
            </p>
          </>
        ),
      },
      {
        id: "provider",
        category: "Headers",
        question: "How do I choose the provider (OpenAI, OpenRouter, …)?",
        answer: (
          <>
            <p>
              Your provider credential still goes in{" "}
              <code>Authorization</code> (or the provider’s normal auth
              header). TokenGuard routes by path/provider config on the server.
            </p>
            <p>
              Some deployments also accept{" "}
              <code>X-TokenGuard-Provider</code> with values like{" "}
              <code>openai</code>, <code>openrouter</code>, or{" "}
              <code>anthropic</code>.
            </p>
            <p>
              Most beginners only need: TokenGuard base URL +{" "}
              <code>tg_</code> key + provider key + model name.
            </p>
          </>
        ),
      },
      {
        id: "401",
        category: "Errors",
        question: "I got 401 invalid / missing API key.",
        answer: (
          <>
            <ul>
              <li>
                Header name is exactly <code>X-TokenGuard-API-Key</code>
              </li>
              <li>
                Value is the full <code>tg_…</code> key from API keys (shown
                only once at creation)
              </li>
              <li>Key was not revoked</li>
              <li>
                You are calling the TokenGuard URL, not the provider URL alone
              </li>
            </ul>
          </>
        ),
      },
      {
        id: "budget-402",
        category: "Errors",
        question: "I got 402 budget exceeded. What now?",
        answer: (
          <>
            <p>
              Your personal or team allowance is used up (or this request would
              exceed what remains).
            </p>
            <ul>
              <li>Check Overview for spent vs limit in the selected scope.</li>
              <li>Personal budget: ask an operator to raise your limit.</li>
              <li>
                Team spend: ask the owner to raise the pool or your member cap.
              </li>
              <li>
                Missing/wrong <code>X-TokenGuard-Team-ID</code> uses the
                personal budget.
              </li>
            </ul>
          </>
        ),
      },
      {
        id: "loop-409",
        category: "Errors",
        question: "I got 409 agent loop detected. What does that mean?",
        answer: (
          <>
            <p>
              TokenGuard saw the same session id sending nearly the same
              request too many times in a short window (default: 3). It blocked
              the call to stop runaway agent spend.
            </p>
            <ul>
              <li>Fix the agent so it does not repeat the same step forever.</li>
              <li>
                Keep one session id for the whole run; start a new id only for
                a new run.
              </li>
              <li>
                Wait for the loop TTL to expire, or change the prompt
                meaningfully.
              </li>
            </ul>
          </>
        ),
      },
      {
        id: "who-sees-what",
        category: "Portal",
        question: "What is Personal vs Team in the portal?",
        answer: (
          <>
            <p>
              Use the <strong>Spend scope</strong> switcher in the left nav:
            </p>
            <ul>
              <li>
                <strong>Personal</strong> — your own budget and usage (requests
                without a team header).
              </li>
              <li>
                <strong>Team (owner)</strong> — team pool, member caps,
                invites, team-wide usage.
              </li>
              <li>
                <strong>Team (member)</strong> — only your allowance and your
                usage inside that team.
              </li>
            </ul>
            <p>
              Numbers on Overview always match the selected scope. Changing
              scope in the UI does not change your app until you send the team
              header.
            </p>
          </>
        ),
      },
    ],
    [selectedTeam],
  );

  const filtered = items.filter((item) => {
    if (category !== "All" && item.category !== category) return false;
    const q = query.trim().toLowerCase();
    if (!q) return true;
    return (
      item.question.toLowerCase().includes(q) ||
      item.category.toLowerCase().includes(q) ||
      item.id.includes(q)
    );
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="gap-5">
          <div className="flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
            <div className="min-w-0 max-w-2xl">
              <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
                Help
              </p>
              <CardTitle className="mt-1 font-display text-3xl font-bold tracking-tight">
                Setup FAQ
              </CardTitle>
              <CardDescription className="mt-2 text-sm leading-6">
                Session ids, team ids, provider keys, and the errors that stop a
                first successful call — written for beginners.
              </CardDescription>
            </div>

            <div className="grid w-full max-w-md shrink-0 gap-1.5">
              <Label htmlFor="faq-search">Search</Label>
              <Input
                id="faq-search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="session, team, 402, provider…"
                className="h-10"
              />
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            {categories.map((name) => {
              const active = category === name;
              return (
                <Button
                  key={name}
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  onClick={() => setCategory(name)}
                >
                  {name}
                </Button>
              );
            })}
          </div>

          <div className="flex flex-wrap items-center gap-3 text-sm">
            <span className="text-muted-foreground">
              {filtered.length} {filtered.length === 1 ? "topic" : "topics"}
            </span>
            <span className="text-muted-foreground/40" aria-hidden>
              ·
            </span>
            <Button variant="link" className="h-auto px-0 text-signal" asChild>
              <Link href="/portal/integrate">Open Integrate examples</Link>
            </Button>
            <Button
              type="button"
              variant="link"
              className="h-auto px-0 text-signal"
              onClick={() => {
                setCategory("Headers");
                setQuery("");
                setOpenId("session-id");
              }}
            >
              Jump to session id
            </Button>
          </div>
        </CardHeader>
      </Card>

      {filtered.length === 0 ? (
        <Card>
          <CardHeader className="items-center text-center">
            <CardTitle className="font-display text-lg">No topics matched</CardTitle>
            <CardDescription>
              Try a shorter search, or clear filters.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex justify-center pb-6">
            <Button
              variant="outline"
              onClick={() => {
                setQuery("");
                setCategory("All");
              }}
            >
              Clear filters
            </Button>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardContent className="pt-(--card-spacing)">
            <Accordion
              type="single"
              collapsible
              value={openId || undefined}
              onValueChange={(value) => setOpenId(value ?? "")}
              className="w-full"
            >
              {filtered.map((item) => (
                <AccordionItem key={item.id} value={item.id}>
                  <AccordionTrigger className="items-start py-4 hover:no-underline">
                    <span className="flex min-w-0 flex-1 flex-col gap-1.5 text-left">
                      <span className="text-[0.68rem] font-semibold uppercase tracking-widest text-muted-foreground">
                        {item.category}
                      </span>
                      <span
                        className={cn(
                          "font-display text-lg font-semibold leading-snug text-text",
                          openId === item.id && "text-signal",
                        )}
                      >
                        {item.question}
                      </span>
                    </span>
                  </AccordionTrigger>
                  <AccordionContent>
                    <div className={answerClassName}>{item.answer}</div>
                  </AccordionContent>
                </AccordionItem>
              ))}
            </Accordion>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
