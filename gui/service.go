package main

// Service 是暴露给 Wails 前端的服务层（x-tunnel Windows GUI 版）。
// 与 warp-go 版的差异：核心不是库内嵌而是 sidecar 子进程（sidecar.go），
// 配置模型是 x-tunnel Profile（xtconfig.go）。方法面按 M1/M2 里程碑裁剪：
// 无注册（x-tunnel 无账号体系）、无边缘扫描、无分应用代理。

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/callacat/x-tunnel-windows/gui/sysproxy"
)

// Status 是前端状态快照（GetStatus 返回）。
type Status struct {
	State        string `json:"state"`
	ListenAddr   string `json:"listen_addr"`
	ControlURL   string `json:"control_url"`
	Version      string `json:"version"`
	LastError    string `json:"last_error"`
	StartedAt    string `json:"started_at"`
	SysProxyOn   bool   `json:"sys_proxy_on"`
	InitDone     bool   `json:"init_done"`
	SidecarOK    bool   `json:"sidecar_ok"`  // x-tunnel.exe 在位
	ActiveName   string `json:"active_name"` // 当前激活配置名
	Configured   bool   `json:"configured"`  // 已配置服务器
	RouteEnabled bool   `json:"route_enabled,omitempty"`
	RuleCount    int    `json:"rule_count,omitempty"`
	ProxyHits    int64  `json:"proxy_hits,omitempty"`
	DirectHits   int64  `json:"direct_hits,omitempty"`
	RejectedHits int64  `json:"rejected_hits,omitempty"`
	SiteLoaded   bool   `json:"site_loaded,omitempty"`
	IPLoaded     bool   `json:"ip_loaded,omitempty"`
	BytesSent    int64  `json:"bytes_sent,omitempty"`
	BytesRecv    int64  `json:"bytes_recv,omitempty"`
}

// Service 持有 sidecar 管理器与配置目录。
type Service struct {
	mu           sync.Mutex
	sidecar      *sidecarManager
	started      bool
	startErr     error
	defaultsInit bool
}

