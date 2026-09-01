import { describe, it, expect } from "vitest";
import {
  fromStatus,
  fromGeo,
  fromProfiles,
  toProfile,
} from "./types";

// 用后端 gui.Status JSON 的真实形态（snake_case）验证前端映射。
// 序列化实测：{"state":"running","listen_addr":"127.0.0.1:11080",
// "sys_proxy_on":true,"init_done":true,"active_name":"我的服务器",
// "configured":true,"sidecar_ok":true,"route_enabled":true,"rule_count":4,
// "proxy_hits":128,"direct_hits":947,"rejected_hits":23,"site_loaded":true,
// "ip_loaded":true,"bytes_sent":123,"bytes_recv":456}
const STATUS_JSON = {
  state: "running",
  listen_addr: "127.0.0.1:11080",
  started_at: "2026-08-02 10:00:00",
  last_error: "",
  sys_proxy_on: true,
  init_done: true,
  sidecar_ok: true,
  active_name: "我的服务器",
  configured: true,
  route_enabled: true,
  rule_count: 4,
  proxy_hits: 128,
  direct_hits: 947,
  rejected_hits: 23,
  site_loaded: true,
  ip_loaded: true,
  bytes_sent: 123,
  bytes_recv: 456,
};

describe("fromStatus", () => {
  it("maps state to running flag", () => {
    expect(fromStatus(STATUS_JSON).running).toBe(true);
  });

  it("treats starting as running", () => {
    expect(fromStatus({ state: "starting" }).running).toBe(true);
    expect(fromStatus({ state: "stopped" }).running).toBe(false);
    expect(fromStatus({ state: "failed" }).running).toBe(false);
  });

  it("passes through the new status fields", () => {
    const s = fromStatus(STATUS_JSON);
    expect(s.activeName).toBe("我的服务器");
    expect(s.configured).toBe(true);
    expect(s.sidecarOk).toBe(true);
    expect(s.routeEnabled).toBe(true);
    expect(s.ruleCount).toBe(4);
    expect(s.proxyHits).toBe(128);
    expect(s.directHits).toBe(947);
    expect(s.rejectedHits).toBe(23);
    expect(s.siteLoaded).toBe(true);
    expect(s.ipLoaded).toBe(true);
    expect(s.bytesSent).toBe(123);
    expect(s.bytesRecv).toBe(456);
  });

  it("defaults new fields when missing", () => {
    const s = fromStatus({});
    expect(s.activeName).toBeUndefined();
    expect(s.configured).toBe(false);
    expect(s.sidecarOk).toBe(false);
    expect(s.proxyHits).toBe(0);
    expect(s.directHits).toBe(0);
    expect(s.rejectedHits).toBe(0);
    expect(s.bytesSent).toBe(0);
    expect(s.bytesRecv).toBe(0);
    expect(s.running).toBe(false);
  });

  it("state is empty when status lacks state field", () => {
    expect(fromStatus({}).running).toBe(false);
  });

  it("maps init_done flag", () => {
    expect(fromStatus({ init_done: true }).initDone).toBe(true);
    expect(fromStatus({ init_done: false }).initDone).toBe(false);
    expect(fromStatus({}).initDone).toBe(false);
  });

  it("maps sys_proxy_on flag", () => {
    expect(fromStatus({ sys_proxy_on: true }).sysProxyOn).toBe(true);
    expect(fromStatus({ sys_proxy_on: false }).sysProxyOn).toBe(false);
    expect(fromStatus({}).sysProxyOn).toBe(false);
  });
});

describe("fromProfiles / toProfile", () => {
  const PROFILES_JSON = {
    active_profile: "服务器A",
    profiles: [
      {
        name: "服务器A",
        server_url: "wss://cf.example.com:8443",
        token: "t0k3n",
        local_listen: "socks5://127.0.0.1:11080",
        cidr: "10.0.0.0/8",
        dns: "1.1.1.1",
        ech: "cloudflare-ech.com",
        block_ports: "443,853",
        connections: 5,
        insecure: true,
        fallback: false,
        dial_ips: "162.159.192.5",
        ip_strategy: "4,6",
        dns_cache_ttl: "10m",
      },
    ],
  };

  it("maps snake_case AppProfiles to camelCase", () => {
    const p = fromProfiles(PROFILES_JSON);
    expect(p.activeProfile).toBe("服务器A");
    expect(p.profiles).toHaveLength(1);
    const [x] = p.profiles;
    expect(x.name).toBe("服务器A");
    expect(x.serverURL).toBe("wss://cf.example.com:8443");
    expect(x.localListen).toBe("socks5://127.0.0.1:11080");
    expect(x.blockPorts).toBe("443,853");
    expect(x.connections).toBe(5);
    expect(x.insecure).toBe(true);
    expect(x.fallback).toBe(false);
    expect(x.dialIPs).toBe("162.159.192.5");
    expect(x.ipStrategy).toBe("4,6");
    expect(x.dnsCacheTTL).toBe("10m");
  });

  it("handles missing profiles array", () => {
    expect(fromProfiles({}).profiles).toEqual([]);
    expect(fromProfiles({}).activeProfile).toBe("");
  });

  it("round-trips toProfile back to snake_case", () => {
    const [x] = fromProfiles(PROFILES_JSON).profiles;
    const back = toProfile(x);
    expect(back.server_url).toBe("wss://cf.example.com:8443");
    expect(back.local_listen).toBe("socks5://127.0.0.1:11080");
    expect(back.block_ports).toBe("443,853");
    expect(back.connections).toBe(5);
    expect(back.insecure).toBe(true);
    expect(back.fallback).toBe(false);
    expect(back.dial_ips).toBe("162.159.192.5");
    expect(back.ip_strategy).toBe("4,6");
    expect(back.dns_cache_ttl).toBe("10m");
  });
});

describe("fromGeo", () => {
  const GEO_JSON = {
    geosite_path: "geo/geosite.dat",
    geoip_path: "geo/geoip-lite.dat",
    geosite_updated: "2026-08-02 10:00",
    geoip_updated: "2026-08-02 10:00",
    repository: "MetaCubeX/meta-rules-dat",
    base_url: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest",
    auto_update_days: 7,
    last_checked: "2026-08-02 10:00",
  };

  it("maps updated timestamps from snake_case", () => {
    const g = fromGeo(GEO_JSON);
    expect(g.geositeUpdated).toBe("2026-08-02 10:00");
    expect(g.geoipUpdated).toBe("2026-08-02 10:00");
    expect(g.lastChecked).toBe("2026-08-02 10:00");
    expect(g.repository).toBe("MetaCubeX/meta-rules-dat");
  });

  it("falls back to defaults when fields missing", () => {
    const g = fromGeo({});
    expect(g.geositeUpdated).toBeUndefined();
    expect(g.repository).toBe("MetaCubeX/meta-rules-dat");
  });
});