// 前端契约类型: wails3 generate bindings 生成的 TS 为唯一类型源。
//
// from* 输入参数为 any（接受 ServiceAPI 的 unknown 返回值与测试部分对象），
// 输出为前端人体工学 camelCase 类型（命名自适应层）。
// 保留 fromLogs (level 校验降级) 与 fromStatus (state -> running 派生)。

export interface AppStatus {
  running: boolean;
  listening: string;
  startedAt?: string;
  error?: string;
  initDone: boolean;
  sysProxyOn: boolean;
  // 以下为 x-tunnel 新增状态字段（可选，后端未就绪时缺省）。
  activeName?: string; // 当前激活配置名
  configured?: boolean; // 已配置服务器
  sidecarOk?: boolean; // sidecar（x-tunnel.exe）在位
  routeEnabled?: boolean; // 路由规则生效
  ruleCount?: number; // 规则条数
  proxyHits?: number; // 走隧道命中数
  directHits?: number; // 直连命中数
  rejectedHits?: number; // 拦截命中数
  siteLoaded?: boolean; // geosite 数据已加载
  ipLoaded?: boolean; // geoip 数据已加载
  bytesSent?: number; // 上行字节
  bytesRecv?: number; // 下行字节
  init?: {
    state: "idle" | "downloading" | "done" | "failed";
    current?: string;
    progress?: number;
    error?: string;
  };
}

// Profile 是 GUI 管理的一个服务器配置（对齐 Go gui.XTunnelProfile 全字段）。
export interface Profile {
  name: string;
  serverURL: string; // wss://host:port[/path] 或 ws://
  token: string;
  localListen: string; // socks5://127.0.0.1:11080
  cidr: string; // 路由 CIDR（空=默认）
  dns: string; // 自定义 DNS（空=默认）
  ech: string; // ECH 域名（空=默认 cloudflare-ech.com）
  blockPorts: string; // UDP 阻断端口，如 "443"
  connections: number; // 每 IP 连接数
  insecure: boolean;
  fallback: boolean;
  dialIPs: string; // -ip CF 优选 IP/域名，逗号分隔
  ipStrategy: string; // 4|6|4,6|6,4
  dnsCacheTTL: string;
}

// AppProfiles 是 profiles.json 的顶层结构。
export interface AppProfiles {
  activeProfile: string;
  profiles: Profile[];
}

export type RuleAction = "proxy" | "direct";

export interface RuleEntry {
  line: number;
  action: RuleAction;
  condition: string;
}

export interface GeoInfo {
  geositePath: string;
  geoipPath: string;
  geositeUpdated?: string;
  geoipUpdated?: string;
  repository: string;
  baseURL: string;
  autoUpdateDays: number;
  lastChecked?: string;
}

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  time: string;
  level: LogLevel;
  msg: string;
}

// ---------- bindings 适配 (生成类型 -> UI 类型) ----------

export function fromStatus(v: any): AppStatus {
  const o = v ?? {};
  const state = o.state;
  return {
    running: state === "running" || state === "starting",
    listening: o.listen_addr ?? "127.0.0.1:40000",
    startedAt: o.started_at,
    error: o.last_error,
    initDone: o.init_done === true,
    sysProxyOn: o.sys_proxy_on === true,
    activeName: o.active_name,
    configured: o.configured === true,
    sidecarOk: o.sidecar_ok === true,
    routeEnabled: o.route_enabled === true,
    ruleCount: typeof o.rule_count === "number" ? o.rule_count : undefined,
    proxyHits: typeof o.proxy_hits === "number" ? o.proxy_hits : 0,
    directHits: typeof o.direct_hits === "number" ? o.direct_hits : 0,
    rejectedHits: typeof o.rejected_hits === "number" ? o.rejected_hits : 0,
    siteLoaded: o.site_loaded === true,
    ipLoaded: o.ip_loaded === true,
    bytesSent: typeof o.bytes_sent === "number" ? o.bytes_sent : 0,
    bytesRecv: typeof o.bytes_recv === "number" ? o.bytes_recv : 0,
    init: o.init
      ? {
          state: o.init.state ?? "idle",
          current: o.init.current,
          progress: typeof o.init.progress === "number" ? o.init.progress : undefined,
          error: o.init.error,
        }
      : undefined,
  };
}

/** 后端 XTunnelProfile（snake_case）-> 前端 Profile（camelCase）。 */
export function fromProfile(v: any): Profile {
  const o = v ?? {};
  return {
    name: String(o.name ?? ""),
    serverURL: String(o.server_url ?? ""),
    token: String(o.token ?? ""),
    localListen: String(o.local_listen ?? "socks5://127.0.0.1:11080"),
    cidr: String(o.cidr ?? ""),
    dns: String(o.dns ?? ""),
    ech: String(o.ech ?? ""),
    blockPorts: String(o.block_ports ?? "443"),
    connections: Number.isFinite(o.connections) ? o.connections : 3,
    insecure: o.insecure === true,
    fallback: o.fallback !== false,
    dialIPs: String(o.dial_ips ?? ""),
    ipStrategy: String(o.ip_strategy ?? "4"),
    dnsCacheTTL: String(o.dns_cache_ttl ?? "5m"),
  };
}

/** 后端 AppProfiles -> 前端 AppProfiles。 */
export function fromProfiles(v: any): AppProfiles {
  const o = v ?? {};
  return {
    activeProfile: String(o.active_profile ?? ""),
    profiles: Array.isArray(o.profiles) ? o.profiles.map(fromProfile) : [],
  };
}

/** 前端 Profile -> 后端 XTunnelProfile（snake_case），供 SaveProfiles 回写。 */
export function toProfile(p: Profile): Record<string, unknown> {
  return {
    name: p.name,
    server_url: p.serverURL,
    token: p.token,
    local_listen: p.localListen,
    cidr: p.cidr,
    dns: p.dns,
    ech: p.ech,
    block_ports: p.blockPorts,
    connections: p.connections,
    insecure: p.insecure,
    fallback: p.fallback,
    dial_ips: p.dialIPs,
    ip_strategy: p.ipStrategy,
    dns_cache_ttl: p.dnsCacheTTL,
  };
}

export function fromGeo(v: any): GeoInfo {
  const o = v ?? {};
  return {
    geositePath: o.geosite_path ?? "geo/geosite.dat",
    geoipPath: o.geoip_path ?? "geo/geoip-lite.dat",
    geositeUpdated: o.geosite_updated,
    geoipUpdated: o.geoip_updated,
    repository: o.repository ?? "MetaCubeX/meta-rules-dat",
    baseURL: o.base_url ?? "",
    autoUpdateDays: o.auto_update_days ?? 7,
    lastChecked: o.last_checked,
  };
}

const LEVELS: LogLevel[] = ["debug", "info", "warn", "error"];

export function fromLogs(v: any): LogEntry[] {
  if (!Array.isArray(v)) return [];
  const out: LogEntry[] = [];
  for (const raw of v) {
    const o = raw ?? {};
    // 空条目直接丢弃（防御：后端环形缓冲空槽位/旧版残留——空 level 会被
    // 下面归一化成 info，表现为日志页 info 空行刷屏）。
    if (!o.msg && !o.time) continue;
    const lv = (o.level ?? "info").toLowerCase();
    out.push({
      time: o.time ?? "",
      level: (LEVELS.includes(lv as LogLevel) ? lv : "info") as LogLevel,
      msg: o.msg ?? "",
    });
  }
  return out;
}
