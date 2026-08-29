import type { ReactNode } from "react";

import { cn } from "../lib/cn.ts";
import { Button, type ButtonProps } from "./ui/button.tsx";

export function Confirmation({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
  approval?: unknown;
  state?: string;
}) {
  return (
    <div
      className={cn(
        "w-full space-y-2 rounded-lg border border-amber-700/50 bg-amber-950/20 p-3",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function ConfirmationTitle({ children }: { children: ReactNode }) {
  return <div className="text-sm font-medium text-amber-100">{children}</div>;
}

export function ConfirmationRequest({ children }: { children: ReactNode }) {
  return <div className="text-sm leading-6 text-foreground/80">{children}</div>;
}

export function ConfirmationAccepted({ children }: { children: ReactNode }) {
  return <div className="flex items-center gap-2 text-sm text-emerald-400">{children}</div>;
}

export function ConfirmationRejected({ children }: { children: ReactNode }) {
  return <div className="flex items-center gap-2 text-sm text-destructive">{children}</div>;
}

export function ConfirmationActions({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap gap-2">{children}</div>;
}

export function ConfirmationAction({ children, ...props }: ButtonProps) {
  return (
    <Button size="sm" {...props}>
      {children}
    </Button>
  );
}
