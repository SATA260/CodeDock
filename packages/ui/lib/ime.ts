"use client";

import { useRef, type CompositionEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";

type KeyLike = KeyboardEvent | ReactKeyboardEvent;

// flag 读取事件上的布尔字段，避免 KeyLike 联合类型没有 isComposing。
function flag(event: object, name: "isComposing"): boolean {
  return name in event && Boolean((event as { isComposing?: boolean }).isComposing);
}

/** 输入法回车是在确认候选，不是提交。 */
export function isImeConfirm(event: KeyLike): boolean {
  const native = "nativeEvent" in event ? event.nativeEvent : event;
  return Boolean(
    flag(event, "isComposing") ||
      flag(native, "isComposing") ||
      event.keyCode === 229 ||
      native.keyCode === 229 ||
      event.key === "Process",
  );
}

export function useImeGuard() {
  const composing = useRef(false);

  const onCompositionStart = (_event?: CompositionEvent<HTMLElement>) => {
    composing.current = true;
  };

  const onCompositionEnd = (_event?: CompositionEvent<HTMLElement>) => {
    // compositionend 常早于确认候选的那次 Enter
    window.requestAnimationFrame(() => {
      composing.current = false;
    });
  };

  const isBlocked = (event: KeyLike) => composing.current || isImeConfirm(event);

  return { onCompositionStart, onCompositionEnd, isBlocked };
}
