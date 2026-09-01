// API bridge between the React UI and the Wails Go backend.
//
// The real backend methods live in `gui/service.go` (parallel task) and are
// surfaced to the frontend as generated TypeScript bindings under
// `frontend/bindings/gui/` (produced by `wails3 generate bindings`).
//
// Until those bindings exist, a compile-time placeholder occupies the same
// path and carries the `__MOCK_BINDINGS__` marker. This module detects the
// marker at runtime:
//   - placeholder present (standalone `npm run build`/`npm run dev`) → all
//     calls resolve to realistic demo data, so the UI is fully exercisable;
//   - real bindings present (Wails app) → calls go straight to Go.
//
// The structural `ServiceAPI` interface keeps this file decoupled from the
// exact generated typings, so regenerating bindings never breaks it.

import {
  AppProfiles,
  AppStatus,
  fromGeo,
  fromLogs,
  fromProfiles,
  fromStatus,
  GeoInfo,
  LogEntry,
  Profile,
  toProfile,
} from "./types";

// ---------- backend service shape (structural, mirror of gui/service.go) ----------

interface ServiceAPI {
  GetStatus(): Promise<unknown>;
  Start(): Promise<unknown>;
  Stop(): Promise<unknown>;
  ListProfiles(): Promise<unknown>;
  SaveProfiles(profiles: unknown): Promise<unknown>;
  GetRules(): Promise<unknown>;
  SaveRules(rulesText: string): Promise<unknown>;
  ReloadRules(): Promise<unknown>;
  GetGeo(): Promise<unknown>;
  UpdateGeo(): Promise<unknown>;
  SetSystemProxy(enabled: boolean): Promise<unknown>;
  GetSystemProxyEnabled(): Promise<unknown>;
  SetAutostart(enabled: boolean): Promise<unknown>;
  GetAutostartEnabled(): Promise<unknown>;
  GetLogs(limit: number): Promise<unknown>;
  ClearLogs(): Promise<unknown>;
  SidecarLog(limit: number): Promise<unknown>;
  GetVersion(): Promise<unknown>;
  CheckUpdate(): Promise<unknown>;
  OpenExternalBrowser(url: string): Promise<unknown>;
}

// ---------- demo data (used while bindings are placeholders) ----------

const DEFAULT_RULES = `x-tunnel 路由规则（每行：行为,条件；# 为注释）
direct,cn
direct,geosite:cn
direct,geoip:private
proxy,geosite:geolocation-!cn
`;

const mockState = {
  running: false,
  sysProxy: false,
  autostart: false,
  rulesText: DEFAULT_RULES,
  hits: { proxy: 128, direct: 947, rejected: 23 },
  bytes: { sent: 12_345_678, recv: 98_765_432 },
  startedAt: undefined as string | undefined,
  profiles: [
    {
      name: "我的服务器",
      serverURL: "wss://cf.example.com:8443",
      token: "demo-token",
      localListen: "socks5://127.0.0.1:11080",
      cidr: "",
      dns: "",
      ech: "",
      blockPorts: "443",
      connections: 3,
      insecure: false,
      fallback: true,
      dialIPs: "",
      ipStrategy: "4",
      dnsCacheTTL: "5m",
    },
  ] as Profile[],
  activeProfile: "我的服务器",
  logs: [
    { time: "19:40:02", level: "info", msg: "x-tunnel Windows GUI（演示模式）" },
    { time: "19:40:02", level: "info", msg: "已加载配置 profiles.json" },
    { time: "19:40:03", level: "info", msg: "规则引擎就绪：4 条规则" },
    { time: "19:40:03", level: "debug", msg: "GEO 数据库命中缓存" },
    { time: "19:41:27", level: "warn", msg: "重连：连接空闲超时，自动恢复" },
    { time: "19:41:28", level: "info", msg: "已建立 wss 隧道" },
  ] as LogEntry[],
};

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const jitter = (base: number) => base + Math.random() * 220;

