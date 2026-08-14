import { type ReactNode } from "react";
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function EmptyState({
  title,
  description,
  action,
  className,
}: {
  title: string;
  description: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <Card className={cn("items-center text-center", className)}>
      <CardHeader className="items-center">
        <CardTitle className="font-display text-lg">{title}</CardTitle>
        <CardDescription className="max-w-md text-sm leading-6">
          {description}
        </CardDescription>
        {action ? <div className="mt-2">{action}</div> : null}
      </CardHeader>
    </Card>
  );
}
