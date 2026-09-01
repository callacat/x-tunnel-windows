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
	// control 端口固定用随机（-control 127.0.0.1:0），实际地址由 ready.json 回报，
	// 避免与旧实例/其他程序冲突。
	args := []string{
		"-config", cfgPath,
		"-l", m.listen,
		"-control", "127.0.0.1:0",
		"-ready-file", m.readyFile,
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
			m.state = "running"
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
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
			st.ListenAddr = m.listen
		}
	}
	return st
}

// controlStatus 轮询 control HTTP。ready.json 记录实际 control 地址。
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
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(ready.ControlURL + "/v1/status")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var cs ctlStatus
	if err := json.NewDecoder(resp.Body).Decode(&cs); err != nil {
		return nil
	}
	return &cs
}

// BinExists 检查 sidecar 二进制是否在位（GUI 启动时给前端明确提示）。
func (m *sidecarManager) BinExists() bool {
	_, err := os.Stat(m.binPath)
	return err == nil
}

// reloadRoute 触发 sidecar 规则热重载（control POST /v1/route/reload）。
func (m *sidecarManager) reloadRoute() error {
	url := m.controlBaseURL()
	if url == "" {
		return errors.New("sidecar 未运行")
	}
	resp, err := http.Post(url+"/v1/route/reload", "application/json", nil)
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
