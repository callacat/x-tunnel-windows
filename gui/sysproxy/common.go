// Package sysproxy 设置/清除操作系统级代理，供 CLI（-sysproxy）与 GUI 共用。
package sysproxy

import (
	"fmt"
	"net"
	"strings"
)

// Set 将系统代理指向 addr（形如 "host:port"，host 可为域名或 IP，IPv6 需方括号）。
// enabled=false 时清除系统代理（还原到禁用状态）——此时 addr 可为空串
// （清除不依赖地址）。httpAddr 与 socksAddr 允许不同（x-tunnel 场景：
// http/https 段走 HTTP 代理端口，socks 段走 SOCKS5 端口）；为空时全部用 addr。
//
// 各平台实现：
//   - windows：写入 HKCU Internet Settings 的 ProxyEnable/ProxyServer
//   - darwin：对每个网络服务执行 networksetup（web/secure 代理）
//   - linux：gsettings（GNOME）org.gnome.system.proxy
func Set(addr string, enabled bool) error {
	return SetDual(addr, addr, enabled)
}

// SetDual 同 Set，但 http/https 段与 socks 段可分别指向不同地址。
// 语义见 Set；启用时两个地址都会做 host:port 校验。
//
// 禁用路径（v0.1.4 东哥真机回归）：不对地址做校验——清除系统代理只动
// ProxyEnable/mode/proxystate，地址用不上；此前空串地址在 splitAddr 就报
// 「missing port in address」，导致停止内核后系统代理永远关不掉（开关卡死）。
func SetDual(httpAddr, socksAddr string, enabled bool) error {
	if !enabled {
		// 清除只需一次（只动开关），地址不参与，空串直接放行。
		return set("", "", "", "", false)
	}
	hh, hp, err := splitAddr(httpAddr)
	if err != nil {
		return err
	}
	sh, sp, err := splitAddr(socksAddr)
	if err != nil {
		return err
	}
	return set(hh, hp, sh, sp, true)
}

// Enabled 报告系统代理当前是否启用且指向 addr（本程序设置的地址）。
// 用于检测"外部软件关闭了系统代理"——GUI 轮询它来同步开关状态。
// addr 形如 "host:port"，与 Set 一致。
func Enabled(addr string) (bool, error) {
	host, port, err := splitAddr(addr)
	if err != nil {
		return false, err
	}
	return enabled(host, port)
}

// EnabledDual 同 Enabled，但任一地址命中即视为启用。
// 地址为空串时该侧跳过（返回 false 不报错）——无激活配置/监听未解析时
// 前端轮询不应收到错误（v0.1.4 东哥真机回归）。
func EnabledDual(httpAddr, socksAddr string) (bool, error) {
	if strings.TrimSpace(httpAddr) == "" && strings.TrimSpace(socksAddr) == "" {
		return false, nil
	}
	var on bool
	var err error
	if hh, hp, e := splitAddr(httpAddr); e == nil {
		on, err = enabled(hh, hp)
		if err != nil || on {
			return on, err
		}
	}
	sh, sp, err := splitAddr(socksAddr)
	if err != nil {
		return on, nil // http 侧已查过未命中；socks 地址非法不致命
	}
	on2, err2 := enabled(sh, sp)
	if err2 != nil {
		return on, nil
	}
	return on2, nil
}

func splitAddr(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("系统代理地址 %q 非法：%w", addr, err)
	}
	if strings.TrimSpace(host) == "" {
		return "", "", fmt.Errorf("系统代理地址 %q 缺少主机名", addr)
	}
	if strings.TrimSpace(port) == "" {
		return "", "", fmt.Errorf("系统代理地址 %q 缺少端口", addr)
	}
	return host, port, nil
}

// containsTarget 检查代理配置字符串中是否含 "host:port" 段（按 ; 与 ,
// 分隔的协议段，忽略 "proto=" 前缀）。命中任一即 true。Windows 的
// ProxyServer（"http=host:port;https=host:port;socks=host:port"）用它判断
// 系统代理是否仍指向目标地址。
func containsTarget(proxyConfig, ep string) bool {
	for _, seg := range strings.FieldsFunc(proxyConfig, func(r rune) bool {
		return r == ';' || r == ','
	}) {
		seg = strings.TrimSpace(seg)
		if i := strings.IndexByte(seg, '='); i >= 0 {
			seg = seg[i+1:]
		}
		if seg == ep {
			return true
		}
	}
	return false
}
