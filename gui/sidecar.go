package main

// sidecar 子进程管理：x-tunnel 客户端二进制以外部进程方式运行（方案A 核心决策①）。
// GUI 只负责：生成配置 → 拉起进程 → 等 ready-file → 轮询 control HTTP → 停止/崩溃回收。
// sidecar 零改动（flag 面：-config/-l/-control/-ready-file/-route-enabled 等，见
// x-tunnel internal/app/config.go）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SidecarStatus 是 control HTTP /v1/status 与本地进程状态的合成快照。
// 字段与 Android XTunnelRuntimeManager.RouteStatus/RuntimeSnapshot 语义对齐，
// 前端可复用 Android 的展示范式。
type SidecarStatus struct {
	State      string `json:"state"` // stopped|starting|running|failed
	PID        int    `json:"pid,omitempty"`
	ListenAddr string `json:"listenAddr,omitempty"`
	ControlURL string `json:"controlUrl,omitempty"`
	Version    string `json:"version,omitempty"`
	LastError  string `json:"lastError,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	// GEO/分流运行态（/v1/route/stats，分流引擎关闭时为空）
	RouteEnabled bool   `json:"routeEnabled,omitempty"`
	RuleCount    int    `json:"ruleCount,omitempty"`
	ProxyHits    int64  `json:"proxyHits,omitempty"`
	DirectHits   int64  `json:"directHits,omitempty"`
	RejectedHits int64  `json:"rejectedHits,omitempty"`
	SiteLoaded   bool   `json:"siteLoaded,omitempty"`
	IPLoaded     bool   `json:"ipLoaded,omitempty"`
	Fallback     string `json:"fallback,omitempty"`
	// 流量统计（/v1/stats）
	BytesSent int64 `json:"bytesSent,omitempty"`
	BytesRecv int64 `json:"bytesRecv,omitempty"`
}

// sidecarManager 管理一个 x-tunnel 子进程的生命周期。
type sidecarManager struct {
	mu           sync.Mutex
	binPath      string // x-tunnel.exe 绝对路径
	workDir      string // 工作目录（config.json/geo/日志落这里）
	listen       string // socks5 监听 127.0.0.1:<port>
	control      string // control 监听 127.0.0.1:<port>（0=随机，ready 后从 ready-file 读）
	readyFile    string
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	startedAt    time.Time
	lastErr      string
	state        string
	controlToken string
}

func newSidecarManager(binPath, workDir, listen string) *sidecarManager {
	return &sidecarManager{
		binPath:   binPath,
		workDir:   workDir,
		listen:    listen,
		readyFile: filepath.Join(workDir, "ready.json"),
	}
}

// Start 拉起 sidecar（幂等）。readyTimeout 内等 ready.json 出现（sidecar
// -ready-file 在数据面就绪后原子写出，Android XTunnelRuntimeManager 同款语义）。
func (m *sidecarManager) Start(cfgPath string, extraArgs []string, readyTimeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == "running" || m.state == "starting" {
		return nil
	}
	_ = os.Remove(m.readyFile)
	// control 鉴权：显式指定 token 文件（GUI 侧可读）。core 无条件生成
	// 64hex token 强制 Bearer 鉴权；不传则 token 落系统临时目录拿不到，
	// /v1/status 等端点全部 401（状态轮询全瞎）。
	tokenPath := filepath.Join(m.workDir, "control-token")
	_ = os.Remove(tokenPath)
	// control 端口固定用随机（-control 127.0.0.1:0），实际地址由 ready.json 回报，
	// 避免与旧实例/其他程序冲突。
	args := []string{
		"-config", cfgPath,
		"-control", "127.0.0.1:0",
		"-ready-file", m.readyFile,
		"-control-token-file", tokenPath,
	}
	// listen 只在显式指定时才传 -l：core 的合并规则是「命令行出现的 flag
	// 覆盖 config」（applyStringConfig + flag.Visit），空串 -l 也会顶掉
	// config 里的双监听（socks5+http）→ 系统代理 HTTP 段失效。
	if m.listen != "" {
		args = append(args, "-l", m.listen)
	}
	args = append(args, extraArgs...)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, m.binPath, args...)
	cmd.Dir = m.workDir
	// sidecar 的 stdout/stderr 不直接进 GUI 日志环（控制面日志走 control/ready-file；
	// 数据面日志由 GUI 轮询时按需拉），先丢到工作目录文件便于诊断。
	logF, err := os.Create(filepath.Join(m.workDir, "sidecar.log"))
	if err == nil {
		cmd.Stdout = logF
		cmd.Stderr = logF
	}
	if err := cmd.Start(); err != nil {
		cancel()
		m.state = "failed"
		m.lastErr = fmt.Sprintf("拉起 sidecar 失败: %v", err)
		return errors.New(m.lastErr)
	}
	m.cmd = cmd
	m.cancel = cancel
	m.startedAt = time.Now()
	m.state = "starting"
	m.lastErr = ""

	// 守护 goroutine：等退出或 ready。
	go func(pid int) {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		if logF != nil {
			_ = logF.Close()
		}
		if m.state != "stopped" { // 非 GUI 主动停止 = 崩溃/异常退出
			m.state = "failed"
			if err != nil {
				m.lastErr = fmt.Sprintf("sidecar 退出: %v", err)
			}
		}
	}(cmd.Process.Pid)

	// 等 ready-file（不持锁，Start 的锁由 defer 释放前完成等待——ready 慢时
	// 前端 Start 调用会阻塞，但语义更简单：返回即知道结果。超时按 starting 继续
	// 后台等（control 轮询会翻 running）。
	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(m.readyFile); err == nil {
			m.readControlToken(tokenPath)
			m.state = "running"
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

// readControlToken 读 control token 文件（core 写 hex token + "\n"，strip 尾换行）。
func (m *sidecarManager) readControlToken(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	m.controlToken = strings.TrimRight(string(data), "\r\n")
}

// Stop 停止 sidecar（幂等；Windows 无 SIGTERM，直接 Kill——sidecar 无需优雅
// 退出语义，配置都是先写文件再启动）。
func (m *sidecarManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == "stopped" || m.cmd == nil {
		m.state = "stopped"
		return nil
	}
	m.state = "stopped"
	if m.cancel != nil {
		m.cancel()
	}
	_ = m.cmd.Process.Kill()
	m.cmd = nil
	return nil
}

// Status 返回合成状态（进程态 + control HTTP 轮询）。
func (m *sidecarManager) Status() SidecarStatus {
	m.mu.Lock()
	st := SidecarStatus{State: m.state, LastError: m.lastErr}
	cmd := m.cmd
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		st.PID = cmd.Process.Pid
	}
	if st.State == "running" || st.State == "starting" {
		if c := m.controlStatus(); c != nil {
			st.ControlURL = c.ControlURL
			st.Version = c.Version
			st.RouteEnabled = c.RouteEnabled
			st.RuleCount = c.RuleCount
			st.ProxyHits = c.ProxyHits
			st.DirectHits = c.DirectHits
			st.RejectedHits = c.RejectedHits
			st.SiteLoaded = c.SiteLoaded
			st.IPLoaded = c.IPLoaded
			st.Fallback = c.Fallback
			st.BytesSent = c.BytesSent
			st.BytesRecv = c.BytesRecv
			st.ListenAddr = m.listen
		}
	}
	return st
}

// controlStatus 轮询 control HTTP。ready.json 记录实际 control 地址。
// /v1/status 提供 Version（mode/started_at 等 GUI 暂不消费）；
// GEO/分流运行态在独立端点 /v1/route/stats（{enabled,stats{proxy,direct,
// rejected,miss},geo{site_loaded,ip_loaded,rule_count,fallback}}）；
// 流量在 /v1/stats 的 traffic 段。三者均需 Bearer 鉴权。
type ctlStatus struct {
	ControlURL   string `json:"control_url"`
	Version      string `json:"version"`
	RouteEnabled bool   `json:"route_enabled"`
	RuleCount    int    `json:"rule_count"`
	ProxyHits    int64  `json:"proxy_hits"`
	DirectHits   int64  `json:"direct_hits"`
	RejectedHits int64  `json:"rejected_hits"`
	SiteLoaded   bool   `json:"site_loaded"`
	IPLoaded     bool   `json:"ip_loaded"`
	Fallback     string `json:"fallback"`
	BytesSent    int64  `json:"bytes_sent"`
	BytesRecv    int64  `json:"bytes_recv"`
}

func (m *sidecarManager) controlStatus() *ctlStatus {
	raw, err := os.ReadFile(m.readyFile)
	if err != nil {
		return nil
	}
	var ready struct {
		ControlURL string `json:"control_url"`
		PID        int    `json:"pid"`
	}
	if err := json.Unmarshal(raw, &ready); err != nil {
		return nil
	}
	cs := &ctlStatus{ControlURL: ready.ControlURL}

	client := &http.Client{Timeout: 2 * time.Second}
	get := func(path string, out any) bool {
		req, err := http.NewRequest(http.MethodGet, ready.ControlURL+path, nil)
		if err != nil {
			return false
		}
		req.Header.Set("Authorization", "Bearer "+m.controlToken)
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		return json.NewDecoder(resp.Body).Decode(out) == nil
	}

	// 1) /v1/status → Version。
	var st struct {
		Version string `json:"version"`
	}
	if !get("/v1/status", &st) {
		return nil // 进程态由 manager 自身字段兜底，control 不通视为未就绪
	}
	cs.Version = st.Version

	// 2) /v1/route/stats → GEO/分流运行态（未启用分流时 enabled=false，字段全零）。
	var rs struct {
		Enabled bool `json:"enabled"`
		Stats   struct {
			Proxy    int64 `json:"proxy"`
			Direct   int64 `json:"direct"`
			Rejected int64 `json:"rejected"`
		} `json:"stats"`
		Geo struct {
			SiteLoaded bool   `json:"site_loaded"`
			IPLoaded   bool   `json:"ip_loaded"`
			RuleCount  int    `json:"rule_count"`
			Fallback   string `json:"fallback"`
		} `json:"geo"`
	}
	if get("/v1/route/stats", &rs) {
		cs.RouteEnabled = rs.Enabled
		cs.ProxyHits = rs.Stats.Proxy
		cs.DirectHits = rs.Stats.Direct
		cs.RejectedHits = rs.Stats.Rejected
		cs.SiteLoaded = rs.Geo.SiteLoaded
		cs.IPLoaded = rs.Geo.IPLoaded
		cs.RuleCount = rs.Geo.RuleCount
		cs.Fallback = rs.Geo.Fallback
	}

	// 3) /v1/stats → 流量增量（诊断包与状态卡展示）。
	var stats struct {
		Traffic struct {
			BytesSent     int64 `json:"bytes_sent"`
			BytesReceived int64 `json:"bytes_received"`
		} `json:"traffic"`
	}
	if get("/v1/stats", &stats) {
		cs.BytesSent = stats.Traffic.BytesSent
		cs.BytesRecv = stats.Traffic.BytesReceived
	}
	return cs
}

// BinExists 检查 sidecar 二进制是否在位（GUI 启动时给前端明确提示）。
func (m *sidecarManager) BinExists() bool {
	_, err := os.Stat(m.binPath)
	return err == nil
}

// reloadRoute 触发 sidecar 规则热重载（control POST /v1/rules/reload，
// Bearer 鉴权——端点名与 core internal/app/control.go 路由表一致）。
func (m *sidecarManager) reloadRoute() error {
	url := m.controlBaseURL()
	if url == "" {
		return errors.New("sidecar 未运行")
	}
	req, err := http.NewRequest(http.MethodPost, url+"/v1/rules/reload", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.controlToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("reload HTTP %d", resp.StatusCode)
	}
	return nil
}

// tailLog 返回 sidecar.log 尾部（诊断页展示）。
func (m *sidecarManager) tailLog(limit int) (string, error) {
	data, err := os.ReadFile(filepath.Join(m.workDir, "sidecar.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n"), nil
}

// controlBaseURL 从 ready.json 读 control 地址（sidecar 未就绪返回空）。
func (m *sidecarManager) controlBaseURL() string {
	raw, err := os.ReadFile(m.readyFile)
	if err != nil {
		return ""
	}
	var ready struct {
		ControlURL string `json:"control_url"`
	}
	if json.Unmarshal(raw, &ready) != nil || ready.ControlURL == "" {
		return ""
	}
	return ready.ControlURL
}
