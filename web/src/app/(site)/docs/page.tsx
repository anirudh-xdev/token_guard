import type { Metadata } from "next";
import { IntegratorDocs } from "@/components/IntegratorDocs";

export const metadata: Metadata = {
  title: "Docs",
  description:
    "Use TokenGuard in 5 minutes — budgets, keys, headers, and status codes.",
};

export default function DocsPage() {
  return (
    <div className="pt-14 sm:pt-16">
      <IntegratorDocs />
    </div>
  );
}
