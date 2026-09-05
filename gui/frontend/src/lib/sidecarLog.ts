// sidecar 日志行解析：sidecar.log（x-tunnel.exe stdout，标准库 log 格式）
// → 前端 LogEntry。LogsPage 轮询消费（东哥 v0.1.4 反馈②：日志页接入
// sidecar 数据面日志）。纯函数，独立成 lib 便于单测。

import { LogEntry, LogLevel } from "./types";

export function parseSidecarLine(line: string): LogEntry | null {
  const t = line.trim();
  if (!t) return null;
  const m = t.match(/^\d{4}\/\d{2}\/\d{2}\s+(\d{2}:\d{2}:\d{2})\s+(.*)$/);
  const time = m ? m[1] : "";
  const msg = m ? m[2] : t;
  const l = msg.toLowerCase();
  let level: LogLevel = "info";
  if (l.includes("error") || msg.includes("失败") || msg.includes("无法")) level = "error";
  else if (l.includes("warn") || msg.includes("⚠") || msg.includes("警告")) level = "warn";
  else if (l.includes("debug")) level = "debug";
  return { time, level, msg };
}

export function parseSidecarLog(raw: string, limit: number): LogEntry[] {
  const lines = raw.split("\n").filter((l) => l.trim());
  return lines
    .slice(-limit)
    .map(parseSidecarLine)
    .filter((e): e is LogEntry => e !== null);
}
