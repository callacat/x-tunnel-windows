# x-tunnel-windows

x-tunnel 桌面 GUI 客户端（Wails v3），支持 **Windows / macOS / Linux**。方案A：复用 warp-go 桌面壳架构，x-tunnel 客户端以子进程方式集成（sidecar 零改动），数据面为系统代理。

## 平台支持矩阵

| 平台 | 架构 | 系统代理机制 | 自启机制 | 产物 |
|---|---|---|---|---|
| Windows | amd64 | WinINET 注册表（`http=/https=/socks=` 三段） | HKCU Run 键 | `x-tunnel-windows.exe` + `x-tunnel.exe` |
| macOS | arm64 | networksetup（web/secure 代理） | LaunchAgent plist | `x-tunnel-windows` + `x-tunnel` |
| Linux | amd64 (GNOME) | gsettings（org.gnome.system.proxy） | XDG autostart .desktop | `x-tunnel-windows` + `x-tunnel` |

> Linux 系统代理依赖 GNOME（gsettings）；其他桌面（KDE 亲测可用 gsettings 桥接/Wayland 会话）未覆盖时，可用环境变量 `http_proxy/https_proxy` 指向 sidecar 的 HTTP 代理监听替代。macOS Linux 版的 GUI 构建/运行需 WebKitGTK（Linux）或系统 WebView（macOS WKWebView，系统自带）。

## 架构

```
x-tunnel-windows (Wails v3 GUI, 全平台)
  ├─ Service (Go 绑定层)：Profile 管理 / 生命周期 / 状态轮询 / GEO / 系统代理 / 诊断包
  ├─ sidecar 子进程：x-tunnel（-config -l -control -ready-file -control-token-file）
  └─ sysproxy：Windows WinINET / macOS networksetup / Linux gsettings
```

方案文档：`docs/plan-windows-gui-v1.md`（评审通过版）。

## sidecar 双监听（系统代理的关键）

sidecar 以 `-l "socks5://127.0.0.1:11080,http://127.0.0.1:11081"` 同进程双监听：

- **HTTP 代理端口（socks 端口 +1）**：系统代理 `http=`/`https=` 段指向这里——浏览器走 CONNECT 域名透传（服务端远程解析，无本地 DNS 污染问题）
- **SOCKS5 端口**：系统代理 `socks=` 段兜底 + 高级用户手动配置

## 开发

```bash
cd gui/frontend && npm install && npm run build   # 前端
cd gui && go build ./...                          # 后端
# Linux 本机编译需 GTK4/WebKitGTK 开发库：
#   apt install libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev
# Windows 交叉编译（无 CGO 依赖验证用）：
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o ../bin/x-tunnel-windows.exe .
# macOS/Linux 完整 GUI 构建需在对应平台 runner（CGO），见 release.yml 矩阵
```

GUI 与 sidecar（自 `callacat/x-tunnel` feat/route-engine 构建）**同目录部署**。

## CI

- `ci.yml`：Go fmt/vet/test -race（windows runner）+ 前端 vitest/build + linux GUI 编译门禁（gtk4 CGO）+ windows 数据面 spike（真实拉起 sidecar 全链路验证）
- `release.yml`：tag 触发，三平台矩阵构建 → 各平台 zip（GUI+sidecar+README+默认规则）+ 汇总 SHA256SUMS → GitHub Release
