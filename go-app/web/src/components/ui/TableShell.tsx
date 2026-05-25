import type { HTMLAttributes } from "react";
import { cn } from "../../lib/cn";

export function TableShell({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("mt-4 overflow-x-auto overflow-y-hidden rounded-md border border-border", className)} {...props} />;
}
