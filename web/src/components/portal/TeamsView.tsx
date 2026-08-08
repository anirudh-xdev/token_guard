"use client";

import { useCallback, useEffect, useState } from "react";
import { usePortal } from "@/components/portal/PortalWorkspace";
import { formatUSD } from "@/components/portal/OverviewView";
import {
  Alert,
  Button,
  ConfirmDialog,
  EmptyState,
  StatusBadge,
} from "@/components/ui/PortalUI";
import {
  asArray,
  tgFetch,
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
  const [teamName, setTeamName] = useState("");
  const [teamBudget, setTeamBudget] = useState("100");
  const [pool, setPool] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteCap, setInviteCap] = useState("10");
  const [caps, setCaps] = useState<Record<string, string>>({});
  const [remove, setRemove] = useState<TeamMember | null>(null);

  const loadOwnerDetails = useCallback(async () => {
    if (!selectedTeam || selectedTeam.my_role !== "owner") {
      setMembers([]);
      setInvites([]);
      return;
    }
    const token = await getToken();
    const [memberRes, inviteRes] = await Promise.all([
      tgFetch<{ members?: TeamMember[] }>(
        `/portal/api/teams/members?team_id=${encodeURIComponent(selectedTeam.id)}`,
        token,
      ),
      tgFetch<{ invites?: TeamInvite[] }>(
        `/portal/api/teams/invites?team_id=${encodeURIComponent(selectedTeam.id)}`,
        token,
      ),
    ]);
    if (!memberRes.ok) throw new Error(memberRes.data.error || "Could not load members");
    if (!inviteRes.ok) throw new Error(inviteRes.data.error || "Could not load invites");
    const nextMembers = asArray(memberRes.data.members);
    setMembers(nextMembers);
    setInvites(asArray(inviteRes.data.invites));
    setPool(String(selectedTeam.budget_usd ?? 0));
    setCaps(
      Object.fromEntries(
        nextMembers.map((member) => [member.user_id, String(member.cap_usd ?? 0)]),
      ),
    );
  }, [getToken, selectedTeam]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadOwnerDetails().catch((cause) =>
        setError(cause instanceof Error ? cause.message : "Could not load team"),
      );
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadOwnerDetails, setError]);

  async function createTeam() {
    const budget = Number(teamBudget);
    if (!teamName.trim() || !Number.isFinite(budget) || budget <= 0) {
      setError("Enter a team name and a budget greater than $0.");
      return;
    }
    setBusy("create-team");
    setError("");
    try {
      const token = await getToken();
      const { ok, data } = await tgFetch<{ id?: string }>(
        "/portal/api/teams",
        token,
        {
          method: "POST",
          body: JSON.stringify({ name: teamName.trim(), budget_usd: budget }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not create team");
      setTeamName("");
      await refreshMe();
      if (data.id) setScopeID(data.id);
      setNotice("Team created. Add members and set their caps next.");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not create team");
    } finally {
      setBusy("");
    }
  }

  async function savePool() {
    if (!selectedTeam) return;
    const value = Number(pool);
    if (!Number.isFinite(value) || value < 0) {
      setError("Team budget must be $0 or more.");
      return;
    }
    setBusy("pool");
    setError("");
    try {
      const token = await getToken();
      const { ok, data } = await tgFetch(
        "/portal/api/teams/budget",
        token,
        {
          method: "POST",
          body: JSON.stringify({ team_id: selectedTeam.id, budget_usd: value }),
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

  async function inviteMember() {
    if (!selectedTeam) return;
    const cap = Number(inviteCap);
    if (!inviteEmail.includes("@") || !Number.isFinite(cap) || cap < 0) {
      setError("Enter a valid email and a non-negative member cap.");
      return;
    }
    setBusy("invite");
    setError("");
    try {
      const token = await getToken();
      const { ok, status, data } = await tgFetch(
        "/portal/api/teams/members",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            email: inviteEmail.trim(),
            cap_usd: cap,
          }),
        },
      );
      if (!ok) throw new Error(data.error || "Could not invite member");
      setInviteEmail("");
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
    const cap = Number(caps[member.user_id]);
    if (!Number.isFinite(cap) || cap < 0) {
      setError("Member cap must be $0 or more.");
      return;
    }
    setBusy(`cap:${member.user_id}`);
    setError("");
    try {
      const token = await getToken();
      const { ok, data } = await tgFetch(
        "/portal/api/teams/members/cap",
        token,
        {
          method: "POST",
          body: JSON.stringify({
            team_id: selectedTeam.id,
            user_id: member.user_id,
            cap_usd: cap,
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
      const token = await getToken();
      const { ok, data } = await tgFetch(
        "/portal/api/teams/members/remove",
        token,
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
        <h1 className="mt-1 font-display text-3xl font-bold">Spend together, clearly</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-muted">
          Team budgets are separate from personal spend. Your application must
          send the team header to charge a pool.
        </p>
      </header>

      <section aria-labelledby="assignments-heading">
        <h2 id="assignments-heading" className="font-display text-lg font-semibold">
          My team assignments
        </h2>
        {asArray(me.user.teams).length === 0 ? (
          <EmptyState
            title="You are not on a team yet"
            description="Create a team to manage a shared LLM budget, or ask an owner to invite your sign-in email."
          />
        ) : (
          <div className="mt-4 grid gap-px overflow-hidden rounded-lg border border-line bg-line sm:grid-cols-2">
            {asArray(me.user.teams).map((team) => (
              <button
                key={team.id}
                type="button"
                onClick={() => setScopeID(team.id)}
                className={`min-h-32 bg-panel p-4 text-left hover:bg-ink ${
                  selectedTeam?.id === team.id ? "outline-2 -outline-offset-2 outline-signal" : ""
                }`}
              >
                <span className="flex items-start justify-between gap-3">
                  <strong className="font-display text-lg">{team.name}</strong>
                  <StatusBadge status={team.my_role} />
                </span>
                <span className="mt-3 block text-sm text-muted">
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
            pool={pool}
            setPool={setPool}
            inviteEmail={inviteEmail}
            setInviteEmail={setInviteEmail}
            inviteCap={inviteCap}
            setInviteCap={setInviteCap}
            members={members}
            invites={invites}
            caps={caps}
            setCaps={setCaps}
            busy={busy}
            onSavePool={savePool}
            onInvite={inviteMember}
            onSaveCap={saveCap}
            onRemove={setRemove}
          />
        ) : (
          <section aria-labelledby="member-team-heading" className="border-t border-line pt-6">
            <h2 id="member-team-heading" className="font-display text-xl font-semibold">
              My access in {selectedTeam.name}
            </h2>
            <p className="mt-2 text-sm text-muted">
              Owner: {selectedTeam.owner_name || selectedTeam.owner_email || "Team owner"}
            </p>
            <dl className="mt-5 grid gap-4 sm:grid-cols-3">
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
            <Alert tone="info">
              Only the owner can view the full roster, change caps, or manage the
              team pool. Your usage page shows only your activity in this team.
            </Alert>
          </section>
        )
      ) : null}

      <section aria-labelledby="create-team-heading" className="border-t border-line pt-7">
        <h2 id="create-team-heading" className="font-display text-lg font-semibold">
          Create a team
        </h2>
        <p className="mt-1 text-sm text-muted">
          You become the owner and can invite members after creation.
        </p>
        <div className="mt-4 grid gap-4 sm:grid-cols-[minmax(0,1fr)_12rem_auto] sm:items-end">
          <Field id="team-name" label="Team name">
            <input id="team-name" value={teamName} onChange={(event) => setTeamName(event.target.value)} placeholder="Acme AI" />
          </Field>
          <Field id="team-budget" label="Pool budget (USD)">
            <input id="team-budget" type="number" min="0.01" step="0.01" value={teamBudget} onChange={(event) => setTeamBudget(event.target.value)} />
          </Field>
          <Button onClick={() => void createTeam()} disabled={busy === "create-team"}>
            {busy === "create-team" ? "Creating…" : "Create team"}
          </Button>
        </div>
      </section>

      <ConfirmDialog
        open={Boolean(remove)}
        title="Remove team member?"
        description={`${remove?.email || "This member"} will lose access to the team pool. Historical team usage remains available to the owner.`}
        confirmLabel="Remove member"
        busy={Boolean(remove && busy === `remove:${remove.user_id}`)}
        onClose={() => setRemove(null)}
        onConfirm={() => void removeMember()}
      />
    </>
  );
}

function OwnerTeamPanel(props: {
  teamName: string;
  pool: string;
  setPool: (value: string) => void;
  inviteEmail: string;
  setInviteEmail: (value: string) => void;
  inviteCap: string;
  setInviteCap: (value: string) => void;
  members: TeamMember[];
  invites: TeamInvite[];
  caps: Record<string, string>;
  setCaps: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  busy: string;
  onSavePool: () => Promise<void>;
  onInvite: () => Promise<void>;
  onSaveCap: (member: TeamMember) => Promise<void>;
  onRemove: (member: TeamMember) => void;
}) {
  return (
    <section aria-labelledby="owner-team-heading" className="border-t border-line pt-7">
      <p className="text-xs font-semibold uppercase tracking-[0.1em] text-signal">Owner controls</p>
      <h2 id="owner-team-heading" className="mt-1 font-display text-2xl font-semibold">
        Manage {props.teamName}
      </h2>

      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void props.onSavePool();
          }}
        >
          <h3 className="font-semibold">Team pool</h3>
          <div className="mt-3 flex items-end gap-3">
            <Field id="pool-budget" label="Budget (USD)" className="flex-1">
              <input id="pool-budget" type="number" min="0" step="0.01" value={props.pool} onChange={(event) => props.setPool(event.target.value)} />
            </Field>
            <Button variant="secondary" type="submit" disabled={props.busy === "pool"}>
              {props.busy === "pool" ? "Saving…" : "Save"}
            </Button>
          </div>
        </form>

        <form
          onSubmit={(event) => {
            event.preventDefault();
            void props.onInvite();
          }}
        >
          <h3 className="font-semibold">Invite member</h3>
          <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_8rem_auto] sm:items-end">
            <Field id="invite-email" label="Email">
              <input id="invite-email" type="email" value={props.inviteEmail} onChange={(event) => props.setInviteEmail(event.target.value)} placeholder="member@company.com" />
            </Field>
            <Field id="invite-cap" label="Cap (USD)">
              <input id="invite-cap" type="number" min="0" step="0.01" value={props.inviteCap} onChange={(event) => props.setInviteCap(event.target.value)} />
            </Field>
            <Button type="submit" disabled={props.busy === "invite"}>
              {props.busy === "invite" ? "Inviting…" : "Invite"}
            </Button>
          </div>
        </form>
      </div>

      {props.invites.length > 0 ? (
        <div className="mt-7">
          <h3 className="font-semibold">Pending invites</h3>
          <ul className="mt-3 divide-y divide-line border-y border-line">
            {props.invites.map((invite) => (
              <li key={invite.id} className="flex flex-wrap justify-between gap-3 py-3 text-sm">
                <span>{invite.email}</span>
                <span className="text-muted">{formatUSD(invite.cap_usd)} cap · waiting for sign-in</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="mt-7 overflow-x-auto">
        <h3 className="font-semibold">Members</h3>
        <table className="mt-3 w-full min-w-[44rem] border-collapse text-left text-sm">
          <thead>
            <tr className="border-b border-line text-xs uppercase tracking-[0.08em] text-muted">
              <th scope="col" className="py-3 pr-4">Member</th>
              <th scope="col" className="py-3 pr-4">Role</th>
              <th scope="col" className="py-3 pr-4 text-right">Spent</th>
              <th scope="col" className="py-3 pr-4">Cap (USD)</th>
              <th scope="col" className="py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {props.members.map((member) => (
              <tr key={member.user_id}>
                <th scope="row" className="py-3 pr-4 font-medium">
                  {member.name || member.email}
                  <span className="block text-xs font-normal text-muted">{member.email}</span>
                </th>
                <td className="py-3 pr-4"><StatusBadge status={member.role} /></td>
                <td className="py-3 pr-4 text-right font-mono">{formatUSD(member.spent_usd)}</td>
                <td className="py-3 pr-4">
                  {member.role === "owner" ? (
                    <span className="text-muted">{formatUSD(member.cap_usd)}</span>
                  ) : (
                    <input
                      aria-label={`Cap for ${member.email}`}
                      type="number"
                      min="0"
                      step="0.01"
                      className="w-28"
                      value={props.caps[member.user_id] ?? String(member.cap_usd ?? 0)}
                      onChange={(event) =>
                        props.setCaps((current) => ({ ...current, [member.user_id]: event.target.value }))
                      }
                    />
                  )}
                </td>
                <td className="py-3 text-right">
                  {member.role !== "owner" ? (
                    <span className="inline-flex gap-2">
                      <Button
                        variant="secondary"
                        onClick={() => void props.onSaveCap(member)}
                        disabled={props.busy === `cap:${member.user_id}`}
                      >
                        Save
                      </Button>
                      <Button variant="danger" onClick={() => props.onRemove(member)}>
                        Remove
                      </Button>
                    </span>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function Field({
  id,
  label,
  className = "",
  children,
}: {
  id: string;
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label htmlFor={id} className={`block text-sm ${className}`}>
      <span className="mb-1.5 block font-semibold text-text-dim">{label}</span>
      <span className="portal-field block">{children}</span>
    </label>
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
      <dt className="text-xs uppercase tracking-[0.08em] text-muted">{label}</dt>
      <dd className="mt-1 font-mono text-xl">{formatUSD(value)}</dd>
    </div>
  );
}
