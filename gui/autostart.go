package main

// 开机自启：Windows 写注册表 Run 键（HKCU\...\Run），非 Windows 返回未支持。

import (
	"errors"
	"os"
	"path/filepath"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValueName = "x-tunnel-windows"

func setAutostart(enabled bool) error {
	if runtimeGOOS() != "windows" {
		return errors.New("开机自启仅支持 Windows")
	}
	return setAutostartWindows(enabled)
}

func autostartEnabled() bool {
	if runtimeGOOS() != "windows" {
		return false
	}
	return autostartEnabledWindows()
}

func autostartCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func autostartShortcutTarget() string { return filepath.Base(autostartCommand()) }
