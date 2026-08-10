import { type ReactNode } from "react";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import {
  CircleAlertIcon,
  CircleCheckIcon,
  InfoIcon,
  TriangleAlertIcon,
} from "lucide-react";

const tones = {
  error: {
    className: "border-danger/30 bg-danger-dim text-danger",
    icon: CircleAlertIcon,
    variant: "destructive" as const,
  },
  success: {
    className: "border-signal/30 bg-signal-dim text-text",
    icon: CircleCheckIcon,
    variant: "default" as const,
  },
  warning: {
    className: "border-warn/30 bg-warn-dim text-text",
    icon: TriangleAlertIcon,
    variant: "default" as const,
  },
  info: {
    className: "border-info/30 bg-info-dim text-text",
    icon: InfoIcon,
    variant: "default" as const,
  },
};

export function ToneAlert({
  tone,
  title,
  children,
  className,
}: {
  tone: keyof typeof tones;
  title?: string;
  children: ReactNode;
  className?: string;
}) {
  const config = tones[tone];
  const Icon = config.icon;
  return (
    <Alert
      variant={config.variant}
      className={cn(config.className, className)}
    >
      <Icon />
      {title ? <AlertTitle>{title}</AlertTitle> : null}
      <AlertDescription className="text-current/90">{children}</AlertDescription>
    </Alert>
  );
}
