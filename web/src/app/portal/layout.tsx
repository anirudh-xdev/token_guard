import { ClerkProvider } from "@clerk/nextjs";
import type { ReactNode } from "react";
import { PortalWorkspace } from "@/components/portal/PortalWorkspace";

export default function PortalLayout({ children }: { children: ReactNode }) {
  return (
    <ClerkProvider>
      <PortalWorkspace>{children}</PortalWorkspace>
    </ClerkProvider>
  );
}
