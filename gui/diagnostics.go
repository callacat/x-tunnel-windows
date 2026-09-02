package main

// diagnostics.go 诊断包导出（M3）：zip 内 diagnostics.json + logs.txt +
// sidecar.log。语义平移 Android DiagnosticExporter——导出可反馈的完整现场，
// token/服务器地址一律脱敏（profiles 只导名字清单）。

import (
	"archive/zip"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ExportDiagnostics 导出诊断包到用户下载目录，返回文件绝对路径。
// 内容：diagnostics.json（状态/GEO/流量/进程）+ logs.txt（GUI 日志环）+
// sidecar.log 尾部。失败返回空串与错误。
func (s *Service) ExportDiagnostics() (string, error) {
	s.mu.Lock()
	started := s.started
	startErr := s.startErr
	s.mu.Unlock()

	st := s.GetStatus() // 含 control 轮询（运行中才有运行态字段）

	diag := map[string]any{
		"collected_at": time.Now().Format(time.RFC3339),
		"gui_version":  VersionString(),
		"os":           runtime.GOOS + "/" + runtime.GOARCH,
		"state":        st.State,
		"sidecar_ok":   st.SidecarOK,
		"active_name":  st.ActiveName,
		"configured":   st.Configured,
		"sys_proxy_on": st.SysProxyOn,
		"listen_addr":  st.ListenAddr,
	}
	if st.LastError != "" {
		diag["last_error"] = st.LastError
	}
	if startErr != nil {
		diag["start_error"] = startErr.Error()
	}
	if st.ControlURL != "" {
		diag["control_url"] = st.ControlURL
	}
	if started {
		diag["core_version"] = st.Version
		diag["route_enabled"] = st.RouteEnabled
		diag["rule_count"] = st.RuleCount
		diag["hits"] = map[string]int64{
			"proxy": st.ProxyHits, "direct": st.DirectHits, "rejected": st.RejectedHits,
		}
		diag["geo"] = map[string]bool{"site_loaded": st.SiteLoaded, "ip_loaded": st.IPLoaded}
		diag["traffic"] = map[string]int64{"bytes_sent": st.BytesSent, "bytes_received": st.BytesRecv}
	}

	// 配置清单（仅名字，绝不写 token/服务器地址）。
	profiles := loadProfiles(dataDir())
	names := make([]string, 0, len(profiles.Profiles))
	for _, p := range profiles.Profiles {
		names = append(names, p.Name)
	}
	diag["profiles"] = names

	dir := downloadDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建导出目录失败：%w", err)
	}
	out := filepath.Join(dir, fmt.Sprintf("x-tunnel-diag-%s.zip", time.Now().Format("20060102-150405")))
	sidecarLog, _ := s.sidecar.tailLog(400)
	if err := writeDiagZip(out, diag, getLogs(0), sidecarLog); err != nil {
		return "", err
	}
	log.Printf("✓ 诊断包已导出：%s", out)
	return out, nil
}

// writeDiagZip 打包诊断 JSON 与日志为 zip（先写临时文件再 rename，避免半包）。
func writeDiagZip(out string, diag map[string]any, entries []LogEntry, sidecarLog string) error {
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // 失败时清理；成功 rename 后无害

	zw := zip.NewWriter(f)

	dj, err := jsonMarshalIndent(diag)
	if err != nil {
		f.Close()
		return err
	}
	if w, err := zw.Create("diagnostics.json"); err == nil {
		_, _ = w.Write(dj)
	}

	if w, err := zw.Create("logs.txt"); err == nil {
		var b strings.Builder
		for _, e := range entries {
			fmt.Fprintf(&b, "%s [%s] %s\n", e.Time, e.Level, e.Msg)
		}
		_, _ = w.Write([]byte(b.String()))
	}

	if w, err := zw.Create("sidecar.log"); err == nil {
		_, _ = w.Write([]byte(sidecarLog))
	}

	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, out)
}

// downloadDir 导出目录：用户下载目录（回退 dataDir/diagnostics）。
func downloadDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		dl := filepath.Join(home, "Downloads")
		if fi, err := os.Stat(dl); err == nil && fi.IsDir() {
			return dl
		}
	}
	return filepath.Join(dataDir(), "diagnostics")
}
