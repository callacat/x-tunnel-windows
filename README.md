# x-tunnel-windows

x-tunnel Windows GUI 客户端（Wails v3）。方案A：复用 warp-go 桌面壳架构，x-tunnel 客户端以子进程方式集成（sidecar 零改动），数据面为系统代理（WinINET）。

## 架构

```
x-tunnel-windows.exe (Wails v3 GUI)
  ├─ Service (Go 绑定层)：Profile 管理 / 生命周期 / 状态轮询 / GEO / 系统代理
  ├─ sidecar 子进程：x-tunnel.exe（-config -l -control -ready-file）
  └─ sysproxy：WinINET 注册表系统代理
```

方案文档：`docs/plan-windows-gui-v1.md`（评审通过版）。

## 开发

```bash
cd gui/frontend && npm install && npm run build   # 前端
cd gui && go build ./...                          # 后端（Linux 上需 GOOS=windows）
GOOS=windows GOARCH=amd64 go build -o ../bin/x-tunnel-windows.exe .
```

GUI 与 `x-tunnel.exe`（自 `callacat/x-tunnel` feat/route-engine 构建）同目录部署。

## CI

- `ci.yml`：Go fmt/vet/test -race（windows runner）+ 前端 vitest/build
- `release.yml`：tag 触发，GUI+sidecar 双二进制打包 zip → GitHub Release
