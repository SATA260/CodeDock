import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";

import { Providers } from "./providers";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "CodeDock",
  description: "Local 对话工作台",
};

// RootLayout 包字体、主题和全局 Provider。
export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="zh-CN"
      className={`${geistSans.variable} ${geistMono.variable} h-full dark antialiased`}
    >
      <body className="h-full bg-background font-sans text-foreground antialiased">
        <Providers>
          <div className="h-full">{children}</div>
        </Providers>
      </body>
    </html>
  );
}
