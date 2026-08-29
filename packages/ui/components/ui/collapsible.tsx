"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../../lib/cn.ts";

type CollapsibleContextValue = {
  open: boolean;
  setOpen: (open: boolean) => void;
};

const CollapsibleContext = createContext<CollapsibleContextValue | null>(null);

export function Collapsible({
  open,
  defaultOpen = false,
  onOpenChange,
  className,
  children,
}: {
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
  children: ReactNode;
}) {
  const [uncontrolled, setUncontrolled] = useState(defaultOpen);
  const isOpen = open ?? uncontrolled;
  const setOpen = useCallback(
    (next: boolean) => {
      if (open === undefined) {
        setUncontrolled(next);
      }
      onOpenChange?.(next);
    },
    [open, onOpenChange],
  );
  const value = useMemo(() => ({ open: isOpen, setOpen }), [isOpen, setOpen]);
  return (
    <CollapsibleContext.Provider value={value}>
      <div className={className}>{children}</div>
    </CollapsibleContext.Provider>
  );
}

function useCollapsible() {
  const ctx = useContext(CollapsibleContext);
  if (!ctx) {
    throw new Error("Collapsible components must be used within Collapsible");
  }
  return ctx;
}

export function CollapsibleTrigger({
  className,
  children,
  ...props
}: HTMLAttributes<HTMLButtonElement>) {
  const { open, setOpen } = useCollapsible();
  return (
    <button
      type="button"
      aria-expanded={open}
      className={cn("w-full text-left", className)}
      onClick={() => setOpen(!open)}
      {...props}
    >
      {children}
    </button>
  );
}

export function CollapsibleContent({
  className,
  children,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  const { open } = useCollapsible();
  if (!open) {
    return null;
  }
  return (
    <div className={className} {...props}>
      {children}
    </div>
  );
}
