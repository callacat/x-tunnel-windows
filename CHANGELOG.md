# Changelog

本项目所有重要变更记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

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
