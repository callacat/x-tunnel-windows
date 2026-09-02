package main

// 开机自启（平台实现按文件分派，构建标签互斥）：
//   - autostart_windows.go：HKCU Run 键
//   - autostart_darwin.go：~/Library/LaunchAgents plist
//   - autostart_linux.go：XDG autostart .desktop
// 本文件只放三平台共享的辅助函数。

import (
	"os"
	"path/filepath"
)

func autostartCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func autostartShortcutTarget() string { return filepath.Base(autostartCommand()) }
