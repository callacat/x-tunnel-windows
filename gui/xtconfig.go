package main

// xtconfig：x-tunnel sidecar 配置模型与 GUI Profile 管理。
// Profile 持久化到工作目录 profiles.json（多配置+激活项），启动时合成
// sidecar FileConfig（与 x-tunnel internal/app/config.go FileConfig 字段
// 一一对应，GUI 侧零猜测）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// XTunnelProfile 是 GUI 管理的一个服务器配置（对齐 Android XTunnelProfile 全字段）。
type XTunnelProfile struct {
	Name        string `json:"name"`
	ServerURL   string `json:"server_url"` // wss://host:port[/path] 或 ws://
	Token       string `json:"token"`
	LocalListen string `json:"local_listen"` // socks5://127.0.0.1:11080
	CIDR        string `json:"cidr"`         // 路由 CIDR（空=默认）
	DNS         string `json:"dns"`          // 自定义 DNS（空=默认）
	ECH         string `json:"ech"`          // ECH 域名（空=默认 cloudflare-ech.com）
	BlockPorts  string `json:"block_ports"`  // UDP 阻断端口，如 "443"
	Connections int    `json:"connections"`  // 每 IP 连接数
	Insecure    bool   `json:"insecure"`
	Fallback    bool   `json:"fallback"`
	DialIPs     string `json:"dial_ips"`    // -ip CF 优选 IP/域名，逗号分隔
	IPStrategy  string `json:"ip_strategy"` // 4|6|4,6|6,4
	DNSCacheTTL string `json:"dns_cache_ttl"`
}

// AppProfiles 是 profiles.json 的顶层结构。
type AppProfiles struct {
	ActiveName string           `json:"active_profile"`
	Profiles   []XTunnelProfile `json:"profiles"`
	// GhProxy 是 GitHub 加速下载前缀（东哥 09-05 反馈④），用于 GEO 数据库
	// 与检查更新等 GitHub 下载。空 = 默认 https://gh-proxy.org；"off" = 直连。
	GhProxy string `json:"gh_proxy,omitempty"`
}

// DefaultProfile 返回一个空模板配置（不内置任何真实服务器地址/token——
// 对齐 Android 点7 纪律）。
func DefaultProfile(n int) XTunnelProfile {
	return XTunnelProfile{
		Name:        fmt.Sprintf("配置 %d", n),
		ServerURL:   "",
		Token:       "",
		LocalListen: "socks5://127.0.0.1:11080",
		CIDR:        "",
		BlockPorts:  "443",
		Connections: 3,
		Insecure:    false,
		Fallback:    true,
		IPStrategy:  "4",
		DNSCacheTTL: "5m",
	}
}

// loadProfiles 读 profiles.json（缺失/损坏返回空列表，不报错——首启空模板）。
func loadProfiles(dir string) AppProfiles {
	raw, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if err != nil {
		return AppProfiles{}
	}
	var p AppProfiles
	if json.Unmarshal(raw, &p) != nil {
		return AppProfiles{}
	}
	return p
}

// saveProfiles 原子写回 profiles.json。
func saveProfiles(dir string, p AppProfiles) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(filepath.Join(dir, "profiles.json"), data)
}

// validateProfile 校验必填项与 URL 格式（保存前调用；不内置服务器=允许空 server_url）。
func validateProfile(p XTunnelProfile) error {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return errors.New("配置名称不能为空")
	}
	if len(name) > 50 {
		return errors.New("配置名称过长（≤50 字符）")
	}
	if u := strings.TrimSpace(p.ServerURL); u != "" {
		if !strings.HasPrefix(u, "ws://") && !strings.HasPrefix(u, "wss://") {
			return errors.New("服务器地址必须以 ws:// 或 wss:// 开头")
		}
	}
	if p.LocalListen == "" {
		return errors.New("本地监听地址不能为空")
	}
	if !strings.HasPrefix(p.LocalListen, "socks5://") {
		return errors.New("本地监听必须是 socks5:// 格式")
	}
	return nil
}

