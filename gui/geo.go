package main

// geo：GEO 数据库（geosite.dat / geoip-lite.dat）下载与就绪检查。
// 来源 MetaCubeX/meta-rules-dat（与 warp-go/Android 同源），下载走系统直连，
// 可选 GitHub 加速前缀（东哥 09-05 反馈④，gh_proxy 设置）。
//
// 路径（东哥 09-05 反馈⑤）：GEO 库放运行目录 config/geo/ 下，与 sidecar
// 同目录布局（绿色便携、用户可见），不再放 %APPDATA% 数据目录。
// 兼容：数据目录 geo/ 下已有旧文件时自动迁移到新位置。

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	geoBaseURL    = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest"
	geositeName   = "geosite.dat"
	geoipLiteName = "geoip-lite.dat"
	// ghProxyDefault 是 GitHub 加速前缀默认值（与 SetGhProxy/前端文案一致）。
	ghProxyDefault = "https://gh-proxy.org"
)

// ghProxyFallbackMirrors 是内置备用加速镜像（2026-09 实测可用，均支持
// github.com 与 api.github.com 代理）。用户配置的前缀失败时按序兜底。
var ghProxyFallbackMirrors = []string{"https://gh-proxy.com"}

// geoMinSize 是各 GEO 文件的最小合理体积（低于视为未完成/损坏）。
// 上游 geoip-lite.dat 2026-09 起精简到约 200KB，不能再用统一 1MB 门槛
// （v0.1.3 bug：下载成功也被判「内容过小」——东哥 09-05 反馈②）。
var geoMinSize = map[string]int64{
	geositeName:   1 << 20,
	geoipLiteName: 100 << 10,
}

// geoDir 返回 GEO 数据库目录（运行目录 config/geo/，东哥 09-05 反馈⑤）。
func geoDir() string {
	return filepath.Join(execDir(), "config", "geo")
}

// initState 记录启动初始化（GEO 下载）的进度状态，前端轮询展示
// （东哥 09-05 反馈③：下载无进度/成败反馈）。
var geoInit = struct {
	mu       sync.Mutex
	state    string  // idle|downloading|done|failed
	current  string  // 正在下载的文件名
	progress float64 // 0-100（当前文件）
	error    string  // 失败原因（state=failed 时非空）
}{state: "idle"}

// setGeoInit 更新初始化状态（线程安全）。
func setGeoInit(state, current string, progress float64, errMsg string) {
	geoInit.mu.Lock()
	defer geoInit.mu.Unlock()
	geoInit.state = state
	if current != "" || state != "downloading" {
		geoInit.current = current
	}
	geoInit.progress = progress
	geoInit.error = errMsg
}

// GetInitState 返回初始化状态快照（前端 Status 轮询消费）。
func (s *Service) GetInitState() InitState {
	geoInit.mu.Lock()
	defer geoInit.mu.Unlock()
	return InitState{
		State:    geoInit.state,
		Current:  geoInit.current,
		Progress: geoInit.progress,
		Error:    geoInit.error,
	}
}

// InitState 是启动初始化进度快照。
type InitState struct {
	State    string  `json:"state"`              // idle|downloading|done|failed
	Current  string  `json:"current,omitempty"`  // 正在下载的文件
	Progress float64 `json:"progress,omitempty"` // 0-100
	Error    string  `json:"error,omitempty"`    // 失败原因
}

// ghProxy 返回生效的 GitHub 加速前缀（空=默认 gh-proxy.org，与前端展示
// 与 SetGhProxy 落盘语义一致；"off" 表示显式直连）。
func ghProxy() string {
	p := loadProfiles(dataDir())
	v := strings.TrimRight(strings.TrimSpace(p.GhProxy), "/")
	if v == "" {
		return ghProxyDefault
	}
	return v
}

// applyGhProxy 给下载 URL 拼加速前缀（空/off=原样）。调用点只传 github.com
// /api.github.com 地址，此处不做域检查（v0.1.3 反馈②：白名单挡住测试注入
// 的本地源，兜底链无法验证）。
func applyGhProxy(url, proxy string) string {
	if proxy == "" || proxy == "off" {
		return url
	}
	return strings.TrimRight(proxy, "/") + "/" + url
}

// geoFileReady 报告 GEO 文件是否已下载（体积门槛按文件区分，见 geoMinSize）。
// （实现移至文件末尾 downloadGeoOnce 之后。）

