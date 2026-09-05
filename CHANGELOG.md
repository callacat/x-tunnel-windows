# Changelog

本项目所有重要变更记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.1.5] - 2026-09-05

东哥 v0.1.4 真机反馈修复（系统代理卡死为最高优）。

### Fixed

- **系统代理无法关闭（最高优）**：停止内核后系统代理仍开着，点关闭报 `RuntimeError: 系统代理地址 "" 非法：missing port in address`，开关永久卡死。根因：`sysproxy.SetDual` 在**禁用路径也校验地址**，而 `Stop()`/关闭开关传空串地址（清除只动 `ProxyEnable`，地址用不上）→ 校验先报错，注册表永远清不掉。修复：禁用路径跳过地址校验（空串放行）；`EnabledDual` 空地址返回 false 不报错；`Stop()` 还原失败不再吞掉、写日志并提示。回归测试 3 项固化（禁用空串放行 / 启用仍校验 / 空地址查询不报错）。
- **启动时间不显示**：`GetStatus` 从未给 `StartedAt` 赋值（sidecarManager 有记录但没接通）——前端启动时间恒显示"—"。已接通，格式 `2006-01-02 15:04:05`。
- **日志页无内容**：日志页只显示 GUI 自身日志环（运行期几乎无输出），真正有内容的 sidecar 数据面日志（连接/分流/直连明细，仅诊断包可见）从未接入。已改为每秒轮询合并 GUI 环 + sidecar.log 尾部 200 条，按时间排序展示，sidecar 行按关键词分级（失败→error）。

## [0.1.4] - 2026-09-05

东哥 v0.1.3 真机反馈两点修复。

### Fixed

- **配置页「GitHub 加速」保存按钮永久转圈禁用**（反馈①）：前端解构遗漏 `busy`，按钮 loading 判定误用恒非空的函数引用（`runGh !== null`），导致保存流程实际完成后按钮仍停留 pending 态——改按动作 key 判定（`busy === "ghproxy"`），与其余页面既有模式对齐。
- **「立即更新」GEO 下载失败（geoip 被体积门槛误杀）**（反馈②）：上游 geoip-lite.dat 2026-09 起精简到约 200KB，v0.1.3 统一 1MB 最小体积门槛把下载成功的文件误判「内容过小疑似失败」——门槛改为按文件区分（geosite 1MB / geoip 100KB）。
- **检查更新不走加速前缀**（反馈②伴生）：`CheckUpdate` 此前直连 `api.github.com`（国内网络必挂），与配置页「检查更新经此前缀下载」的文案承诺不符——改走 gh-proxy 前缀，与 GEO 下载同策略。
- **GitHub 下载链无兜底**（反馈②伴生）：单一镜像失败即整链失败——下载改为「用户配置前缀 → 内置备用镜像（gh-proxy.com）→ 直连」逐级兜底；GEO 与检查更新共用。
- **`ghProxy()` 空值语义矛盾**：空配置原被当作直连，与 `SetGhProxy`「空=默认落盘」及前端文案矛盾——空值统一解析为默认 `https://gh-proxy.org`。

### Changed

- 回归测试：geo 下载兜底链/体积门槛/全源失败指引/前缀拼接/空值语义 6 项（`geo_test.go`）。

## [0.1.3] - 2026-09-05

东哥 v0.1.1 真机反馈五点修复。

### Changed