// ---------- service resolution ----------

let svcPromise: Promise<ServiceAPI | null> | null = null;

function loadService(): Promise<ServiceAPI | null> {
  if (!svcPromise) {
    svcPromise = (async () => {
      try {
        // Wails v3 把 bindings 生成到 frontend/bindings/warp/gui/（带 module 路径）；
        // 之前误用 bindings/gui/ 导致永远命中占位/演示模式。
        const mod = (await import("../../bindings/github.com/callacat/x-tunnel-windows/gui/index.js")) as {
          Service?: Record<string, unknown>;
        };
        const ns = mod.Service;
        if (!ns) return null;
        // Placeholder stand-in? -> use demo data instead of calling $Call.
        if (ns.__MOCK_BINDINGS__ === true) return null;
        return ns as unknown as ServiceAPI;
      } catch {
        return null;
      }
    })();
  }
  return svcPromise;
}

/** True when running standalone without the Wails bridge. */
export async function isDemoMode(): Promise<boolean> {
  return (await loadService()) === null;
}

// ---------- mock implementation ----------

function mockStatus(): AppStatus {
  return {
    running: mockState.running,
    listening: "127.0.0.1:11080",
    startedAt: mockState.startedAt,
    initDone: true,
    sysProxyOn: mockState.sysProxy,
    activeName: mockState.running ? mockState.activeProfile : undefined,
    configured: true,
    sidecarOk: true,
    routeEnabled: true,
    ruleCount: 4,
    proxyHits: mockState.hits.proxy,
    directHits: mockState.hits.direct,
    rejectedHits: mockState.hits.rejected,
    siteLoaded: true,
    ipLoaded: true,
    bytesSent: mockState.bytes.sent,
    bytesRecv: mockState.bytes.recv,
  };
}

function mockGeo(): GeoInfo {
  return {
    geositePath: "geo/geosite.dat",
    geoipPath: "geo/geoip-lite.dat",
    geositeUpdated: "2026-07-30 04:00:00",
    geoipUpdated: "2026-07-30 04:00:00",
    repository: "MetaCubeX/meta-rules-dat",
    baseURL: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest",
    autoUpdateDays: 7,
    lastChecked: "2026-07-31 04:00:00",
  };
}

// ---------- public API ----------

export async function getStatus(): Promise<AppStatus> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(150));
    return mockStatus();
  }
  return fromStatus(await svc.GetStatus());
}

export async function start(): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(300));
    mockState.running = true;
    mockState.startedAt = new Date().toLocaleString("zh-CN");
    mockState.logs.push({ time: now(), level: "info", msg: "代理已启动（演示）" });
    return;
  }
  await svc.Start();
}

export async function stop(): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(300));
    mockState.running = false;
    mockState.startedAt = undefined;
    mockState.logs.push({ time: now(), level: "info", msg: "代理已停止（演示）" });
    return;
  }
  await svc.Stop();
}

export async function getProfiles(): Promise<AppProfiles> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(150));
    return fromProfiles({ active_profile: mockState.activeProfile, profiles: mockState.profiles });
  }
  return fromProfiles(await svc.ListProfiles());
}

export async function saveProfiles(app: AppProfiles): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(250));
    mockState.activeProfile = app.activeProfile;
    mockState.profiles = app.profiles;
    mockState.logs.push({ time: now(), level: "info", msg: "配置已保存（演示）" });
    return;
  }
  // 前端 camelCase -> 后端 snake_case（对齐 Go XTunnelProfile JSON tag）。
  await svc.SaveProfiles({
    active_profile: app.activeProfile,
    profiles: app.profiles.map(toProfile),
  });
}

export async function getRules(): Promise<string> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(150));
    return mockState.rulesText;
  }
  const raw = await svc.GetRules();
  return typeof raw === "string" ? raw : String(raw ?? "");
}

