import type { Metadata } from "next";
import { DashboardApp } from "@/components/DashboardApp";

export const metadata: Metadata = {
  title: "Operator console",
};

export default function DashboardPage() {
  return <DashboardApp />;
}
