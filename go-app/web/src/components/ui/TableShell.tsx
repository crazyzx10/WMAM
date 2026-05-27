import type { HTMLAttributes } from "react";
import { cn } from "../../lib/cn";

export function TableShell({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "mt-4 overflow-x-auto overflow-y-hidden rounded-lg border border-border bg-card shadow-[0_1px_1px_rgb(0_0_0/0.03)]",
        className
      )}
      {...props}
    />
  );
}
