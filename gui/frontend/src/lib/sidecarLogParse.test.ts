import { describe, expect, it } from "vitest";
// LogsPage 内部解析函数（导出以供测试）——sidecar.log 行 → LogEntry。
import { parseSidecarLog, parseSidecarLine } from "../lib/sidecarLog";

describe("parseSidecarLine", () => {
  it("解析标准库日期前缀行", () => {
    const e = parseSidecarLine(
      "2026/09/05 17:43:39 [客户端] SOCKS5 代理: 127.0.0.1:11080",
    );
    expect(e).not.toBeNull();
    expect(e!.time).toBe("17:43:39");
    expect(e!.level).toBe("info");
    expect(e!.msg).toBe("[客户端] SOCKS5 代理: 127.0.0.1:11080");
  });

  it("失败行分级 error", () => {
    const e = parseSidecarLine(
      "2026/09/05 17:46:38 [客户端] 127.0.0.1:58172 DIRECT 直连失败 dotcounter.douyucdn.cn:443: dial tcp: no such host",
    );
    expect(e!.level).toBe("error");
  });

  it("空行返回 null", () => {
    expect(parseSidecarLine("   ")).toBeNull();
  });

  it("无日期前缀行也能解析（时间空串）", () => {
    const e = parseSidecarLine("plain message");
    expect(e!.time).toBe("");
    expect(e!.msg).toBe("plain message");
  });
});

describe("parseSidecarLog", () => {
  it("多行解析 + limit 截尾", () => {
    const raw = [
      "2026/09/05 17:43:39 ✓ geosite.dat 已加载（1549 类别）",
      "2026/09/05 17:43:40 [客户端] 通道 1 就绪 (smux)",
      "2026/09/05 17:46:38 [客户端] DIRECT 直连失败 x:443",
    ].join("\n");
    const entries = parseSidecarLog(raw, 2);
    expect(entries).toHaveLength(2);
    expect(entries[0].msg).toContain("通道 1 就绪");
    expect(entries[1].level).toBe("error");
  });
});
