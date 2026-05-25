import { RefreshCw } from "lucide-react";
import { Button, type ButtonProps } from "./Button";

type RefreshButtonProps = Omit<ButtonProps, "variant"> & {
  loading?: boolean;
};

export function RefreshButton({ loading = false, className, children = "刷新", ...props }: RefreshButtonProps) {
  return (
    <Button {...props} variant="outline" className={["min-w-[96px]", className].filter(Boolean).join(" ")} disabled={loading || props.disabled}>
      <RefreshCw className={["h-4 w-4", loading ? "animate-spin" : ""].join(" ")} />
      {children}
    </Button>
  );
}
