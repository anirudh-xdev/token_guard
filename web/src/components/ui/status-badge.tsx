import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export function StatusBadge({
  status,
  className,
}: {
  status: string;
  className?: string;
}) {
  const normalized = status.replaceAll("_", " ");
  const tone =
    status === "completed" ||
    status === "active" ||
    status === "healthy" ||
    status === "owner" ||
    status === "member"
      ? "ok"
      : status.startsWith("blocked") || status === "revoked"
        ? "bad"
        : "warn";

  return (
    <Badge
      variant={tone === "bad" ? "destructive" : "secondary"}
      className={cn(
        "rounded-full px-2 py-0.5 capitalize",
        tone === "ok" &&
          "border-transparent bg-signal-dim text-signal hover:bg-signal-dim",
        tone === "warn" &&
          "border-transparent bg-warn-dim text-warn hover:bg-warn-dim",
        className,
      )}
    >
      {normalized}
    </Badge>
  );
}
