import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import type { ButtonHTMLAttributes } from "react";
import { cn } from "../../lib/cn";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 rounded-md text-sm font-medium transition-[background-color,border-color,color,box-shadow,opacity,transform] duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30 disabled:pointer-events-none disabled:opacity-50 active:translate-y-px",
  {
    variants: {
      variant: {
        default:
          "bg-foreground text-background shadow-[0_1px_1px_rgb(0_0_0/0.05),0_2px_2px_rgb(0_0_0/0.08)] hover:opacity-90",
        outline:
          "border border-border bg-card shadow-[0_1px_1px_rgb(0_0_0/0.03)] hover:border-muted-foreground/35 hover:bg-muted/70",
        ghost: "hover:bg-muted/70",
        danger: "border border-danger/30 bg-card text-danger hover:bg-danger/10",
        warning: "border border-warning/30 bg-card text-warning hover:bg-warning/10"
      },
      size: {
        default: "h-10 px-4",
        sm: "h-8 px-3",
        icon: "h-9 w-9"
      }
    },
    defaultVariants: {
      variant: "default",
      size: "default"
    }
  }
);

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export function Button({ className, variant, size, asChild = false, ...props }: ButtonProps) {
  const Comp = asChild ? Slot : "button";
  return <Comp className={cn(buttonVariants({ variant, size, className }))} {...props} />;
}