- **状态页「开机自启」「关于」迁至配置页**（反馈①）：状态页聚焦运行状态与开关，应用设置类入口统一收进配置页。
- **GEO 数据库移至运行目录 config/geo/**（反馈⑤）：与 sidecar 同目录布局，绿色便携、用户可见；旧数据目录 geo/ 下已下载文件自动迁移，无需重新下载。
- **配置页新增「GitHub 加速」设置**（反馈④）：GEO 下载与检查更新经加速前缀，默认 `https://gh-proxy.org`，填 `off` 直连；设置存 profiles.json 顶层 `gh_proxy`。

### Fixed

- **日志页 info 空行刷屏**（反馈②）：后端日志环按预分配容量返回未写入的空槽位（`len(buf)` 恒等于容量），前端把空条目归一化成 info——改为按实际写入条数返回（回归测试固化），前端再加空条目过滤兜底。
- **GEO 下载无进度/成败反馈**（反馈③）：初始化状态（downloading/done/failed + 文件名 + 百分比 + 失败原因）随状态轮询回报，状态页实时展示；失败提示可到 GEO 页重试或更换加速地址。

## [0.1.2] - 2026-09-04

东哥 v0.1.0 反馈①的收尾修复 + 三平台打包结构对齐。

### Fixed

- **主程序启动黑框**：v0.1.1 只隐藏了 sidecar 子进程窗口，GUI 主程序仍以 CONSOLE 子系统链接（双击仍弹 CMD 黑框）。Windows 构建补 `-H windowsgui`（对齐 Wails 官方生产构建），PE subsystem 实测翻转为 GUI。
- **Linux/macOS 包结构**：v0.1.1 GUI 三平台统一从 `config/` 查找 sidecar，但 linux/macos zip 仍把内核放根目录——装上即报"未找到内核"。两平台打包与 Windows 对齐：`config/x-tunnel` + `config/rules.txt`。

## [0.1.1] - 2026-09-04

东哥 v0.1.0 真机反馈修复。

### Fixed

- **演示模式根因**（3c3ee43）：v0.1.0 打包时 Wails bindings 未生成（`__MOCK_BINDINGS__` 占位在库），整个应用跑在演示数据上——前端所有功能调用都到不了后端。本轮 `wails3 generate bindings --ts` 生成真绑定（24 方法/7 模型）入库，release 流程前置绑定生成。
- **CMD 黑框**（3c3ee43）：sidecar 子进程补 `CREATE_NO_WINDOW`+`HideWindow`，启动/连接不再闪黑框。
- **sidecar 位置**（3c3ee43）：x-tunnel.exe 移至运行目录 `config/` 子目录，release zip 打包结构同步（GUI 同级 config/x-tunnel.exe + config/rules.txt）。

### Changed

- **自建图标**（3c3ee43）：应用/托盘/PE 资源三处换新设计（橙环+青弧+橙心，`scripts/icongen` 生成），不再与 warp-go 默认图标相同；info.json 品牌同步为 x-tunnel-windows。

## [0.1.0] - 2026-09-03

首个正式版：x-tunnel 桌面 GUI 客户端（Wails v3），Windows / macOS / Linux 三平台。

### Added

- **三平台支持**（673b8b9）：Windows（WinINET 注册表系统代理 + HKCU Run 自启）、macOS arm64（networksetup + LaunchAgent）、Linux amd64 GNOME（gsettings + XDG autostart），三平台 zip 均含 GUI + x-tunnel sidecar + README + 默认规则。
- **M1 闭环**（9d55a35、da97be1、f04b0bc）：Wails v3 壳移植 + sidecar 子进程管理 + Profile 模型 + 前端适配（配置管理页 / 状态页新字段 / 裁注册与扫描）+ 双监听系统代理。
- **诊断包导出**（581283c）：日志页一键导出 zip（diagnostics.json + logs.txt + sidecar.log）。
- **GEO / 流量运行态对接真实端点**（cd75ad6）：`/v1/route/stats` 与 `/v1/stats`，替换占位数据。

### Fixed

- **control API 鉴权补全**（bec562e）：修复 GUI 状态轮询全部 401。
- **autostart 三平台按文件分派**（ebeedad）：修复 darwin/linux 下编译失败。
- **release Linux 构建基线**（ca57b1c）：改用 ubuntu-24.04 + libwebkitgtk-6.0-dev（GTK4 同代 WebKitGTK 仅 24.04+ 提供）。
- **Windows PE 版本元数据随 tag 注入**：release 构建用 goversioninfo 生成 .syso，exe 文件属性（FileVersion / ProductVersion）与 zip 文件名、GUI 首页显示三处同源，均由 tag 派生。
