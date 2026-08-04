import { ClerkProvider } from "@clerk/nextjs";

export default function PortalLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return <ClerkProvider>{children}</ClerkProvider>;
}
