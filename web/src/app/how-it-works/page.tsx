import type { Metadata } from "next";
import { RequestPath } from "@/components/RequestPath";

export const metadata: Metadata = {
  title: "How it works",
  description:
    "Guarded request lifecycle: provider resolve, budget reserve, loop check, strip headers, settle usage.",
};

export default function HowItWorksPage() {
  return (
    <div className="pt-14 sm:pt-16">
      <div className="border-b border-line bg-panel/60 site-grid">
        <div className="mx-auto max-w-6xl px-5 py-16 sm:px-8 sm:py-20">
          <p className="text-[0.7rem] uppercase tracking-[0.18em] text-signal">
            Request lifecycle
          </p>
          <h1 className="font-display mt-3 max-w-2xl text-4xl font-bold tracking-tight sm:text-5xl">
            How TokenGuard decides before upstream.
          </h1>
          <p className="mt-5 max-w-xl text-sm leading-relaxed text-muted sm:text-base">
            Every guarded call runs preflight in parallel: budget reservation and
            loop detection. Failures never reach the provider. Success strips
            control headers and settles after the response.
          </p>
        </div>
      </div>

      <div className="mx-auto max-w-6xl px-5 py-16 sm:px-8 sm:py-20">
        <RequestPath />
      </div>
    </div>
  );
}
