"use client";

import { ArrowDownIcon, MessageSquare } from "lucide-react";
import {
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../lib/cn.ts";
import { Button } from "./ui/button.tsx";

export function Conversation({ className, children, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("relative flex min-h-0 flex-1 flex-col", className)} {...props}>
      {children}
    </div>
  );
}

export function ConversationContent({
  className,
  children,
  scrollKey,
  ...props
}: HTMLAttributes<HTMLDivElement> & { scrollKey?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const nearBottom = useRef(true);
  const seenKey = useRef(scrollKey);
  const [showJump, setShowJump] = useState(false);

  if (seenKey.current !== scrollKey) {
    seenKey.current = scrollKey;
    nearBottom.current = true;
  }

  const onScroll = useCallback(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight;
    nearBottom.current = distance < 96;
    setShowJump(!nearBottom.current);
  }, []);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || !nearBottom.current) {
      return;
    }
    el.scrollTop = el.scrollHeight;
    setShowJump(false);
  });

  return (
    <>
      <div
        ref={ref}
        onScroll={onScroll}
        className={cn(
          "mx-auto flex w-full max-w-3xl flex-1 flex-col gap-5 overflow-y-auto px-4 py-6",
          className,
        )}
        {...props}
      >
        {children}
      </div>
      {showJump ? (
        <Button
          size="icon"
          variant="secondary"
          className="absolute bottom-4 left-1/2 z-10 -translate-x-1/2 rounded-full"
          onClick={() => {
            nearBottom.current = true;
            ref.current?.scrollTo({ top: ref.current.scrollHeight, behavior: "smooth" });
            setShowJump(false);
          }}
        >
          <ArrowDownIcon className="size-4" />
        </Button>
      ) : null}
    </>
  );
}

export function ConversationEmptyState({
  className,
  title = "开始一段对话",
  description = "在下方输入消息，Agent 的思考与工具会按瀑布展开。",
  icon,
  children,
}: {
  className?: string;
  title?: string;
  description?: string;
  icon?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div
      className={cn(
        "flex flex-1 flex-col items-center justify-center gap-3 px-8 text-center text-muted-foreground",
        className,
      )}
    >
      {children ?? (
        <>
          {icon ?? <MessageSquare className="size-10 text-muted-foreground/50" />}
          <div className="text-base font-medium text-accent-foreground">{title}</div>
          <p className="max-w-sm text-sm leading-6">{description}</p>
        </>
      )}
    </div>
  );
}

export function ConversationScrollButton() {
  return null;
}
