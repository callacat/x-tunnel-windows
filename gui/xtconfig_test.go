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
