"use client";

import { ArrowDownIcon, MessageSquare } from "lucide-react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../lib/cn.ts";
import { Button } from "./ui/button.tsx";

/** 最新一条消息落在视口从上往下的位置。 */
const LATEST_ANCHOR = 0.7;
/** 新消息入列后再滚到锚点的时长。 */
const FOLLOW_MS = 200;
/** 流式生成时按这个间隔把最新内容拉回锚点。 */
const STREAM_FOLLOW_MS = 500;

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
  followKey,
  streaming = false,
  ...props
}: HTMLAttributes<HTMLDivElement> & {
  scrollKey?: string;
  followKey?: string;
  streaming?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const innerRef = useRef<HTMLDivElement>(null);
  const following = useRef(true);
  const seenKey = useRef<string | undefined>(undefined);
  const skipFollow = useRef(false);
  const animating = useRef(false);
  const animFrame = useRef(0);
  const wasStreaming = useRef(false);
  const [showJump, setShowJump] = useState(false);

  if (seenKey.current !== scrollKey) {
    seenKey.current = scrollKey;
    following.current = true;
    skipFollow.current = true;
  }

  const stopAnim = useCallback(() => {
    if (animFrame.current) {
      cancelAnimationFrame(animFrame.current);
      animFrame.current = 0;
    }
    animating.current = false;
  }, []);

  const applyPad = useCallback(() => {
    const el = ref.current;
    const inner = innerRef.current;
    if (!el || !inner) {
      return;
    }
    inner.style.paddingBottom = `${Math.round(el.clientHeight * (1 - LATEST_ANCHOR))}px`;
  }, []);

  const targetTop = useCallback((edge: "top" | "bottom" = "top") => {
    const el = ref.current;
    if (!el) {
      return 0;
    }
    return latestScrollTop(el, edge);
  }, []);

  const goToLatest = useCallback(
    (ms: number, edge: "top" | "bottom" = "top") => {
      const el = ref.current;
      if (!el) {
        return;
      }
      applyPad();
      const to = targetTop(edge);
      following.current = true;
      setShowJump(false);
      if (ms <= 0) {
        stopAnim();
        el.scrollTop = to;
        return;
      }
      animateScroll(el, to, ms, stopAnim, (handle) => {
        animating.current = true;
        animFrame.current = handle;
      });
    },
    [applyPad, stopAnim, targetTop],
  );

  const onScroll = useCallback(() => {
    const el = ref.current;
    if (!el || animating.current) {
      return;
    }
    const distance = Math.abs(targetTop(streaming ? "bottom" : "top") - el.scrollTop);
    following.current = distance < 96;
    setShowJump(!following.current);
  }, [streaming, targetTop]);

  useLayoutEffect(() => {
    applyPad();
    if (!skipFollow.current) {
      return;
    }
    goToLatest(0);
  });

  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    const ro = new ResizeObserver(() => {
      applyPad();
      if (following.current && !animating.current) {
        el.scrollTop = targetTop(streaming ? "bottom" : "top");
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [applyPad, streaming, targetTop]);

  useEffect(() => {
    if (skipFollow.current) {
      skipFollow.current = false;
      return;
    }
    if (!following.current || !followKey) {
      return;
    }
    let inner = 0;
    const outer = requestAnimationFrame(() => {
      inner = requestAnimationFrame(() => goToLatest(FOLLOW_MS));
    });
    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
      stopAnim();
    };
  }, [followKey, goToLatest, stopAnim]);

  useEffect(() => {
    if (!streaming) {
      return;
    }
    const timer = window.setInterval(() => {
      if (following.current) {
        goToLatest(FOLLOW_MS, "bottom");
      }
    }, STREAM_FOLLOW_MS);
    return () => window.clearInterval(timer);
  }, [goToLatest, streaming]);

  useEffect(() => {
    const ended = wasStreaming.current && !streaming;
    wasStreaming.current = streaming;
    if (!ended || !following.current) {
      return;
    }
    let inner = 0;
    const outer = requestAnimationFrame(() => {
      inner = requestAnimationFrame(() => goToLatest(FOLLOW_MS, "bottom"));
    });
    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
    };
  }, [goToLatest, streaming]);

  useEffect(() => () => stopAnim(), [stopAnim]);

  return (
    <>
      <div
        ref={ref}
        onScroll={onScroll}
        onWheel={() => {
          stopAnim();
        }}
        onPointerDown={() => {
          stopAnim();
        }}
        className="mx-auto min-h-0 w-full max-w-3xl flex-1 overflow-y-auto"
        data-conversation-scroll=""
      >
        <div ref={innerRef} className={cn("flex flex-col gap-5 px-4 py-4", className)} {...props}>
          {children}
        </div>
      </div>
      {showJump ? (
        <Button
          size="icon"
          variant="secondary"
          className="absolute bottom-4 left-1/2 z-10 -translate-x-1/2 rounded-full"
          onClick={() => goToLatest(FOLLOW_MS)}
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
          <p className="max-w-sm text-sm leading-5">{description}</p>
        </>
      )}
    </div>
  );
}

export function ConversationScrollButton() {
  return null;
}

function latestScrollTop(scroller: HTMLElement, edge: "top" | "bottom" = "top"): number {
  const inner = scroller.firstElementChild as HTMLElement | null;
  const latest =
    inner?.querySelector<HTMLElement>("[data-conversation-latest]") ??
    (inner?.lastElementChild as HTMLElement | null);
  if (!latest) {
    return Math.max(0, scroller.scrollHeight - scroller.clientHeight);
  }
  const scrollerBox = scroller.getBoundingClientRect();
  const latestBox = latest.getBoundingClientRect();
  const y =
    (edge === "bottom" ? latestBox.bottom : latestBox.top) - scrollerBox.top + scroller.scrollTop;
  const max = Math.max(0, scroller.scrollHeight - scroller.clientHeight);
  return Math.max(0, Math.min(max, y - scroller.clientHeight * LATEST_ANCHOR));
}

function animateScroll(
  el: HTMLElement,
  to: number,
  ms: number,
  onStop: () => void,
  onFrame: (handle: number) => void,
) {
  const from = el.scrollTop;
  const started = performance.now();
  const tick = (now: number) => {
    const t = Math.min(1, (now - started) / ms);
    const eased = t * (2 - t);
    el.scrollTop = from + (to - from) * eased;
    if (t < 1) {
      onFrame(requestAnimationFrame(tick));
      return;
    }
    onStop();
  };
  onFrame(requestAnimationFrame(tick));
}
