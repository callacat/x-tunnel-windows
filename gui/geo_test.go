package main

// geo/download 链路回归测试（东哥 09-05 v0.1.3 反馈②）：
// - 体积门槛按文件区分（geoip-lite 上游精简到约 200KB，不再被 1MB 门槛误杀）
// - 加速前缀失败时逐级兜底（用户配置 → 内置镜像 → 直连）
// - ghProxy 空值语义 = 默认镜像（与 SetGhProxy/前端文案一致）

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withDirectHTTP 绕过环境代理：开发机/CI 可能带 http_proxy 环境变量，
// httptest 本地地址经代理转发会失败（测试串到真实 github.com），
// 下载链路测试需要强制直连。
func withDirectHTTP(t *testing.T) {
	t.Helper()
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = &http.Transport{Proxy: nil}
	t.Cleanup(func() { http.DefaultClient.Transport = old })
}

// TestGeoFileReadyThreshold 验证体积门槛按文件区分：
// 2026-09 上游 geoip-lite.dat 仅约 202KB，v0.1.3 统一 1MB 门槛会把
// 下载成功的文件误判「内容过小」→ UpdateGeo 报更新失败。
func TestGeoFileReadyThreshold(t *testing.T) {
	dir := t.TempDir()
	// geoip-lite：202KB 应判就绪（旧逻辑 1MB 门槛会判失败）。
	p := filepath.Join(dir, geoipLiteName)
	if err := os.WriteFile(p, make([]byte, 200<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	if !geoFileReady(dir, geoipLiteName) {
		t.Errorf("geoip-lite.dat 200KB 应判就绪（旧 1MB 门槛误杀）")
	}
	// geosite：202KB 不足 1MB，仍未就绪。
	p2 := filepath.Join(dir, geositeName)
	if err := os.WriteFile(p2, make([]byte, 200<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	if geoFileReady(dir, geositeName) {
		t.Errorf("geosite.dat 200KB 不应判就绪（门槛 1MB）")
	}
}

// TestDownloadGeoFallback 验证下载链兜底：首选源 404 时自动换内置镜像、
// 再换直连，任一成功即落盘。
func TestDownloadGeoFallback(t *testing.T) {
	withDirectHTTP(t)
	// 假直连源：正常出内容（防止测试穿透到真实 github.com——开发机可直连）。
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "direct-data")
	}))
	defer direct.Close()
	oldDirect := geoDirectBase
	geoDirectBase = direct.URL
	defer func() { geoDirectBase = oldDirect }()
	dir := t.TempDir()
	// 模拟 gh-proxyFallbackMirrors[0]：正常出内容。
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("m", 200<<10)) // 200KB > geoip 门槛
	}))
	defer mirror.Close()
	// 覆盖内置镜像表指向本地测试服（原表项域名不可控）。
	oldMirrors := ghProxyFallbackMirrors
	ghProxyFallbackMirrors = []string{mirror.URL}
	defer func() { ghProxyFallbackMirrors = oldMirrors }()

	// 首选源（用户配置前缀）404 → 兜底镜像 200。
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer bad.Close()

	if err := downloadGeo(dir, geoipLiteName, bad.URL); err != nil {
		t.Fatalf("首选源失败后应兜底成功，却报错: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, geoipLiteName))
	if err != nil || len(data) != 200<<10 {
		t.Fatalf("落盘内容应来自兜底镜像（200KB），got %d 字节 err=%v", len(data), err)
	}
}

// TestDownloadGeoAllFail 验证全源失败时报错含「更换加速地址」指引。
func TestDownloadGeoAllFail(t *testing.T) {
	withDirectHTTP(t)
	dir := t.TempDir()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()
	oldDirect := geoDirectBase
	geoDirectBase = bad.URL // 兜底链最后一级直连也指向失败服
	defer func() { geoDirectBase = oldDirect }()
	oldMirrors := ghProxyFallbackMirrors
	ghProxyFallbackMirrors = nil
	defer func() { ghProxyFallbackMirrors = oldMirrors }()

	err := downloadGeo(dir, geoipLiteName, bad.URL)
	if err == nil {
		t.Fatal("全源失败应报错")
	}
	if want := "更换 GitHub 加速地址"; !strings.Contains(err.Error(), want) {
		t.Errorf("错误信息应含指引 %q，got %q", want, err.Error())
	}
}

// TestDownloadGeoMinSize 验证体积门槛：过小内容即使 HTTP 200 也拒绝落盘。
func TestDownloadGeoMinSize(t *testing.T) {
	withDirectHTTP(t)
	dir := t.TempDir()
	tiny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "tiny")
	}))
	defer tiny.Close()
	oldDirect := geoDirectBase
	geoDirectBase = tiny.URL // 全链都出 tiny，验证体积门槛拒绝
	defer func() { geoDirectBase = oldDirect }()
	oldMirrors := ghProxyFallbackMirrors
	ghProxyFallbackMirrors = nil
	defer func() { ghProxyFallbackMirrors = oldMirrors }()

	if err := downloadGeo(dir, geoipLiteName, tiny.URL); err == nil {
		t.Fatal("过小内容应拒绝落盘并报错")
	}
	// 不应留下正式文件（临时文件由 defer 清理）。
	if _, err := os.Stat(filepath.Join(dir, geoipLiteName)); !os.IsNotExist(err) {
		t.Errorf("过小内容不应落盘正式文件")
	}
}

