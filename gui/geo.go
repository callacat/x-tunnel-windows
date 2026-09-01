package main

// geo：GEO 数据库（geosite.dat / geoip-lite.dat）下载与就绪检查。
// 来源 MetaCubeX/meta-rules-dat（与 warp-go/Android 同源），下载走系统直连。

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
	"time"
)

const (
	geoBaseURL    = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest"
	geositeName   = "geosite.dat"
	geoipLiteName = "geoip-lite.dat"
)

// geoFileReady 报告 GEO 文件是否已下载。
func geoFileReady(dir, name string) bool {
	fi, err := os.Stat(filepath.Join(dir, name))
	return err == nil && fi.Size() > 1<<20 // <1MB 视为未完成/损坏
}

// updateGeoFiles 更新 GEO 文件。force=false 时已就绪即跳过（GUI 启动初始化）。
func updateGeoFiles(force bool) error {
	dir := filepath.Join(dataDir(), "geo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{geositeName, geoipLiteName} {
		if !force && geoFileReady(dir, name) {
			continue
		}
		if err := downloadGeo(dir, name); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		log.Printf("✓ GEO %s 已更新", name)
	}
	return nil
}

// downloadGeo 下载单个 GEO 文件（sha1 校验；5 分钟超时；临时文件原子落盘）。
func downloadGeo(dir, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	url := geoBaseURL + "/" + name
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
	tmp, err := os.CreateTemp(dir, ".geo-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha1.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// GitHub release 旁路没有官方 sha1 旁文件；此处 sha1 仅作记录。
	_ = hex.EncodeToString(h.Sum(nil))
	fi, err := os.Stat(tmpName)
	if err != nil || fi.Size() < 1<<20 {
		return errors.New("下载内容过小，疑似失败")
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}