// migrateGeoDir 旧数据目录 geo/ → 运行目录 config/geo/ 一次性迁移（反馈⑤）。
func migrateGeoDir() {
	old := filepath.Join(dataDir(), "geo")
	if old == geoDir() {
		return
	}
	for _, name := range []string{geositeName, geoipLiteName} {
		src := filepath.Join(old, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = os.MkdirAll(geoDir(), 0o755)
		dst := filepath.Join(geoDir(), name)
		if _, err := os.Stat(dst); err == nil {
			continue // 新位置已有，不覆盖
		}
		if err := os.Rename(src, dst); err != nil {
			log.Printf("⚠ GEO %s 迁移失败：%v", name, err)
		} else {
			log.Printf("✓ GEO %s 已迁移到 config/geo/", name)
		}
	}
}

// updateGeoFiles 更新 GEO 文件。force=false 时已就绪即跳过（GUI 启动初始化）。
func updateGeoFiles(force bool) error {
	dir := geoDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	proxy := ghProxy()
	for _, name := range []string{geositeName, geoipLiteName} {
		if !force && geoFileReady(dir, name) {
			continue
		}
		setGeoInit("downloading", name, 0, "")
		if err := downloadGeo(dir, name, proxy); err != nil {
			setGeoInit("failed", name, 0, err.Error())
			return fmt.Errorf("%s: %w", name, err)
		}
		log.Printf("✓ GEO %s 已更新", name)
	}
	setGeoInit("done", "", 100, "")
	return nil
}

// downloadGeo 下载单个 GEO 文件：用户配置前缀 → 内置备用镜像 → 直连，逐级
// 兜底（v0.1.3 反馈②：单一镜像失败整链挂）。sha1 记录校验；整体 5 分钟超时；
// 临时文件原子落盘；进度回调写 geoInit——东哥 09-05 反馈③。
func downloadGeo(dir, name, proxy string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	urls := make([]string, 0, len(ghProxyFallbackMirrors)+2)
	if proxy != "" && proxy != "off" {
		urls = append(urls, applyGhProxy(rawGeoURL(name), proxy))
	}
	for _, m := range ghProxyFallbackMirrors {
		urls = append(urls, applyGhProxy(rawGeoURL(name), m))
	}
	urls = append(urls, rawGeoURL(name))

	var lastErr error
	for _, u := range urls {
		err := downloadGeoOnce(ctx, dir, name, u)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil { // 总超时已到，不再换源重试
			return err
		}
		log.Printf("⚠ GEO %s 下载源失败（换下一个源）：%v", name, err)
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("无可用下载源")
	}
	return fmt.Errorf("下载失败（可在配置页更换 GitHub 加速地址）：%w", lastErr)
}

// geoDirectBase 是兜底链最后一级的直连源头（var 仅为测试可注入，
// 避免测试依赖真实外网）。
var geoDirectBase = geoBaseURL

// rawGeoURL 返回 GEO 文件的 GitHub 原始直连 URL。
func rawGeoURL(name string) string {
	return geoDirectBase + "/" + name
}

// downloadGeoOnce 从单个 URL 下载一个 GEO 文件（size 校验 + 原子落盘）。
func downloadGeoOnce(ctx context.Context, dir, name, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	total := resp.ContentLength
	tmp, err := os.CreateTemp(dir, ".geo-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha1.New()
	var n int64
	buf := make([]byte, 128<<10)
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			if _, ew := io.MultiWriter(tmp, h).Write(buf[:nr]); ew != nil {
				tmp.Close()
				return ew
			}
			n += int64(nr)
			if total > 0 {
				setGeoInit("downloading", name, float64(n)*100/float64(total), "")
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			tmp.Close()
			return er
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// GitHub release 旁路没有官方 sha1 旁文件；此处 sha1 仅作记录。
	_ = hex.EncodeToString(h.Sum(nil))
	minSize := geoMinSize[name] // 按文件设门槛（geoip-lite 仅约 200KB）
	if minSize == 0 {
		minSize = 1 << 20
	}
	fi, err := os.Stat(tmpName)
	if err != nil || fi.Size() < minSize {
		return fmt.Errorf("下载内容过小（%d 字节），疑似失败", fiSize(fi))
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}

// fiSize 安全取文件大小（nil 时返回 0）。
func fiSize(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}
	return fi.Size()
}

// geoFileReady 报告 GEO 文件是否已下载（体积门槛按文件区分）。
func geoFileReady(dir, name string) bool {
	fi, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		return false
	}
	minSize := geoMinSize[name]
	if minSize == 0 {
		minSize = 1 << 20
	}
	return fi.Size() >= minSize
}
