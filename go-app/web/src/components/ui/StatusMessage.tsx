type StatusMessageProps = {
  message?: string;
  error?: string;
};

export function StatusMessage({ message, error }: StatusMessageProps) {
  if (!message && !error) {
    return null;
  }

  return (
    <div
      className={[
        "rounded-md border px-3 py-2 text-sm",
        error ? "border-danger/30 bg-danger/5 text-danger" : "border-success/30 bg-success/5 text-success"
      ].join(" ")}
    >
      {error || message}
    </div>
  );
}
