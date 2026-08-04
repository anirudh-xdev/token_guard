"use client";

import { ClerkProvider, SignInButton, useAuth, UserButton } from "@clerk/nextjs";
import { PortalApp } from "@/components/PortalApp";

function PortalGate() {
  const { isLoaded, isSignedIn } = useAuth();

  if (!isLoaded) {
    return (
      <p className="px-5 py-16 text-center text-muted sm:px-8">Loading…</p>
    );
  }

  if (!isSignedIn) {
    return (
      <div className="mx-auto max-w-md px-5 py-16 text-center sm:px-8">
        <p className="font-mono text-[0.7rem] uppercase tracking-[0.14em] text-signal">
          TokenGuard
        </p>
        <h1 className="mt-3 font-display text-3xl font-bold text-text">
          Sign in to get an API key
        </h1>
        <p className="mt-3 text-sm text-muted">
          Frontend auth (Clerk). Keys and budgets live on the TokenGuard API —
          you never set up Turso or Redis.
        </p>
        <div className="mt-8 flex justify-center">
          <SignInButton mode="modal">
            <button
              type="button"
              className="rounded-md bg-signal px-5 py-2.5 text-sm font-semibold text-on-signal"
            >
              Sign in
            </button>
          </SignInButton>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className="absolute right-5 top-20 z-20 sm:right-8">
        <UserButton />
      </div>
      <PortalApp />
    </>
  );
}

export default function PortalPage() {
  return (
    <ClerkProvider>
      <div className="relative min-h-[70vh] pt-20">
        <PortalGate />
      </div>
    </ClerkProvider>
  );
}
