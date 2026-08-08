"use client";

import {
  type ButtonHTMLAttributes,
  type ReactNode,
  useEffect,
  useRef,
} from "react";

export function Button({
  variant = "primary",
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "danger";
}) {
  const styles = {
    primary: "bg-signal text-on-signal hover:bg-signal-bright",
    secondary: "border border-line bg-panel text-text hover:border-signal hover:text-signal",
    danger: "border border-danger/30 bg-panel text-danger hover:bg-danger-dim",
  };
  return (
    <button
      {...props}
      className={`inline-flex min-h-11 items-center justify-center rounded-md px-4 py-2 text-sm font-semibold transition disabled:cursor-not-allowed disabled:opacity-50 ${styles[variant]} ${className}`}
    />
  );
}

export function Alert({
  tone,
  children,
}: {
  tone: "error" | "success" | "warning" | "info";
  children: ReactNode;
}) {
  const styles = {
    error: "border-danger/30 bg-danger-dim text-danger",
    success: "border-signal/30 bg-signal-dim text-text",
    warning: "border-warn/30 bg-warn-dim text-text",
    info: "border-info/30 bg-info-dim text-text",
  };
  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      aria-live="polite"
      className={`rounded-md border px-4 py-3 text-sm ${styles[tone]}`}
    >
      {children}
    </div>
  );
}

export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="border-y border-line py-10 text-center">
      <h3 className="font-display text-lg font-semibold text-text">{title}</h3>
      <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted">
        {description}
      </p>
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  );
}

export function Skeleton({ className = "" }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`block animate-pulse rounded bg-ink-2 ${className}`}
    />
  );
}

export function StatusBadge({ status }: { status: string }) {
  const tone =
    status === "completed" ||
    status === "active" ||
    status === "healthy" ||
    status === "owner" ||
    status === "member"
      ? "bg-signal-dim text-signal"
      : status.startsWith("blocked") || status === "revoked"
        ? "bg-danger-dim text-danger"
        : "bg-warn-dim text-warn";
  return (
    <span className={`rounded-full px-2 py-1 text-xs font-semibold ${tone}`}>
      {status.replaceAll("_", " ")}
    </span>
  );
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  busy,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  busy?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (open && !dialog.open) dialog.showModal();
    if (!open && dialog.open) dialog.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClose={onClose}
      className="m-auto w-[min(28rem,calc(100%-2rem))] rounded-xl border border-line bg-panel p-0 text-text backdrop:bg-text/30"
    >
      <div className="p-6">
        <h2 className="font-display text-xl font-semibold">{title}</h2>
        <p className="mt-2 text-sm leading-6 text-muted">{description}</p>
        <div className="mt-6 flex justify-end gap-3">
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="danger" onClick={onConfirm} disabled={busy}>
            {busy ? "Working…" : confirmLabel}
          </Button>
        </div>
      </div>
    </dialog>
  );
}
