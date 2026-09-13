package main

import "testing"

// TestAppendHTTPListen 验证 sidecar config 的双监听合成。
func TestAppendHTTPListen(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"默认回环", "socks5://127.0.0.1:11080", "socks5://127.0.0.1:11080,http://127.0.0.1:11081"},
		{"localhost", "socks5://localhost:11080", "socks5://localhost:11080,http://localhost:11081"},
		{"非回环不追加", "socks5://0.0.0.0:11080", "socks5://0.0.0.0:11080"},
		{"非 socks5 前缀原样", "http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"空串原样", "", ""},
		{"端口 65535 不追加（+1 越界）", "socks5://127.0.0.1:65535", "socks5://127.0.0.1:65535"},
		{"自定义端口", "socks5://127.0.0.1:20000", "socks5://127.0.0.1:20000,http://127.0.0.1:20001"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := appendHTTPListen(c.in); got != c.want {
				t.Errorf("appendHTTPListen(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestSysproxyTargetsFromListen 验证注册表双地址推导。
func TestSysproxyTargetsFromListen(t *testing.T) {
	// 回环：http 段=+1 端口，socks 段=原端口。
	http, socks, err := sysproxyTargetsFromListen("socks5://127.0.0.1:11080")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if http != "127.0.0.1:11081" {
		t.Errorf("httpAddr = %q, want 127.0.0.1:11081", http)
	}
	if socks != "127.0.0.1:11080" {
		t.Errorf("socksAddr = %q, want 127.0.0.1:11080", socks)
	}

	// 非回环：无 HTTP 伴生端口，双地址都落 SOCKS。
	http, socks, err = sysproxyTargetsFromListen("socks5://0.0.0.0:11080")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if http != "0.0.0.0:11080" || socks != "0.0.0.0:11080" {
		t.Errorf("非回环兜底: http=%q socks=%q, want 均 0.0.0.0:11080", http, socks)
	}

	// 非法输入。
	if _, _, err := sysproxyTargetsFromListen("http://127.0.0.1:8080"); err == nil {
		t.Error("非 socks5:// 前缀应报错")
	}
	if _, _, err := sysproxyTargetsFromListen("socks5://127.0.0.1"); err == nil {
		t.Error("缺端口应报错")
	}
}

// TestSynthesizeFileConfigDualListen 验证合成 config 用的是追加后的监听串。
func TestSynthesizeFileConfigDualListen(t *testing.T) {
	p := DefaultProfile(1)
	p.ServerURL = "wss://example.com:443"
	p.Token = "tok"
	fc := synthesizeFileConfig(p, "geo", "rules.txt", false)
	listen, _ := fc["listen"].(string)
	if listen == "" || !containsSeg(listen, "http://127.0.0.1:11081") {
		t.Errorf("合成 listen 缺少 HTTP 监听: %q", listen)
	}
	if !containsSeg(listen, "socks5://127.0.0.1:11080") {
		t.Errorf("合成 listen 缺少 SOCKS5 监听: %q", listen)
	}
}

// TestSynthesizeFileConfigBaiduRelay 验证百度中转开关开/关时 websocket_front_proxy 注入与 dialIPs 联动。
func TestSynthesizeFileConfigBaiduRelay(t *testing.T) {
	// 关：不应注入 websocket_front_proxy，且 dialIPs 正常透传（回归）。
	off := DefaultProfile(1)
	off.DialIPs = "1.2.3.4"
	off.BaiduRelay = false
	fcOff := synthesizeFileConfig(off, "geo", "rules.txt", false)
	if _, ok := fcOff["websocket_front_proxy"]; ok {
		t.Error("开关关时不应注入 websocket_front_proxy")
	}
	if fcOff["ip"] != "1.2.3.4" {
		t.Errorf("开关关时 dialIPs 应透传: got %v", fcOff["ip"])
	}

	// 开：注入 front proxy，且 dialIPs 被强制忽略（内核坑规避）。
	on := DefaultProfile(1)
	on.DialIPs = "1.2.3.4"
	on.BaiduRelay = true
	fcOn := synthesizeFileConfig(on, "geo", "rules.txt", false)
	if _, ok := fcOn["ip"]; ok {
		t.Errorf("开关开时不应写 ip(dialIPs): got %v", fcOn["ip"])
	}
	raw, ok := fcOn["websocket_front_proxy"]
	if !ok {
		t.Fatal("开关开时应注入 websocket_front_proxy")
	}
	fwp, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("websocket_front_proxy 类型错误: %T", raw)
	}
	if fwp["enabled"] != true || fwp["type"] != "http_connect" {
		t.Errorf("enabled/type 不符: %v/%v", fwp["enabled"], fwp["type"])
	}
	if fwp["server"] != "cloudnproxy.baidu.com:443" {
		t.Errorf("默认 server 不符: %v", fwp["server"])
	}
	if fwp["connect_host"] != "sptest.baidu.com" {
		t.Errorf("默认 connect_host 不符: %v", fwp["connect_host"])
	}
	hdr, ok := fwp["headers"].(map[string]string)
	if !ok || hdr["X-T5-Auth"] != "482857715" {
		t.Errorf("headers 缺省值不符: %v", fwp["headers"])
	}

	// 自定义参数优先于默认。
	custom := DefaultProfile(1)
	custom.BaiduRelay = true
	custom.BaiduServer = "relay.example.com:8443"
	custom.BaiduConnectHost = ""
	custom.BaiduHeaders = map[string]string{"X-T5-Auth": "999"}
	fcC := synthesizeFileConfig(custom, "geo", "rules.txt", false)
	fcCwp := fcC["websocket_front_proxy"].(map[string]any)
	if fcCwp["server"] != "relay.example.com:8443" {
		t.Errorf("自定义 server 不符: %v", fcCwp["server"])
	}
	if _, ok := fcCwp["connect_host"]; ok {
		t.Error("connect_host 留空时不应写该字段")
	}
	if h := fcCwp["headers"].(map[string]string); h["X-T5-Auth"] != "999" {
		t.Errorf("自定义 headers 不符: %v", h)
	}
}

// TestValidateProfileBaiduRelay 验证开关开时的必填校验。
func TestValidateProfileBaiduRelay(t *testing.T) {
	p := DefaultProfile(1)
	p.Name = "x"
	p.LocalListen = "socks5://127.0.0.1:11080"
	p.BaiduRelay = true
	p.BaiduServer = ""
	if err := validateProfile(p); err == nil {
		t.Error("开+空 server 应报错")
	}
	p.BaiduServer = "badhost"
	if err := validateProfile(p); err == nil {
		t.Error("server 缺端口应报错")
	}
	p.BaiduServer = "host:443"
	if err := validateProfile(p); err != nil {
		t.Errorf("合法 host:port 不应报错: %v", err)
	}
	// 关时不校验。
	p.BaiduRelay = false
	p.BaiduServer = ""
	if err := validateProfile(p); err != nil {
		t.Errorf("关时不校验百度参数: %v", err)
	}
}

func containsSeg(listen, seg string) bool {
	for _, part := range splitComma(listen) {
		if part == seg {
			return true
		}
	}
	return false
}
func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