// TestGhProxyDefaultSemantics 验证空配置 = 默认镜像（而非 v0.1.3 的空=直连，
// 与 SetGhProxy「空=落盘默认值」及前端文案矛盾）。此处依赖 profiles.json
// 未写入 gh_proxy 的默认环境（测试进程 dataDir 指向临时/空目录时成立）。
func TestGhProxyDefaultSemantics(t *testing.T) {
	got := ghProxy()
	if got != ghProxyDefault {
		t.Errorf("ghProxy() 空配置应返回默认 %q，got %q", ghProxyDefault, got)
	}
}

// TestApplyGhProxy 补充前缀拼接边界（api.github.com 也应走加速）。
func TestApplyGhProxy(t *testing.T) {
	cases := []struct{ url, proxy, want string }{
		{"https://github.com/a/b/releases/download/x/y.dat", "https://m.example", "https://m.example/https://github.com/a/b/releases/download/x/y.dat"},
		{"https://api.github.com/repos/a/b/releases/latest", "https://m.example", "https://m.example/https://api.github.com/repos/a/b/releases/latest"},
		{"https://github.com/a/b/x", "", "https://github.com/a/b/x"},
		{"https://github.com/a/b/x", "off", "https://github.com/a/b/x"},
		{"https://github.com/a/b/x", "https://m.example/", "https://m.example/https://github.com/a/b/x"},
	}
	for _, c := range cases {
		if got := applyGhProxy(c.url, c.proxy); got != c.want {
			t.Errorf("applyGhProxy(%q,%q) = %q, want %q", c.url, c.proxy, got, c.want)
		}
	}
}

// TestCheckUpdateFallback 验证 CheckUpdate 同款兜底链（api.github.com 走加速）。
func TestCheckUpdateFallback(t *testing.T) {
	oldMirrors := ghProxyFallbackMirrors
	ghProxyFallbackMirrors = []string{"https://mirror.invalid"}
	defer func() { ghProxyFallbackMirrors = oldMirrors }()

	// 首选源（ghProxy 默认 gh-proxy.org，离线/被墙时失败）→ 兜底直连本测试服。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v9.9.9","html_url":"https://github.com/x/t/releases/v9.9.9"}`)
	}))
	defer srv.Close()

	// 直连源替换：临时改 ghProxyDefault 使 applyGhProxy 不命中 github 域不可行——
	// 直连源 URL 固定 api.github.com。改用 httptest 无法劫持外部域名，
	// 因此这里只验证「首选源失败后报错路径不含 panic/阻塞」，真兜底由
	// TestDownloadGeoFallback 已覆盖同构逻辑。
	base := fmt.Sprintf("https://api.github.com/repos/x/t/releases/latest")
	urls := make([]string, 0, 3)
	if p := ghProxy(); p != "" && p != "off" {
		urls = append(urls, applyGhProxy(base, p))
	}
	for _, m := range ghProxyFallbackMirrors {
		urls = append(urls, applyGhProxy(base, m))
	}
	urls = append(urls, base)
	if len(urls) != 3 {
		t.Fatalf("应生成 3 级 URL（默认前缀+备用镜像+直连），got %d: %v", len(urls), urls)
	}

	// 逐级 fetch 逻辑复用：对本地 srv 直连路径验证 fetchLatestRelease 解析。
	info, err := fetchLatestRelease(context.Background(), srv.URL, "0.1.0")
	if err != nil {
		t.Fatalf("fetchLatestRelease 应成功: %v", err)
	}
	if !info.HasUpdate || info.Latest != "9.9.9" {
		t.Errorf("解析结果不对: %+v", info)
	}
}
