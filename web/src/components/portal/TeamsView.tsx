"use client";

import {
  useCallback,
  useEffect,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { formatUSD } from "@/components/portal/OverviewView";
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ToneAlert } from "@/components/ui/tone-alert";
import { cn } from "@/lib/utils";
import {
  createTeamSchema,
  inviteMemberSchema,
  memberCapSchema,
  poolBudgetSchema,
  type CreateTeamInput,
  type CreateTeamValues,
  type InviteMemberInput,
  type InviteMemberValues,
  type PoolBudgetInput,
  type PoolBudgetValues,
} from "@/lib/team-form-schemas";
import {
  asArray,
  tgPortalFetch,
  type TeamInvite,
  type TeamMember,
} from "@/lib/tokenguard-api";

export function TeamsView() {
  const {
    me,
    selectedTeam,
    setScopeID,
    getToken,
    refreshMe,
    refreshOverview,
    setError,
    setNotice,
  } = usePortal();
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [invites, setInvites] = useState<TeamInvite[]>([]);
  const [busy, setBusy] = useState("");
  const [caps, setCaps] = useState<Record<string, string>>({});
  const [capErrors, setCapErrors] = useState<Record<string, string>>({});
  const [remove, setRemove] = useState<TeamMember | null>(null);
  const [poolDefault, setPoolDefault] = useState("0");

  const loadOwnerDetails = useCallback(async () => {
    if (!selectedTeam || selectedTeam.my_role !== "owner") {
      setMembers([]);
      setInvites([]);
      return;
    }
    const [memberRes, inviteRes] = await Promise.all([
      tgPortalFetch<{ members?: TeamMember[] }>(
        `/portal/api/teams/members?team_id=${encodeURIComponent(selectedTeam.id)}`,
        getToken,
      ),
      tgPortalFetch<{ invites?: TeamInvite[] }>(
        `/portal/api/teams/invites?team_id=${encodeURIComponent(selectedTeam.id)}`,
        getToken,
      ),
    ]);
    if (!memberRes.ok) throw new Error(memberRes.data.error || "Could not load members");
    if (!inviteRes.ok) throw new Error(inviteRes.data.error || "Could not load invites");
    const nextMembers = asArray(memberRes.data.members);
    setMembers(nextMembers);
    setInvites(asArray(inviteRes.data.invites));
    setPoolDefault(String(selectedTeam.budget_usd ?? 0));
    setCaps(
      Object.fromEntries(
        nextMembers.map((member) => [member.user_id, String(member.cap_usd ?? 0)]),
      ),
    );
    setCapErrors({});
  }, [getToken, selectedTeam]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadOwnerDetails().catch((cause) =>
        setError(cause instanceof Error ? cause.message : "Could not load team"),
      );
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadOwnerDetails, setError]);

  async function createTeam(values: CreateTeamValues) {
    setBusy("create-team");
    setError("");
    try {
      const { ok, data } = await tgPortalFetch<{ id?: string }>(
        "/portal/api/teams",
        getToken,
        {
          method: "POST",
          body: JSON.stringify({
            name: values.name,
            budget_usd: values.budget_usd,
          }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not create team");
      await refreshMe();
      if (data.id) setScopeID(data.id);
      setNotice("Team created. Add members and set their caps next.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not create team");
    } finally {
      setBusy("");
    }
  }

  async function savePool(values: PoolBudgetValues) {
    if (!selectedTeam) return;
    setBusy("pool");
    setError("");
    try {
      const { ok, data } = await tgPortalFetch(
        "/portal/api/teams/budget",
        getToken,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            budget_usd: values.budget_usd,
          }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not update team budget");
      await Promise.all([refreshMe(), refreshOverview()]);
      setNotice("Team budget updated.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update budget");
    } finally {
      setBusy("");
    }
  }

  async function inviteMember(values: InviteMemberValues) {
    if (!selectedTeam) return;
    setBusy("invite");
    setError("");
    try {
      const { ok, status, data } = await tgPortalFetch(
        "/portal/api/teams/members",
        getToken,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            email: values.email,
            cap_usd: values.cap_usd,
          }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not invite member");
      setNotice(
        status === 202
          ? "Invite saved. They will join automatically after signing in."
          : "Member added to the team.",
      );
      await Promise.all([loadOwnerDetails(), refreshMe(), refreshOverview()]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not invite member");
    } finally {
      setBusy("");
    }
  }

  async function saveCap(member: TeamMember) {
    if (!selectedTeam) return;
    const parsed = memberCapSchema.safeParse({
      cap_usd: caps[member.user_id] ?? String(member.cap_usd ?? 0),
    });
    if (!parsed.success) {
      const message =
        parsed.error.issues[0]?.message || "Member cap must be $0 or more.";
      setCapErrors((current) => ({ ...current, [member.user_id]: message }));
      return;
    }
    setCapErrors((current) => {
      const next = { ...current };
      delete next[member.user_id];
      return next;
    });
    setBusy(`cap:${member.user_id}`);
    setError("");
    try {
      const { ok, data } = await tgPortalFetch(
        "/portal/api/teams/members/cap",
        getToken,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            user_id: member.user_id,
            cap_usd: parsed.data.cap_usd,
          }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not update member cap");
      await Promise.all([loadOwnerDetails(), refreshMe(), refreshOverview()]);
      setNotice(`Updated ${member.email}'s cap.`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update member cap");
    } finally {
      setBusy("");
    }
  }

  async function removeMember() {
    if (!selectedTeam || !remove) return;
    setBusy(`remove:${remove.user_id}`);
    setError("");
    try {
      const { ok, data } = await tgPortalFetch(
        "/portal/api/teams/members/remove",
        getToken,
        {
          method: "POST",
          body: JSON.stringify({ team_id: selectedTeam.id, user_id: remove.user_id }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not remove member");
      setNotice(`${remove.email} was removed from the team.`);
      setRemove(null);
      await Promise.all([loadOwnerDetails(), refreshMe(), refreshOverview()]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not remove member");
    } finally {
      setBusy("");
    }
  }

  return (
    <>
      <header>
        <p className="text-xs font-semibold uppercase tracking-[0.12em] text-signal">
          Teams
        </p>
        <h1 className="mt-1 font-display text-3xl font-bold tracking-tight">
          Spend together, clearly
        </h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
          Team budgets are separate from personal spend. Your application must
          send the team header to charge a pool.
        </p>
      </header>

      <section aria-labelledby="assignments-heading" className="space-y-4">
        <h2 id="assignments-heading" className="font-display text-lg font-semibold">
          My team assignments
        </h2>
        {asArray(me.user.teams).length === 0 ? (
          <EmptyState
            title="You are not on a team yet"
            description="Create a team to manage a shared LLM budget, or ask an owner to invite your sign-in email."
          />
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {asArray(me.user.teams).map((team) => (
              <button
                key={team.id}
                type="button"
                onClick={() => setScopeID(team.id)}
                className={cn(
                  "rounded-xl bg-card p-4 text-left ring-1 ring-foreground/10 transition hover:bg-muted/60",
                  selectedTeam?.id === team.id && "ring-2 ring-signal",
                )}
              >
                <span className="flex items-start justify-between gap-3">
                  <strong className="font-display text-lg">{team.name}</strong>
                  <StatusBadge status={team.my_role} />
                </span>
                <span className="mt-3 block text-sm text-muted-foreground">
                  {team.my_role === "owner"
                    ? `${formatUSD(team.spent_usd)} of ${formatUSD(team.budget_usd)} pool spent`
                    : `${formatUSD(team.my_spent_usd)} of ${formatUSD(team.my_cap_usd)} allowance spent`}
                </span>
              </button>
            ))}
          </div>
        )}
      </section>

      {selectedTeam ? (
        selectedTeam.my_role === "owner" ? (
          <OwnerTeamPanel
            teamName={selectedTeam.name}
            poolDefault={poolDefault}
            members={members}
            invites={invites}
            caps={caps}
            setCaps={setCaps}
            capErrors={capErrors}
            setCapErrors={setCapErrors}
            busy={busy}
            onSavePool={savePool}
            onInvite={inviteMember}
            onSaveCap={saveCap}
            onRemove={setRemove}
          />
        ) : (
          <Card>
            <CardHeader>
              <CardTitle className="font-display text-xl">
                My access in {selectedTeam.name}
              </CardTitle>
              <CardDescription>
                Owner: {selectedTeam.owner_name || selectedTeam.owner_email || "Team owner"}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <dl className="grid gap-4 sm:grid-cols-3">
                <TeamMetric label="My cap" value={selectedTeam.my_cap_usd} />
                <TeamMetric label="My spend" value={selectedTeam.my_spent_usd} />
                <TeamMetric
                  label="My remaining"
                  value={
                    selectedTeam.my_available_usd ??
                    (selectedTeam.my_cap_usd ?? 0) - (selectedTeam.my_spent_usd ?? 0)
                  }
                />
              </dl>
              <ToneAlert tone="info">
                Only the owner can view the full roster, change caps, or manage the
                team pool. Your usage page shows only your activity in this team.
              </ToneAlert>
            </CardContent>
          </Card>
        )
      ) : null}

      <CreateTeamCard busy={busy === "create-team"} onCreate={createTeam} />

      <Dialog open={Boolean(remove)} onOpenChange={(open) => !open && setRemove(null)}>
        <DialogContent showCloseButton={!busy.startsWith("remove:")}>
          <DialogHeader>
            <DialogTitle>Remove team member?</DialogTitle>
            <DialogDescription>
              {remove?.email || "This member"} will lose access to the team pool.
              Historical team usage remains available to the owner.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setRemove(null)}
              disabled={Boolean(remove && busy === `remove:${remove.user_id}`)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => void removeMember()}
              disabled={Boolean(remove && busy === `remove:${remove.user_id}`)}
            >
              {remove && busy === `remove:${remove.user_id}`
                ? "Working…"
                : "Remove member"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function CreateTeamCard({
  busy,
  onCreate,
}: {
  busy: boolean;
  onCreate: (values: CreateTeamValues) => Promise<void>;
}) {
  const form = useForm<CreateTeamInput, unknown, CreateTeamValues>({
    resolver: zodResolver(createTeamSchema),
    defaultValues: { name: "", budget_usd: "100" },
    mode: "onSubmit",
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="font-display text-lg">Create a team</CardTitle>
        <CardDescription>
          You become the owner and can invite members after creation.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_12rem_auto] sm:items-start"
          onSubmit={form.handleSubmit(async (values) => {
            await onCreate(values);
            form.reset({ name: "", budget_usd: "100" });
          })}
          noValidate
        >
          <Controller
            name="name"
            control={form.control}
            render={({ field, fieldState }) => (
              <Field data-invalid={fieldState.invalid}>
                <FieldLabel htmlFor="team-name">Team name</FieldLabel>
                <Input
                  {...field}
                  id="team-name"
                  placeholder="Acme AI"
                  className="h-10"
                  aria-invalid={fieldState.invalid}
                  disabled={busy}
                />
                {fieldState.invalid ? (
                  <FieldError errors={[fieldState.error]} />
                ) : null}
              </Field>
            )}
          />
          <Controller
            name="budget_usd"
            control={form.control}
            render={({ field, fieldState }) => (
              <Field data-invalid={fieldState.invalid}>
                <FieldLabel htmlFor="team-budget">Pool budget (USD)</FieldLabel>
                <Input
                  {...field}
                  id="team-budget"
                  type="number"
                  min="0.01"
                  step="0.01"
                  className="h-10"
                  aria-invalid={fieldState.invalid}
                  disabled={busy}
                />
                {fieldState.invalid ? (
                  <FieldError errors={[fieldState.error]} />
                ) : null}
              </Field>
            )}
          />
          <Button type="submit" size="lg" className="sm:mt-6" disabled={busy}>
            {busy ? "Creating…" : "Create team"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function OwnerTeamPanel(props: {
  teamName: string;
  poolDefault: string;
  members: TeamMember[];
  invites: TeamInvite[];
  caps: Record<string, string>;
  setCaps: Dispatch<SetStateAction<Record<string, string>>>;
  capErrors: Record<string, string>;
  setCapErrors: Dispatch<SetStateAction<Record<string, string>>>;
  busy: string;
  onSavePool: (values: PoolBudgetValues) => Promise<void>;
  onInvite: (values: InviteMemberValues) => Promise<void>;
  onSaveCap: (member: TeamMember) => Promise<void>;
  onRemove: (member: TeamMember) => void;
}) {
  const poolForm = useForm<PoolBudgetInput, unknown, PoolBudgetValues>({
    resolver: zodResolver(poolBudgetSchema),
    defaultValues: { budget_usd: props.poolDefault },
    mode: "onSubmit",
  });

  const inviteForm = useForm<InviteMemberInput, unknown, InviteMemberValues>({
    resolver: zodResolver(inviteMemberSchema),
    defaultValues: { email: "", cap_usd: "10" },
    mode: "onSubmit",
  });

  useEffect(() => {
    poolForm.reset({ budget_usd: props.poolDefault });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset when server pool changes
  }, [props.poolDefault]);

  return (
    <section aria-labelledby="owner-team-heading" className="space-y-6">
      <div>
        <p className="text-xs font-semibold uppercase tracking-widest text-signal">
          Owner controls
        </p>
        <h2 id="owner-team-heading" className="mt-1 font-display text-2xl font-semibold">
          Manage {props.teamName}
        </h2>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Team pool</CardTitle>
            <CardDescription>Hard limit for the shared budget.</CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="flex items-start gap-3"
              onSubmit={poolForm.handleSubmit((values) => props.onSavePool(values))}
              noValidate
            >
              <Controller
                name="budget_usd"
                control={poolForm.control}
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid} className="flex-1">
                    <FieldLabel htmlFor="pool-budget">Budget (USD)</FieldLabel>
                    <Input
                      {...field}
                      id="pool-budget"
                      type="number"
                      min="0"
                      step="0.01"
                      className="h-10"
                      aria-invalid={fieldState.invalid}
                      disabled={props.busy === "pool"}
                    />
                    {fieldState.invalid ? (
                      <FieldError errors={[fieldState.error]} />
                    ) : null}
                  </Field>
                )}
              />
              <Button
                variant="outline"
                type="submit"
                size="lg"
                className="mt-6"
                disabled={props.busy === "pool"}
              >
                {props.busy === "pool" ? "Saving…" : "Save"}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Invite member</CardTitle>
            <CardDescription>
              Cap can be at most the pool; caps may oversubscribe the pool.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              onSubmit={inviteForm.handleSubmit(async (values) => {
                await props.onInvite(values);
                inviteForm.reset({ email: "", cap_usd: "10" });
              })}
              noValidate
            >
              <FieldGroup className="gap-3 sm:grid sm:grid-cols-[minmax(0,1fr)_8rem_auto] sm:items-start">
                <Controller
                  name="email"
                  control={inviteForm.control}
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel htmlFor="invite-email">Email</FieldLabel>
                      <Input
                        {...field}
                        id="invite-email"
                        type="email"
                        placeholder="member@company.com"
                        className="h-10"
                        aria-invalid={fieldState.invalid}
                        disabled={props.busy === "invite"}
                      />
                      {fieldState.invalid ? (
                        <FieldError errors={[fieldState.error]} />
                      ) : null}
                    </Field>
                  )}
                />
                <Controller
                  name="cap_usd"
                  control={inviteForm.control}
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel htmlFor="invite-cap">Cap (USD)</FieldLabel>
                      <Input
                        {...field}
                        id="invite-cap"
                        type="number"
                        min="0"
                        step="0.01"
                        className="h-10"
                        aria-invalid={fieldState.invalid}
                        disabled={props.busy === "invite"}
                      />
                      {fieldState.invalid ? (
                        <FieldError errors={[fieldState.error]} />
                      ) : null}
                    </Field>
                  )}
                />
                <Button
                  type="submit"
                  size="lg"
                  className="sm:mt-6"
                  disabled={props.busy === "invite"}
                >
                  {props.busy === "invite" ? "Inviting…" : "Invite"}
                </Button>
              </FieldGroup>
            </form>
          </CardContent>
        </Card>
      </div>

      {props.invites.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>Pending invites</CardTitle>
            <CardDescription>
              Waiting for the invitee to sign in with this email.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-0">
            {props.invites.map((invite, index) => (
              <div key={invite.id}>
                {index > 0 ? <Separator /> : null}
                <div className="flex flex-wrap items-center justify-between gap-3 py-3 text-sm">
                  <span className="font-medium">{invite.email}</span>
                  <Badge variant="secondary">
                    {formatUSD(invite.cap_usd)} cap · waiting
                  </Badge>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Members</CardTitle>
          <CardDescription>
            Save a cap after editing. Removing a member does not erase usage history.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Member</TableHead>
                <TableHead>Role</TableHead>
                <TableHead className="text-right">Spent</TableHead>
                <TableHead>Cap (USD)</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.members.map((member) => (
                <TableRow key={member.user_id}>
                  <TableCell>
                    <div className="font-medium">{member.name || member.email}</div>
                    <div className="text-xs text-muted-foreground">{member.email}</div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={member.role} />
                  </TableCell>
                  <TableCell className="text-right font-mono">
                    {formatUSD(member.spent_usd)}
                  </TableCell>
                  <TableCell>
                    {member.role === "owner" ? (
                      <span className="text-muted-foreground">
                        {formatUSD(member.cap_usd)}
                      </span>
                    ) : (
                      <Field
                        data-invalid={Boolean(props.capErrors[member.user_id])}
                        className="gap-1"
                      >
                        <Input
                          aria-label={`Cap for ${member.email}`}
                          type="number"
                          min="0"
                          step="0.01"
                          className="h-9 w-28"
                          value={props.caps[member.user_id] ?? String(member.cap_usd ?? 0)}
                          aria-invalid={Boolean(props.capErrors[member.user_id])}
                          onChange={(event) => {
                            props.setCaps((current) => ({
                              ...current,
                              [member.user_id]: event.target.value,
                            }));
                            props.setCapErrors((current) => {
                              const next = { ...current };
                              delete next[member.user_id];
                              return next;
                            });
                          }}
                        />
                        {props.capErrors[member.user_id] ? (
                          <FieldError>{props.capErrors[member.user_id]}</FieldError>
                        ) : null}
                      </Field>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    {member.role !== "owner" ? (
                      <span className="inline-flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => void props.onSaveCap(member)}
                          disabled={props.busy === `cap:${member.user_id}`}
                        >
                          Save
                        </Button>
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => props.onRemove(member)}
                        >
                          Remove
                        </Button>
                      </span>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </section>
  );
}

function TeamMetric({
  label,
  value,
}: {
  label: string;
  value: number | undefined | null;
}) {
  return (
    <div className="border-l-2 border-line pl-4">
      <dt className="text-xs uppercase tracking-[0.08em] text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1 font-mono text-xl">{formatUSD(value)}</dd>
    </div>
  );
}
