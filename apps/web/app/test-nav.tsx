"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const items = [
  { href: "/", label: "对话", match: (path: string) => path === "/" || path.startsWith("/s/") },
  { href: "/git", label: "仓库", match: (path: string) => path === "/git" || path.startsWith("/git/") },
] as const;

export function TestNav() {
  const pathname = usePathname();

  return (
    <nav className="flex h-9 shrink-0 items-center gap-1 border-b border-border bg-background px-3 text-xs">
      <span className="mr-2 font-mono text-[10px] tracking-wider text-muted-foreground/70">
        测试
      </span>
      <span className="mr-2 h-3 w-px bg-border" aria-hidden />
      {items.map((item) => {
        const active = item.match(pathname);
        return (
          <Link
            key={item.href}
            href={item.href}
            className={
              active
                ? "rounded-md bg-muted px-2 py-1 font-medium text-foreground"
                : "rounded-md px-2 py-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            }
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
