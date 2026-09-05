import { useEffect, useRef, useState } from "react";
import { Download, ScrollText, Trash2 } from "lucide-react";
import { getLogs, getSidecarLog, isDemoMode, clearLogs, exportDiagnostics } from "../lib/api";
import { LogEntry } from "../lib/types";
import { parseSidecarLog } from "../lib/sidecarLog";
import { logsTailChanged } from "../lib/logsTail";
import { usePoll } from "../lib/usePoll";
import { Button, Card } from "../components/ui";

const LEVEL_COLOR: Record<LogEntry["level"], string> = {
  debug: "text-slate-500 dark:text-slate-500",
  info: "text-slate-700 dark:text-slate-300",
  warn: "text-amber-600 dark:text-amber-400",
  error: "text-red-600 dark:text-red-400",
};

export default function LogsPage() {
  const [follow, setFollow] = useState(true);
  const [demo, setDemo] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);

  // 轮询合并两路日志（东哥 v0.1.4 反馈②）：GUI 环（控制面：启动/初始化/错误）
  // + sidecar.log 尾部（数据面：连接/分流/直连明细）。此前日志页只显示 GUI 环，
  // 运行期几乎无输出——「日志未显示任何信息」即此。
  const { data: freshLogs } = usePoll(async () => {
    const [guiEntries, sidecarRaw] = await Promise.all([
      getLogs(200),
      getSidecarLog(200).catch(() => ""),
    ]);
    const sidecarEntries = parseSidecarLog(sidecarRaw, 200);
    // 合并按时间排序（GUI 环与 sidecar 各自带 HH:MM:SS 时间戳），取尾部 200。
    const merged = [...guiEntries, ...sidecarEntries].sort((a, b) =>
      a.time.localeCompare(b.time),
    );
    return merged.slice(-200);
  }, 1000);

  const [logs, setLogs] = useState<LogEntry[]>([]);

  useEffect(() => {
    void isDemoMode().then(setDemo);
  }, []);

  // 从轮询数据更新本地 logs（去重：尾条相同则不更新）
  useEffect(() => {
    if (freshLogs) {
      setLogs((prev) =>
        logsTailChanged(prev, freshLogs) ? freshLogs : prev,
      );
    }
  }, [freshLogs]);

  useEffect(() => {
    if (follow && boxRef.current) {
      boxRef.current.scrollTop = boxRef.current.scrollHeight;
    }
  }, [logs, follow]);

  const onClear = async () => {
    setLogs([]);
    try {
      await clearLogs();
    } catch {
      // 后端清空失败不影响前端清空
    }
  };

  const [diagBusy, setDiagBusy] = useState(false);
  const [diagHint, setDiagHint] = useState("");
  const onExportDiag = async () => {
    setDiagBusy(true);
    setDiagHint("");
    try {
      const path = await exportDiagnostics();
      setDiagHint(path.startsWith("（演示模式）") ? path : `已导出：${path}`);
    } catch {
      setDiagHint("导出失败");
    } finally {
      setDiagBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      <Card
        title="运行日志"
        action={
          <div className="flex items-center gap-4">
            {demo && <span className="text-xs text-slate-400">演示模式</span>}
            <label className="flex cursor-pointer items-center gap-1.5 text-xs text-slate-600 dark:text-slate-300">
              <input
                type="checkbox"
                checked={follow}
                onChange={(e) => setFollow(e.target.checked)}
                className="h-3.5 w-3.5 accent-orange-500"
              />
              自动滚动
            </label>
            <Button variant="ghost" onClick={onExportDiag} loading={diagBusy} className="!px-2 !py-1 text-xs">
              <Download className="h-3.5 w-3.5" /> 导出诊断包
            </Button>
            <Button variant="ghost" onClick={onClear} className="!px-2 !py-1 text-xs">
              <Trash2 className="h-3.5 w-3.5" /> 清空
            </Button>
          </div>
        }
      >
        <div
          ref={boxRef}
          className="h-[420px] overflow-auto rounded-lg border border-slate-200 bg-slate-50 p-3 font-mono text-xs leading-5 dark:border-slate-800 dark:bg-slate-950"
        >
          {logs.length === 0 ? (
            <p className="text-slate-500 dark:text-slate-400">暂无日志</p>
          ) : (
            logs.map((l, i) => (
              <div key={i} className="whitespace-pre-wrap break-all">
                <span className="text-slate-400 dark:text-slate-500">{l.time}</span>{" "}
                <span className={`font-semibold uppercase ${LEVEL_COLOR[l.level]}`}>
                  {l.level}
                </span>{" "}
                <span className={LEVEL_COLOR[l.level]}>{l.msg}</span>
              </div>
            ))
          )}
        </div>
        <p className="mt-3 flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
          <ScrollText className="h-3.5 w-3.5" />
          每秒刷新，最多显示最近 200 条
          {diagHint && <span className="ml-auto text-emerald-600 dark:text-emerald-400">{diagHint}</span>}
        </p>
      </Card>
    </div>
  );
}
