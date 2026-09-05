package main

// update_check：GitHub Releases 最新版检查（CheckUpdate 后端）。
// v0.1.3 bug 修复（东哥 09-05 反馈②）：此前直连 api.github.com，国内网络
// 必挂，与配置页「检查更新经此前缀下载」的承诺不符。现走 gh-proxy 前缀
// （用户配置 → 内置备用镜像 → 直连）逐级兜底，与 GEO 下载同策略。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// checkGitHubUpdate 查询 repo 的 latest release 并与当前版本比较。
// 版本比较用简单的非前缀语义化对齐（vX.Y.Z）， prerelease/例外场景手动核对。
func checkGitHubUpdate(ctx context.Context, repo, current string) (*UpdateInfo, error) {
	base := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	urls := make([]string, 0, len(ghProxyFallbackMirrors)+2)
	if p := ghProxy(); p != "" && p != "off" {
		urls = append(urls, applyGhProxy(base, p))
	}
	for _, m := range ghProxyFallbackMirrors {
		urls = append(urls, applyGhProxy(base, m))
	}
	urls = append(urls, base)

	var lastErr error
	for _, u := range urls {
		info, err := fetchLatestRelease(ctx, u, current)
		if err == nil {
			return info, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		log.Printf("⚠ 检查更新下载源失败（换下一个源）：%v", err)
		lastErr = err
	}
	return nil, fmt.Errorf("检查更新失败（可在配置页更换 GitHub 加速地址）：%w", lastErr)
}

// fetchLatestRelease 从单个 URL 拉取 latest release 并归一化。
func fetchLatestRelease(ctx context.Context, url, current string) (*UpdateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	info := &UpdateInfo{Current: current, Latest: latest, URL: rel.HTMLURL}
	if current != "" && current != "dev" && latest != current && versionLess(current, latest) {
		info.HasUpdate = true
	}
	return info, nil
}

// versionLess 比较 a<b（仅 X.Y.Z 数字段）。
func versionLess(a, b string) bool {
	pa := strings.Split(strings.SplitN(a, "-", 2)[0], ".")
	pb := strings.Split(strings.SplitN(b, "-", 2)[0], ".")
	for i := 0; i < 3; i++ {
		var x, y int
		fmt.Sscanf(pa[i], "%d", &x)
		fmt.Sscanf(pb[i], "%d", &y)
		if x != y {
			return x < y
		}
	}
	return false
}

var _ = time.Second // 保留 time import（部分平台兜底）
