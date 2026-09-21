"use client";

import { useCallback, useEffect, useRef, useState } from "react";

const LEFT_KEY = "codedock.column.left";
const RIGHT_KEY = "codedock.column.right";
const LEFT_OPEN_KEY = "codedock.column.leftOpen";
const RIGHT_OPEN_KEY = "codedock.column.rightOpen";
const LEFT_DEFAULT = 240;
const RIGHT_DEFAULT = 420;
export const LEFT_MIN = 168;
export const LEFT_MAX = 480;
export const RIGHT_MIN = 240;
export const RIGHT_MAX = 860;
export const MID_MIN = 280;
const SASH = 6;

type ColumnOpen = {
  left: boolean;
  right: boolean;
};

type ColumnWidths = {
  left: number;
  right: number;
};

// readStoredWidth 读本机记住的栏宽，坏值回默认。
function readStoredWidth(key: string, fallback: number, min: number, max: number): number {
  if (typeof window === "undefined") {
    return fallback;
  }
  try {
    const raw = window.localStorage.getItem(key);
    const value = raw ? Number.parseInt(raw, 10) : fallback;
    if (!Number.isFinite(value)) {
      return fallback;
    }
    return clamp(value, min, max);
  } catch {
    return fallback;
  }
}

// writeStoredWidth 把栏宽记到本机。
function writeStoredWidth(key: string, value: number): void {
  try {
    window.localStorage.setItem(key, String(Math.round(value)));
  } catch {
    // ignore
  }
}

// readStoredOpen 读本机记住的栏开合，坏值回默认。
function readStoredOpen(key: string, fallback: boolean): boolean {
  if (typeof window === "undefined") {
    return fallback;
  }
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === "0") {
      return false;
    }
    if (raw === "1") {
      return true;
    }
    return fallback;
  } catch {
    return fallback;
  }
}

// writeStoredOpen 把栏开合记到本机。
function writeStoredOpen(key: string, open: boolean): void {
  try {
    window.localStorage.setItem(key, open ? "1" : "0");
  } catch {
    // ignore
  }
}

// clamp 把数字限制在闭区间。
function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

// fitColumns 按总宽收紧仍打开的左右栏，保证中间对话还有 MID_MIN。
export function fitColumns(
  left: number,
  right: number,
  total: number,
  open: ColumnOpen = { left: true, right: true },
): ColumnWidths {
  const sashes = (open.left ? 1 : 0) + (open.right ? 1 : 0);
  const minSides = (open.left ? LEFT_MIN : 0) + (open.right ? RIGHT_MIN : 0);
  const usable = Math.max(total - SASH * sashes, minSides + MID_MIN);
  let nextLeft = open.left ? clamp(left, LEFT_MIN, LEFT_MAX) : 0;
  let nextRight = open.right ? clamp(right, RIGHT_MIN, RIGHT_MAX) : 0;
  const overflow = nextLeft + nextRight + MID_MIN - usable;
  if (overflow <= 0) {
    return { left: nextLeft, right: nextRight };
  }
  if (open.right) {
    const shrinkRight = Math.min(overflow, nextRight - RIGHT_MIN);
    nextRight -= shrinkRight;
    const remain = overflow - shrinkRight;
    if (remain > 0 && open.left) {
      nextLeft = Math.max(LEFT_MIN, nextLeft - remain);
    }
  } else if (open.left) {
    nextLeft = Math.max(LEFT_MIN, nextLeft - overflow);
  }
  return { left: nextLeft, right: nextRight };
}

// useColumnLayout 记住并拖动三栏宽度，中间栏吃剩余空间；左右栏可收起。
export function useColumnLayout() {
  const rowRef = useRef<HTMLDivElement>(null);
  const leftRef = useRef(LEFT_DEFAULT);
  const rightRef = useRef(RIGHT_DEFAULT);
  const leftOpenRef = useRef(true);
  const rightOpenRef = useRef(true);
  const [left, setLeft] = useState(LEFT_DEFAULT);
  const [right, setRight] = useState(RIGHT_DEFAULT);
  const [leftOpen, setLeftOpenState] = useState(true);
  const [rightOpen, setRightOpenState] = useState(true);

  const commit = useCallback((nextLeft: number, nextRight: number, persist: boolean) => {
    const open = { left: leftOpenRef.current, right: rightOpenRef.current };
    const sashes = (open.left ? 1 : 0) + (open.right ? 1 : 0);
    const total = rowRef.current?.clientWidth ?? nextLeft + nextRight + MID_MIN + SASH * sashes;
    const fitted = fitColumns(nextLeft, nextRight, total, open);
    if (open.left) {
      leftRef.current = fitted.left;
    }
    if (open.right) {
      rightRef.current = fitted.right;
    }
    setLeft(fitted.left);
    setRight(fitted.right);
    if (persist) {
      writeStoredWidth(LEFT_KEY, leftRef.current);
      writeStoredWidth(RIGHT_KEY, rightRef.current);
    }
  }, []);

  useEffect(() => {
    leftOpenRef.current = readStoredOpen(LEFT_OPEN_KEY, true);
    rightOpenRef.current = readStoredOpen(RIGHT_OPEN_KEY, true);
    setLeftOpenState(leftOpenRef.current);
    setRightOpenState(rightOpenRef.current);
    commit(
      readStoredWidth(LEFT_KEY, LEFT_DEFAULT, LEFT_MIN, LEFT_MAX),
      readStoredWidth(RIGHT_KEY, RIGHT_DEFAULT, RIGHT_MIN, RIGHT_MAX),
      false,
    );
  }, [commit]);

  useEffect(() => {
    const node = rowRef.current;
    if (!node || typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(() => {
      commit(leftRef.current, rightRef.current, false);
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [commit]);

  const moveLeft = useCallback(
    (delta: number, persist = false) => {
      if (!leftOpenRef.current) {
        return;
      }
      commit(leftRef.current + delta, rightRef.current, persist);
    },
    [commit],
  );

  const moveRight = useCallback(
    (delta: number, persist = false) => {
      if (!rightOpenRef.current) {
        return;
      }
      commit(leftRef.current, rightRef.current + delta, persist);
    },
    [commit],
  );

  // setLeftOpen 展开或收起会话列表，下次仍按这个开合。
  const setLeftOpen = useCallback(
    (open: boolean) => {
      if (leftOpenRef.current === open) {
        return;
      }
      leftOpenRef.current = open;
      setLeftOpenState(open);
      writeStoredOpen(LEFT_OPEN_KEY, open);
      commit(leftRef.current, rightRef.current, false);
    },
    [commit],
  );

  // setRightOpen 展开或收起右侧窗口栏，下次仍按这个开合。
  const setRightOpen = useCallback(
    (open: boolean) => {
      if (rightOpenRef.current === open) {
        return;
      }
      rightOpenRef.current = open;
      setRightOpenState(open);
      writeStoredOpen(RIGHT_OPEN_KEY, open);
      commit(leftRef.current, rightRef.current, false);
    },
    [commit],
  );

  // toggleLeft 切换会话列表开合。
  const toggleLeft = useCallback(() => {
    setLeftOpen(!leftOpenRef.current);
  }, [setLeftOpen]);

  // toggleRight 切换右侧窗口栏开合。
  const toggleRight = useCallback(() => {
    setRightOpen(!rightOpenRef.current);
  }, [setRightOpen]);

  return {
    rowRef,
    left,
    right,
    leftOpen,
    rightOpen,
    moveLeft,
    moveRight,
    setLeftOpen,
    setRightOpen,
    toggleLeft,
    toggleRight,
  };
}
