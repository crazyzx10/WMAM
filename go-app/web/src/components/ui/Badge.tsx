import type { HTMLAttributes } from "react";
import { cn } from "../../lib/cn";

type BadgeTone = "neutral" | "success" | "warning" | "danger";

const toneClasses: Record<BadgeTone, string> = {
  neutral: "border-border bg-muted/70 text-muted-foreground",
  success: "border-success/20 bg-success/10 text-success",
  warning: "border-warning/20 bg-warning/10 text-warning",
  danger: "border-danger/20 bg-danger/10 text-danger"
};

export function Badge({
  tone = "neutral",
  className,
  ...props
}: HTMLAttributes<HTMLSpanElement> & { tone?: BadgeTone }) {
  return (
    <span
      className={cn(
        "inline-flex h-6 items-center rounded-full border px-2.5 text-xs font-medium",
        toneClasses[tone],
        className
      )}
      {...props}
    />
  );
}

export function toneForStatus(status?: string): BadgeTone {
  switch (status) {
    case "active":
    case "completed":
    case "success":
    case "enabled":
      return "success";
    case "running":
    case "pending":
    case "interrupted":
    case "skipped":
      return "warning";
    case "disabled":
    case "failed":
    case "ended":
      return "danger";
    default:
      return "neutral";
  }
}