export async function saveRules(rulesText: string): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(250));
    mockState.rulesText = rulesText;
    mockState.logs.push({ time: now(), level: "info", msg: "规则已保存（演示）" });
    return;
  }
  await svc.SaveRules(rulesText);
}

export async function reloadRules(): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(200));
    mockState.logs.push({ time: now(), level: "info", msg: "规则已热重载（演示）" });
    return;
  }
  await svc.ReloadRules();
}

export async function getGeo(): Promise<GeoInfo> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(150));
    return mockGeo();
  }
  return fromGeo(await svc.GetGeo());
}

export interface UpdateGeoResult {
  ok: boolean;
  message: string;
}

export async function updateGeo(): Promise<UpdateGeoResult> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(600));
    mockState.logs.push({
      time: now(),
      level: "info",
      msg: "GEO 数据已更新（演示）：geosite.dat / geoip-lite.dat",
    });
    return { ok: true, message: "GEO 数据已是最新（演示）" };
  }
  const raw = await svc.UpdateGeo();
  const r = raw as { ok?: boolean; message?: string } | null;
  // Wails bindings 返回 UpdateGeoResult 对象（{ok, message}），不是 Error。
  return {
    ok: r?.ok === true,
    message: r?.message ?? (r?.ok ? "GEO 数据已更新" : "GEO 数据更新失败"),
  };
}

export async function setSystemProxy(enabled: boolean): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(250));
    mockState.sysProxy = enabled;
    mockState.logs.push({
      time: now(),
      level: "info",
      msg: `系统代理已${enabled ? "启用" : "关闭"}（演示）`,
    });
    return;
  }
  await svc.SetSystemProxy(enabled);
}

export async function getSystemProxyEnabled(): Promise<boolean> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(120));
    return mockState.sysProxy;
  }
  return (await svc.GetSystemProxyEnabled()) === true;
}

export async function setAutostart(enabled: boolean): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(200));
    mockState.autostart = enabled;
    return;
  }
  await svc.SetAutostart(enabled);
}

export async function getAutostartEnabled(): Promise<boolean> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(120));
    return mockState.autostart;
  }
  return (await svc.GetAutostartEnabled()) === true;
}

export async function getLogs(limit = 200): Promise<LogEntry[]> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(120));
    return [...mockState.logs].slice(-limit);
  }
  return fromLogs(await svc.GetLogs(limit));
}

export async function getVersion(): Promise<string> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(60));
    return "dev";
  }
  const raw = await svc.GetVersion();
  return typeof raw === "string" ? raw : "dev";
}

export interface UpdateInfo {
  current: string;
  latest: string;
  has_update: boolean;
  url: string;
  tag: string;
}

export async function checkUpdate(): Promise<UpdateInfo> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(400));
    return { current: "dev", latest: "dev", has_update: false, url: "", tag: "" };
  }
  const raw = await svc.CheckUpdate();
  return (raw as UpdateInfo) ?? { current: "", latest: "", has_update: false, url: "", tag: "" };
}

// clearLogs 清空后端日志环形缓冲（LogsPage "清空" 按钮）。
export async function clearLogs(): Promise<void> {
  const svc = await loadService();
  if (!svc) {
    mockState.logs = [];
    return;
  }
  await svc.ClearLogs();
}

// getSidecarLog 读取 sidecar（x-tunnel.exe）原始日志文本（桌面端）。
export async function getSidecarLog(limit = 200): Promise<string> {
  const svc = await loadService();
  if (!svc) {
    await sleep(jitter(120));
    return "（演示模式）sidecar 日志暂不可用";
  }
  const raw = await svc.SidecarLog(limit);
  return typeof raw === "string" ? raw : String(raw ?? "");
}

// openExternalBrowser 用系统浏览器打开 URL（桌面走默认浏览器）。
export async function openExternalBrowser(url: string): Promise<void> {
  const svc = await loadService();
  if (!svc) return;
  await svc.OpenExternalBrowser(url);
}

function now(): string {
  return new Date().toLocaleTimeString("zh-CN", { hour12: false });
}
