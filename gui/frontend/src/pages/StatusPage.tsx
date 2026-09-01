import { useEffect, useState } from "react";
import { Database, Globe, Play, Rocket, Square } from "lucide-react";
import {
  checkUpdate,
  getAutostartEnabled,
  getStatus,
  getSystemProxyEnabled,
  getVersion,
  openExternalBrowser,
  setAutostart,
  setSystemProxy,
  start,
  stop,
} from "../lib/api";
import { AppStatus } from "../lib/types";
import { Button, Card, StatusPill, Toggle } from "../components/ui";
import { usePoll } from "../lib/usePoll";
import { useAsyncAction } from "../lib/useAsyncAction";

// 字节数格式化（B/KB/MB/GB）。
function fmtBytes(n: number | undefined): string {
  if (typeof n !== "number" || Number.isNaN(n)) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export default function StatusPage() {
  const { data: statusRaw } = usePoll(getStatus, 2000);
  // getStatus 已返回 fromStatus 归一化后的 AppStatus（camelCase），不要再
  // 包一层 fromStatus——二次归一化会丢失 initDone 等字段。首次加载前为 null，
  // 用兜底值避免渲染期空指针。
  const status: AppStatus = statusRaw ?? {
    running: false,
    listening: "127.0.0.1:40000",
    initDone: false,
    sysProxyOn: false,
  };

  const { busy, error: actionError, notice, run } = useAsyncAction();
  // proxyEnabled 跟随轮询的 status.sysProxyOn（后端每 2s 读真实系统状态）：
  // 外部软件关闭系统代理时开关自动变关。初始化时读一次兜底。
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [autostart, setAutostartState] = useState(false);
  const [autostartBusy, setAutostartBusy] = useState(false);
  const [version, setVersion] = useState("…");
  const [updateInfo, setUpdateInfo] = useState<string | null>(null);
  const [updateUrl, setUpdateUrl] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);

  useEffect(() => {
    if (statusRaw) setProxyEnabled(statusRaw.sysProxyOn);
    else getSystemProxyOnce().then(setProxyEnabled);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [statusRaw]);

  useEffect(() => {
    getAutostartEnabled().then(setAutostartState).catch(() => {});
    getVersion().then(setVersion).catch(() => {});
  }, []);

  const toggleRunning = async () => {
    await run(status.running ? "stop" : "start", async () => {
      if (status.running) await stop();
      else await start();
    });
  };

  const toggleProxy = async (v: boolean) => {
    setProxyEnabled(v);
    try {
      await setSystemProxy(v);
    } catch (e) {
      // run 不适合这里（需要回滚 toggle），手写错误处理
      run("proxy", async () => { throw e; });
      setProxyEnabled(!v);
    }
  };

  const toggleAutostart = async (v: boolean) => {
    setAutostartBusy(true);
    try {
      await setAutostart(v);
      setAutostartState(v);
    } catch {
      // 失败保持原状，按钮已禁用由 busy 控制
    } finally {
      setAutostartBusy(false);
    }
  };

  const onCheckUpdate = async () => {
    setChecking(true);
    setUpdateInfo(null);
    setUpdateUrl(null);
    try {
      const info = await checkUpdate();
      if (info.has_update) {
        setUpdateInfo(`发现新版本 ${info.tag}（当前 ${version}）`);
        setUpdateUrl(info.url || null);
      } else if (info.latest && info.latest !== "dev") {
        setUpdateInfo(`已是最新版本 ${info.latest}`);
      } else {
        setUpdateInfo("当前为开发版，无法比较版本");
      }
    } catch (e) {
      setUpdateInfo(`检查失败：${String(e)}`);
    } finally {
      setChecking(false);
    }
  };

  return (
    <div className="space-y-4">
      {notice && (
        <div className="rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
          {notice}
        </div>
      )}

      <Card
        title="运行状态"
        action={<StatusPill ok={status.running} text={status.running ? "运行中" : "已停止"} />}
      >
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <p className="text-xs text-slate-500 dark:text-slate-400">监听地址</p>
            <p className="mt-1 font-mono text-sm text-slate-900 dark:text-slate-100">
              {status.listening}
            </p>
          </div>
          <div>
            <p className="text-xs text-slate-500 dark:text-slate-400">当前配置</p>
            <p className="mt-1 break-all font-mono text-sm text-slate-900 dark:text-slate-100">
              {status.activeName || "—"}
            </p>
          </div>
          <div>
            <p className="text-xs text-slate-500 dark:text-slate-400">启动时间</p>
            <p className="mt-1 font-mono text-sm text-slate-900 dark:text-slate-100">
              {status.startedAt ?? "—"}
            </p>
          </div>
        </div>
        {status.error && (
          <p className="mt-4 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">
            {status.error}
          </p>
        )}
        <div className="mt-5 flex flex-wrap items-center gap-3">
          <Button
            onClick={toggleRunning}
            variant={status.running ? "danger" : "primary"}
            loading={busy !== null}
            disabled={!status.running && !status.initDone}
          >
            {status.running ? (
              <>
                <Square className="h-4 w-4" /> 停止
              </>
            ) : (
              <>
                <Play className="h-4 w-4" /> 启动
              </>
            )}
          </Button>
          {!status.initDone && !status.running && (
            <span className="text-xs text-amber-600 dark:text-amber-400">
              正在初始化（默认规则 / GEO 数据库下载中），完成后即可启动
            </span>
          )}
          {actionError && (
            <span className="text-sm text-red-600 dark:text-red-400">{actionError}</span>
          )}
        </div>
      </Card>

      <Card title="服务器与 GEO">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="flex items-center gap-3">
            <div className="min-w-0 flex-1">
              <p className="text-xs text-slate-500 dark:text-slate-400">服务器已配置</p>
              <div className="mt-1">
                <StatusPill ok={!!status.configured} text={status.configured ? "已配置" : "未配置"} />
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Database className="h-5 w-5 shrink-0 text-slate-400" />
            <div className="min-w-0 flex-1">
              <p className="text-xs text-slate-500 dark:text-slate-400">geosite</p>
              <div className="mt-1">
                <StatusPill ok={!!status.siteLoaded} text={status.siteLoaded ? "已加载" : "未加载"} />
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Database className="h-5 w-5 shrink-0 text-slate-400" />
            <div className="min-w-0 flex-1">
              <p className="text-xs text-slate-500 dark:text-slate-400">geoip</p>
              <div className="mt-1">
                <StatusPill ok={!!status.ipLoaded} text={status.ipLoaded ? "已加载" : "未加载"} />
              </div>
            </div>
          </div>
        </div>
        <p className="mt-4 text-xs text-slate-500 dark:text-slate-400">
          路由规则：{status.ruleCount ?? 0} 条
          {status.routeEnabled ? "（生效中）" : "（未生效）"}
          {status.sidecarOk === false ? "；sidecar 不可用" : ""}
        </p>
      </Card>

      <Card title="流量统计">
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <div className="rounded-lg bg-orange-50 p-4 dark:bg-orange-950/30">
            <p className="text-xs text-orange-700 dark:text-orange-300">走隧道（proxy）</p>
            <p className="mt-1 text-2xl font-semibold text-orange-700 dark:text-orange-300">
              {status.proxyHits ?? 0}
            </p>
          </div>
          <div className="rounded-lg bg-emerald-50 p-4 dark:bg-emerald-950/30">
            <p className="text-xs text-emerald-700 dark:text-emerald-300">直连（direct）</p>
            <p className="mt-1 text-2xl font-semibold text-emerald-700 dark:text-emerald-300">
              {status.directHits ?? 0}
            </p>
          </div>
          <div className="rounded-lg bg-red-50 p-4 dark:bg-red-950/30">
            <p className="text-xs text-red-700 dark:text-red-300">拦截（reject）</p>
            <p className="mt-1 text-2xl font-semibold text-red-700 dark:text-red-300">
              {status.rejectedHits ?? 0}
            </p>
          </div>
          <div className="rounded-lg bg-slate-100 p-4 dark:bg-slate-800">
            <p className="text-xs text-slate-600 dark:text-slate-400">上下行流量</p>
            <p className="mt-1 text-xl font-semibold text-slate-700 dark:text-slate-300">
              {fmtBytes(status.bytesSent)} / {fmtBytes(status.bytesRecv)}
            </p>
          </div>
        </div>
      </Card>

      <Card title="系统代理">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <Globe className="h-5 w-5 text-slate-400" />
            <div>
              <p className="text-sm font-medium text-slate-800 dark:text-slate-200">
                Windows 系统代理
              </p>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                将系统 HTTP/SOCKS 代理指向 {status.listening}
              </p>
            </div>
          </div>
          <Toggle checked={proxyEnabled} onChange={toggleProxy} label="系统代理" />
        </div>
      </Card>

      <Card title="开机自启">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-start gap-3">
            <Rocket className="mt-0.5 h-5 w-5 text-orange-500" />
            <div>
              <p className="text-sm font-medium">登录后自动启动</p>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                系统登录时自动运行（Windows 注册表 / macOS LaunchAgent / Linux autostart）
              </p>
            </div>
          </div>
          <Toggle checked={autostart} onChange={toggleAutostart} disabled={autostartBusy} />
        </div>
      </Card>

      <Card title="关于">
        <div className="flex items-center gap-3">
          <Rocket className="h-5 w-5 text-orange-500" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium">x-tunnel-windows {version}</p>
            <p className="text-xs text-slate-500 dark:text-slate-400">
              x-tunnel 客户端（WS/WSS 隧道）
            </p>
          </div>
          <Button variant="secondary" onClick={onCheckUpdate} disabled={checking}>
            {checking ? "检查中…" : "检查更新"}
          </Button>
        </div>
        {updateInfo && (
          <p className="mt-3 text-sm">
            <span className={updateUrl ? "text-amber-600 dark:text-amber-400" : "text-emerald-600 dark:text-emerald-400"}>
              {updateInfo}
            </span>
            {updateUrl && (
              <button
                type="button"
                onClick={() => openExternalBrowser(updateUrl)}
                className="ml-2 text-orange-600 underline dark:text-orange-400"
              >
                前往下载
              </button>
            )}
          </p>
        )}
      </Card>
    </div>
  );
}

// Small local helper to avoid inline try/catch in the component.
async function getSystemProxyOnce(): Promise<boolean> {
  try {
    return await getSystemProxyEnabled();
  } catch {
    return false;
  }
}
