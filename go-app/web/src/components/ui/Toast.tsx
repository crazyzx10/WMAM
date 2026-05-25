import { createContext, type ReactNode, useContext, useMemo, useState } from "react";
import { AlertCircle, CheckCircle2, Info, X } from "lucide-react";
import { Button } from "./Button";

type ToastVariant = "success" | "danger" | "info";

type ToastInput = {
  title: string;
  description?: string;
  variant?: ToastVariant;
  duration?: number;
};

type ToastItem = ToastInput & {
  id: string;
  variant: ToastVariant;
  exiting?: boolean;
};

type ToastContextValue = {
  toast: (input: ToastInput) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);
const toastExitMs = 240;

function createToastId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function toastIcon(variant: ToastVariant) {
  if (variant === "success") {
    return <CheckCircle2 className="mt-0.5 h-4 w-4 text-success" />;
  }
  if (variant === "danger") {
    return <AlertCircle className="mt-0.5 h-4 w-4 text-danger" />;
  }
  return <Info className="mt-0.5 h-4 w-4 text-muted-foreground" />;
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  function dismiss(id: string) {
    setToasts((current) => current.map((item) => (item.id === id ? { ...item, exiting: true } : item)));
    window.setTimeout(() => {
      setToasts((current) => current.filter((item) => item.id !== id));
    }, toastExitMs);
  }

  const value = useMemo<ToastContextValue>(
    () => ({
      toast(input) {
        const id = createToastId();
        const item: ToastItem = {
          id,
          variant: input.variant ?? "info",
          title: input.title,
          description: input.description,
          duration: input.duration
        };
        setToasts((current) => [item, ...current].slice(0, 4));
        window.setTimeout(() => dismiss(id), input.duration ?? 3200);
      }
    }),
    []
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed right-4 top-4 z-[70] flex w-[360px] max-w-[calc(100vw-32px)] flex-col gap-3">
        {toasts.map((item) => (
          <div
            key={item.id}
            className={[
              "pointer-events-auto flex gap-3 rounded-lg border border-border bg-card p-4 text-card-foreground shadow-lg",
              item.exiting ? "animate-toast-out" : "animate-toast-in"
            ].join(" ")}
          >
            {toastIcon(item.variant)}
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium">{item.title}</div>
              {item.description ? <div className="mt-1 text-sm text-muted-foreground">{item.description}</div> : null}
            </div>
            <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" onClick={() => dismiss(item.id)}>
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used inside ToastProvider");
  }
  return context;
}