// dataDir 返回工作目录（Windows: %APPDATA%/x-tunnel-windows；其他平台回退 ~/.x-tunnel-windows）。
func dataDir() string {
	switch runtime.GOOS {
	case "windows":
		if ad := os.Getenv("APPDATA"); ad != "" {
			return filepath.Join(ad, "x-tunnel-windows")
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".x-tunnel-windows")
}

func newService() *Service {
	svc := &Service{}
	initLogging()
	_ = os.MkdirAll(dataDir(), 0o755)
	svc.sidecar = newSidecarManager(
		filepath.Join(execDir(), "x-tunnel.exe"),
		dataDir(),
		"", // listen 由激活配置决定
	)
	// GUI 启动即异步初始化 GEO（不阻塞窗口显示）。
	go svc.InitDefaults()
	return svc
}

// execDir 返回 GUI 可执行文件所在目录（x-tunnel.exe 同目录部署）。
func execDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func initLogging() {
	log.SetOutput(logWriter{})
	log.SetFlags(0)
}

// InitDefaults 初始化 GEO 数据库（幂等；失败不阻塞 GUI，状态页可手动重试）。
func (s *Service) InitDefaults() {
	s.mu.Lock()
	if s.defaultsInit {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if err := updateGeoFiles(false); err != nil {
		log.Printf("⚠ GEO 初始化失败（可稍后在 GEO 页手动更新）：%v", err)
		return
	}
	s.mu.Lock()
	s.defaultsInit = true
	s.mu.Unlock()
	log.Println("✓ 初始化完成（GEO 数据库已就绪）")
}

// GetStatus 返回状态快照。
func (s *Service) GetStatus() Status {
	s.mu.Lock()
	initDone := s.defaultsInit
	startErr := s.startErr
	started := s.started
	s.mu.Unlock()

	st := Status{
		State:     "stopped",
		InitDone:  initDone,
		SidecarOK: s.sidecar.BinExists(),
	}
	if startErr != nil {
		st.LastError = startErr.Error()
	}
	if !st.SidecarOK {
		st.LastError = "未找到 x-tunnel.exe（应与 GUI 同目录部署）"
	}

	// 激活配置信息
	p, err := s.ActiveProfile()
	if err == nil && p != nil {
		st.ActiveName = p.Name
		st.Configured = strings.TrimSpace(p.ServerURL) != ""
		st.ListenAddr = p.LocalListen
	}

	if started {
		ss := s.sidecar.Status()
		st.State = ss.State
		st.ControlURL = ss.ControlURL
		st.Version = ss.Version
		st.RouteEnabled = ss.RouteEnabled
		st.RuleCount = ss.RuleCount
		st.ProxyHits = ss.ProxyHits
		st.DirectHits = ss.DirectHits
		st.RejectedHits = ss.RejectedHits
		st.SiteLoaded = ss.SiteLoaded
		st.IPLoaded = ss.IPLoaded
		if ss.LastError != "" && st.LastError == "" {
			st.LastError = ss.LastError
		}
	}
	if on, _ := s.sysProxyOnForActive(); on {
		st.SysProxyOn = true
	}
	return st
}

// Start 启动 sidecar（用激活配置合成 config.json；幂等）。
func (s *Service) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	p, err := s.ActiveProfile()
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("尚未配置服务器：请先在「配置」页添加并选择一个配置")
	}
	if err := validateProfile(*p); err != nil {
		return fmt.Errorf("配置无效：%w", err)
	}
	if !s.sidecar.BinExists() {
		return errors.New("未找到 x-tunnel.exe（应与 GUI 同目录部署）")
	}

	// 合成 config.json（geo/rules 路径锚定 dataDir）
	dir := dataDir()
	fc := synthesizeFileConfig(*p, filepath.Join(dir, "geo"), filepath.Join(dir, "rules.txt"), true)
	cfgData, _ := jsonMarshalIndent(fc)
	cfgPath := filepath.Join(dir, "config.json")
	if err := atomicWriteFile(cfgPath, cfgData); err != nil {
		return fmt.Errorf("写入配置失败：%w", err)
	}

	s.mu.Lock()
	s.started = true
	s.startErr = nil
	s.mu.Unlock()

	// sidecar listen 用配置里的本地监听（去掉 socks5:// 前缀给 -l 也可以，
	// 但 sidecar -l 本身接受 socks5:// 全格式——直接透传）。
	go func() {
		err := s.sidecar.Start(cfgPath, nil, 20*time.Second)
		s.mu.Lock()
		if err != nil {
			s.startErr = err
			log.Printf("sidecar 启动失败：%v", err)
		}
		s.mu.Unlock()
	}()
	log.Printf("正在启动：%s（%s）", p.Name, p.ServerURL)
	return nil
}

// Stop 停止 sidecar 并还原系统代理（幂等）。
func (s *Service) Stop() error {
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()
	// 停止时还原系统代理（防止残留指向已死的 socks5 端口）。
	_ = sysproxy.Set("", false)
	return s.sidecar.Stop()
}

// IsRunning 报告 sidecar 是否运行中。
func (s *Service) IsRunning() bool {
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if !started {
		return false
	}
	ss := s.sidecar.Status()
	return ss.State == "running" || ss.State == "starting"
}

// ---------------------------------------------------------------------------
// 配置管理（多 Profile）
// ---------------------------------------------------------------------------

// ListProfiles 返回全部配置与激活名。
func (s *Service) ListProfiles() (AppProfiles, error) {
	return loadProfiles(dataDir()), nil
}

// SaveProfiles 保存配置列表与激活项（前端编辑后整体回写；逐条校验）。
func (s *Service) SaveProfiles(p AppProfiles) error {
	names := map[string]bool{}
	for i := range p.Profiles {
		if err := validateProfile(p.Profiles[i]); err != nil {
			return fmt.Errorf("配置「%s」：%w", p.Profiles[i].Name, err)
		}
		if names[p.Profiles[i].Name] {
			return fmt.Errorf("配置名重复：%s", p.Profiles[i].Name)
		}
		names[p.Profiles[i].Name] = true
	}
	if p.ActiveName != "" && !names[p.ActiveName] {
		return fmt.Errorf("激活配置不存在：%s", p.ActiveName)
	}
	return saveProfiles(dataDir(), p)
}

