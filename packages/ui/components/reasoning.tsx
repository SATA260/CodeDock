"use client";

import { BrainIcon, ChevronDownIcon } from "lucide-react";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { LiveStatus } from "./live-status.tsx";
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

// Reasoning 折叠模型思考。流式时展开，这一段结束后收回，避免和正文叠在一起。
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
  const wasStreaming = useRef(isStreaming);

  useEffect(() => {
    if (isStreaming) {
      setOpen(true);
    } else if (wasStreaming.current) {
      setOpen(false);
    }
    wasStreaming.current = isStreaming;
  }, [isStreaming]);

  const value = useMemo(() => ({ isStreaming, isOpen: open }), [isStreaming, open]);

  return (
    <ReasoningContext.Provider value={value}>
      <Collapsible
        open={open}
        onOpenChange={setOpen}
        className={cn("w-full text-sm leading-5 text-muted-foreground", className)}
      >
        {children}
      </Collapsible>
    </ReasoningContext.Provider>
  );
}

// ReasoningTrigger 是思考折叠的标题，流式时显示 Thinking。
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
          <LiveStatus active={Boolean(ctx?.isStreaming)}>
            {ctx?.isStreaming ? "Thinking" : "Reasoning"}
          </LiveStatus>
          <ChevronDownIcon
            className={cn("size-3.5 transition-transform", ctx?.isOpen && "rotate-180")}
          />
        </>
      )}
    </CollapsibleTrigger>
  );
}

// ReasoningContent 在展开后渲染思考正文。
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
