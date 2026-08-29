"use client";

import type { FormEvent, HTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";

import { cn } from "../lib/cn.ts";
import { Button } from "./ui/button.tsx";

export type PromptInputMessage = { text: string };

export function PromptInput({
  className,
  onSend,
  children,
  ...props
}: Omit<HTMLAttributes<HTMLFormElement>, "onSubmit"> & {
  onSend?: (message: PromptInputMessage) => void;
}) {
  return (
    <form
      className={cn(
        "rounded-xl border border-border bg-accent/80 shadow-inner",
        className,
      )}
      onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const form = event.currentTarget;
        const data = new FormData(form);
        const text = String(data.get("message") ?? "").trim();
        if (!text) {
          return;
        }
        onSend?.({ text });
      }}
      {...props}
    >
      {children}
    </form>
  );
}

export function PromptInputTextarea({
  className,
  ...props
}: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      name="message"
      rows={3}
      className={cn(
        "field-sizing-content w-full resize-none bg-transparent px-3 pt-3 text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground/50",
        className,
      )}
      {...props}
    />
  );
}

export function PromptInputFooter({
  className,
  children,
}: {
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn("flex items-center justify-between gap-2 px-2 pb-2", className)}>
      {children}
    </div>
  );
}

export function PromptInputTools({ children }: { children: ReactNode }) {
  return <div className="flex items-center gap-2">{children}</div>;
}

export function PromptInputSubmit({
  status,
  disabled,
}: {
  status?: "ready" | "streaming";
  disabled?: boolean;
}) {
  return (
    <Button type="submit" disabled={disabled || status === "streaming"}>
      {status === "streaming" ? "运行中" : "发送"}
    </Button>
  );
}
