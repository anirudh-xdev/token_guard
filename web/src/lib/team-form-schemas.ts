import { z } from "zod";

function usdField(message: string) {
  return z
    .string()
    .trim()
    .min(1, message)
    .refine((value) => Number.isFinite(Number(value)), message)
    .transform((value) => Number(value));
}

export const createTeamSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Enter a team name.")
    .max(80, "Team name must be at most 80 characters."),
  budget_usd: usdField("Enter a valid USD amount.").pipe(
    z.number().gt(0, "Budget must be greater than $0."),
  ),
});

export const poolBudgetSchema = z.object({
  budget_usd: usdField("Enter a valid USD amount.").pipe(
    z.number().min(0, "Team budget must be $0 or more."),
  ),
});

export const inviteMemberSchema = z.object({
  email: z
    .string()
    .trim()
    .min(1, "Enter an email address.")
    .email("Enter a valid email address."),
  cap_usd: usdField("Enter a valid USD amount.").pipe(
    z.number().min(0, "Member cap must be $0 or more."),
  ),
});

export const memberCapSchema = z.object({
  cap_usd: usdField("Enter a valid USD amount.").pipe(
    z.number().min(0, "Member cap must be $0 or more."),
  ),
});

export type CreateTeamInput = z.input<typeof createTeamSchema>;
export type CreateTeamValues = z.output<typeof createTeamSchema>;
export type PoolBudgetInput = z.input<typeof poolBudgetSchema>;
export type PoolBudgetValues = z.output<typeof poolBudgetSchema>;
export type InviteMemberInput = z.input<typeof inviteMemberSchema>;
export type InviteMemberValues = z.output<typeof inviteMemberSchema>;
