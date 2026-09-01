/**
 * Navigation model shared by the sidebar (md+) and the bottom nav (<md).
 * Pure data — unit-testable without jsdom.
 */

import { Activity, FileText, Globe, ScrollText, Server } from "lucide-react";

export type PageKey = "status" | "rules" | "geo" | "profiles" | "logs";

export const NAV: { key: PageKey; label: string; icon: typeof Activity }[] = [
  { key: "status", label: "状态", icon: Activity },
  { key: "rules", label: "规则", icon: FileText },
  { key: "geo", label: "GEO", icon: Globe },
  { key: "profiles", label: "配置", icon: Server },
  { key: "logs", label: "日志", icon: ScrollText },
];

export const TITLES: Record<PageKey, string> = {
  status: "状态",
  rules: "路由规则",
  geo: "GEO 数据库",
  profiles: "配置管理",
  logs: "运行日志",
};
