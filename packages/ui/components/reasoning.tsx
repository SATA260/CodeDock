"use client";

import { BrainIcon, ChevronDownIcon } from "lucide-react";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { cn } from "../lib/cn.ts";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./ui/collapsible.tsx";

type ReasoningContextValue = {
  isStreaming: boolean;
  isOpen: boolean;
};

const ReasoningContext = createContext<ReasoningContextValue | null>(null);

export function Reasoning({
  className,
  isStreaming = false,
  defaultOpen,
  children,
}: {
  className?: string;
  isStreaming?: boolean;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen ?? isStreaming);

  useEffect(() => {
    if (isStreaming) {
      setOpen(true);
    }
  }, [isStreaming]);

  const value = useMemo(() => ({ isStreaming, isOpen: open }), [isStreaming, open]);

  return (
    <ReasoningContext.Provider value={value}>
      <Collapsible
        open={open}
        onOpenChange={setOpen}
        className={cn("w-full text-sm text-muted-foreground", className)}
      >
        {children}
      </Collapsible>
    </ReasoningContext.Provider>
  );
}

export function ReasoningTrigger({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  const ctx = useContext(ReasoningContext);
  return (
    <CollapsibleTrigger
      className={cn(
        "flex items-center gap-2 text-muted-foreground transition-colors hover:text-accent-foreground",
        className,
      )}
    >
      {children ?? (
        <>
          <BrainIcon className="size-3.5" />
          <span>{ctx?.isStreaming ? "思考中…" : "思考过程"}</span>
          <ChevronDownIcon
            className={cn("size-3.5 transition-transform", ctx?.isOpen && "rotate-180")}
          />
        </>
      )}
    </CollapsibleTrigger>
  );
}

export function ReasoningContent({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <CollapsibleContent className={cn("mt-1.5 pl-6 text-muted-foreground/80", className)}>
      {children}
    </CollapsibleContent>
  );
}