// synthesizeFileConfig 把 Profile 合成为 sidecar FileConfig（仅 GUI 实际控制的
// 字段；其余走 sidecar 默认值。字段名与 x-tunnel FileConfig 的 json tag 严格一致）。
// GUI 固定注入：listen/control/ready-file 由命令行传（优先级高于配置文件）。
func synthesizeFileConfig(p XTunnelProfile, geoDir, rulesPath string, routeEnabled bool) map[string]any {
	fc := map[string]any{}
	str := func(k, v string) {
		if v != "" {
			fc[k] = v
		}
	}
	str("forward", strings.TrimSpace(p.ServerURL))
	str("token", p.Token)
	str("listen", appendHTTPListen(p.LocalListen))
	str("cidr", p.CIDR)
	str("dns", p.DNS)
	str("ech", p.ECH)
	str("block", p.BlockPorts)
	str("ip", p.DialIPs)
	str("ips", p.IPStrategy)
	str("dns_cache_ttl", p.DNSCacheTTL)
	str("geo_dir", geoDir)
	str("rules_path", rulesPath)
	if p.Connections > 0 {
		fc["connections"] = p.Connections
	}
	if p.Insecure {
		fc["insecure"] = true
	}
	if p.Fallback {
		fc["fallback"] = true
	}
	if routeEnabled {
		fc["route_enabled"] = true
	}
	return fc
}

// httpListenDelta 是系统代理 HTTP 监听相对 SOCKS5 监听的端口偏移。
// sidecar 同进程双监听：socks5://127.0.0.1:11080 + http://127.0.0.1:11081。
const httpListenDelta = 1

// appendHTTPListen 在 socks5 监听上追加同机 +1 端口的 HTTP 代理监听
// （x-tunnel -l 支持逗号分隔多监听）。系统代理注册表的 http=/https= 段
// 指向该 HTTP 端口（WinINET 完整语义：CONNECT 域名透传=服务端远程解析），
// socks= 段指向原 SOCKS5 端口。仅回环监听追加；已是双监听则原样返回。
func appendHTTPListen(socksListen string) string {
	const prefix = "socks5://"
	s := strings.TrimSpace(socksListen)
	if !strings.HasPrefix(s, prefix) {
		return socksListen
	}
	hostPort := s[len(prefix):]
	i := strings.LastIndex(hostPort, ":")
	if i < 0 {
		return socksListen
	}
	host, portStr := hostPort[:i], hostPort[i+1:]
	if host != "127.0.0.1" && host != "localhost" {
		return socksListen // 系统代理只对回环监听有意义
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port+httpListenDelta > 65535 {
		return socksListen
	}
	return s + ",http://" + host + ":" + strconv.Itoa(port+httpListenDelta)
}

// sysproxyTargetsFromListen 返回系统代理注册表的两个地址：
// HTTP 代理（http=/https= 段用）与 SOCKS5（socks= 段用），均为 host:port。
func sysproxyTargetsFromListen(socksListen string) (httpAddr, socksAddr string, err error) {
	const prefix = "socks5://"
	s := strings.TrimSpace(socksListen)
	if !strings.HasPrefix(s, prefix) {
		return "", "", fmt.Errorf("监听地址必须是 socks5:// 格式：%s", socksListen)
	}
	hostPort := s[len(prefix):]
	i := strings.LastIndex(hostPort, ":")
	if i < 0 {
		return "", "", fmt.Errorf("监听地址缺少端口：%s", socksListen)
	}
	host, portStr := hostPort[:i], hostPort[i+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return "", "", fmt.Errorf("监听端口非法：%s", socksListen)
	}
	socksAddr = net.JoinHostPort(host, portStr)
	if host != "127.0.0.1" && host != "localhost" {
		// 非回环监听：无 HTTP 伴生端口，三段都指向 SOCKS（少见配置，尽力兜底）。
		return socksAddr, socksAddr, nil
	}
	httpAddr = net.JoinHostPort(host, strconv.Itoa(port+httpListenDelta))
	return httpAddr, socksAddr, nil
}