// ActiveProfile 返回激活配置（无配置返回 nil 不报错）。
func (s *Service) ActiveProfile() (*XTunnelProfile, error) {
	p := loadProfiles(dataDir())
	if len(p.Profiles) == 0 {
		return nil, nil
	}
	for i := range p.Profiles {
		if p.Profiles[i].Name == p.ActiveName {
			return &p.Profiles[i], nil
		}
	}
	return &p.Profiles[0], nil
}

// ---------------------------------------------------------------------------
// 路由规则
// ---------------------------------------------------------------------------

// GetRules 读 rules.txt。
func (s *Service) GetRules() (string, error) {
	data, err := os.ReadFile(filepath.Join(dataDir(), "rules.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return defaultRules(), nil
		}
		return "", err
	}
	return string(data), nil
}

// SaveRules 校验并写回 rules.txt。
// 语法校验走 sidecar 的规则解析（M1 先做行格式基本校验，M3 接 control 热重载）。
func (s *Service) SaveRules(rulesText string) error {
	if err := validateRulesText(rulesText); err != nil {
		return fmt.Errorf("规则语法错误：%w", err)
	}
	return atomicWriteFile(filepath.Join(dataDir(), "rules.txt"), []byte(rulesText))
}

// ReloadRules 触发 sidecar 热重载（control POST /v1/route/reload）。
func (s *Service) ReloadRules() error {
	return s.sidecar.reloadRoute()
}

// validateRulesText 基本行格式校验（行为,条件）。
func validateRulesText(text string) error {
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			return fmt.Errorf("第 %d 行格式错误（应为「行为,条件」）：%s", i+1, line)
		}
		switch strings.TrimSpace(parts[0]) {
		case "proxy", "direct", "reject":
		default:
			return fmt.Errorf("第 %d 行行为必须是 proxy/direct/reject：%s", i+1, parts[0])
		}
	}
	return nil
}

func defaultRules() string {
	return `# x-tunnel 路由规则（每行"行为,条件"；# 开头为注释）
# 行为：proxy 走隧道 / direct 直连 / reject 拦截
# 条件：domain 后缀 / full 精确 / cidr 网段 / geosite: 分类 / geoip: 国家码
direct,cn
direct,geosite:cn
direct,geoip:private
proxy,geosite:geolocation-!cn
`
}

// ---------------------------------------------------------------------------
// GEO 数据库
// ---------------------------------------------------------------------------

// GeoInfo 是 GEO 页展示快照。
type GeoInfo struct {
	GeositePath    string `json:"geosite_path"`
	GeoIPPath      string `json:"geoip_path"`
	GeositeUpdated string `json:"geosite_updated,omitempty"`
	GeoIPUpdated   string `json:"geoip_updated,omitempty"`
	Repository     string `json:"repository"`
	BaseURL        string `json:"base_url"`
}

// GetGeo 返回 GEO 数据库状态。
func (s *Service) GetGeo() (GeoInfo, error) {
	dir := filepath.Join(dataDir(), "geo")
	info := GeoInfo{
		Repository: "https://github.com/MetaCubeX/meta-rules-dat",
		BaseURL:    "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest",
	}
	info.GeositePath = filepath.Join(dir, "geosite.dat")
	info.GeoIPPath = filepath.Join(dir, "geoip-lite.dat")
	if fi, err := os.Stat(info.GeositePath); err == nil {
		info.GeositeUpdated = fi.ModTime().Format("2006-01-02 15:04")
	}
	if fi, err := os.Stat(info.GeoIPPath); err == nil {
		info.GeoIPUpdated = fi.ModTime().Format("2006-01-02 15:04")
	}
	return info, nil
}

// UpdateGeo 立即更新 GEO 数据库（下载走系统直连）。
func (s *Service) UpdateGeo() UpdateGeoResult {
	if err := updateGeoFiles(true); err != nil {
		return UpdateGeoResult{OK: false, Message: "更新失败：" + err.Error()}
	}
	return UpdateGeoResult{OK: true, Message: "GEO 数据已更新"}
}

