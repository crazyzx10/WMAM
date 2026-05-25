import type { ReactNode } from "react";
import { Inbox } from "lucide-react";
import { cn } from "../../lib/cn";

export function EmptyState({
  title,
  description,
  icon,
  className
}: {
  title: string;
  description?: string;
  icon?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center justify-center px-6 py-10 text-center", className)}>
      <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-md border border-border bg-muted text-muted-foreground">
        {icon ?? <Inbox className="h-5 w-5" />}
      </div>
      <div className="text-sm font-medium">{title}</div>
      {description ? <div className="mt-1 max-w-[360px] text-sm text-muted-foreground">{description}</div> : null}
    </div>
  );
}
