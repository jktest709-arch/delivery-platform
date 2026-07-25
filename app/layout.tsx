import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "统一交付平台",
  description: "基于 GitLab CI 的多项目统一发布、打包与部署工作台。",
  icons: {
    icon: "/favicon.svg",
    shortcut: "/favicon.svg",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