// UpdateGeoResult 是手动更新 GEO 的结果。
type UpdateGeoResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// 系统代理
// ---------------------------------------------------------------------------

// SetSystemProxy 开启/关闭系统代理（指向激活配置的本地监听）。
// 开启且未运行时自动启动。
func (s *Service) SetSystemProxy(enabled bool) error {
	if enabled {
		if !s.IsRunning() {
			if err := s.Start(); err != nil {
				return fmt.Errorf("自动启动失败：%w", err)
			}
			// 等 sidecar 监听就绪再挂系统代理。
			deadline := time.Now().Add(25 * time.Second)
			for time.Now().Before(deadline) {
				ss := s.sidecar.Status()
				if ss.State == "running" {
					break
				}
				time.Sleep(300 * time.Millisecond)
			}
		}
		p, err := s.ActiveProfile()
		if err != nil || p == nil {
			return errors.New("无激活配置")
		}
		addr, err := sysproxyAddrFromListen(p.LocalListen)
		if err != nil {
			return err
		}
		return sysproxy.Set(addr, true)
	}
	return sysproxy.Set("", false)
}

// GetSystemProxyEnabled 报告系统代理当前是否开启（指向本地任一激活监听）。
func (s *Service) GetSystemProxyEnabled() bool {
	p, err := s.ActiveProfile()
	if err != nil || p == nil {
		return false
	}
	addr, err := sysproxyAddrFromListen(p.LocalListen)
	if err != nil {
		return false
	}
	on, _ := sysproxy.Enabled(addr)
	return on
}

// sysproxyAddrFromListen 把 socks5://127.0.0.1:11080 转成 host:port。
func sysproxyAddrFromListen(listen string) (string, error) {
	s := strings.TrimPrefix(listen, "socks5://")
	if s == "" || !strings.Contains(s, ":") {
		return "", fmt.Errorf("监听地址缺少端口：%s", listen)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// 开机自启 / 日志 / 版本
// ---------------------------------------------------------------------------

// SetAutostart 开关开机自启（Windows 注册表 Run 键）。
func (s *Service) SetAutostart(enabled bool) error { return setAutostart(enabled) }

// GetAutostartEnabled 查询自启状态。
func (s *Service) GetAutostartEnabled() bool { return autostartEnabled() }

// GetLogs 返回日志环（最近 limit 条）。
func (s *Service) GetLogs(limit int) []LogEntry { return getLogs(limit) }

// ClearLogs 清空日志环。
func (s *Service) ClearLogs() { clearLogs() }

// GetVersion 返回 GUI 版本号（单源：CI 注入；见 version.go）。
func (s *Service) GetVersion() string { return VersionString() }

// UpdateInfo 是检查更新结果。
type UpdateInfo struct {
	HasUpdate bool   `json:"has_update"`
	Latest    string `json:"latest"`
	Current   string `json:"current"`
	URL       string `json:"url"`
}

// CheckUpdate 查询 GitHub Releases 最新版本（GUI 仓库）。
func (s *Service) CheckUpdate() (*UpdateInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return checkGitHubUpdate(ctx, "callacat/x-tunnel-windows", strings.TrimPrefix(VersionString(), "v"))
}

// OpenExternalBrowser 用系统默认浏览器打开 URL。
func (s *Service) OpenExternalBrowser(url string) error { return openBrowser(url) }

// SidecarLog 返回 sidecar 日志尾部（诊断用）。
func (s *Service) SidecarLog(limit int) (string, error) {
	return s.sidecar.tailLog(limit)
}

// sysProxyOnForActive 查询系统代理是否指向当前激活监听。
func (s *Service) sysProxyOnForActive() (bool, error) {
	p, err := s.ActiveProfile()
	if err != nil || p == nil {
		return false, err
	}
	addr, err := sysproxyAddrFromListen(p.LocalListen)
	if err != nil {
		return false, err
	}
	return sysproxy.Enabled(addr)
}
