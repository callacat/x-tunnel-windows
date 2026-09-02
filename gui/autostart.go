package main

// 开机自启（全平台）：
//   - Windows：HKCU Run 键（autostart_windows.go）
//   - macOS：~/Library/LaunchAgents/com.callacat.x-tunnel-windows.plist
//   - Linux：~/.config/autostart/x-tunnel-windows.desktop（XDG，GNOME/KDE 等）

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func setAutostart(enabled bool) error {
	switch runtime.GOOS {
	case "windows":
		return setAutostartWindows(enabled)
	case "darwin":
		return setAutostartDarwin(enabled)
	case "linux":
		return setAutostartLinux(enabled)
	default:
		return fmt.Errorf("开机自启暂不支持 %s", runtime.GOOS)
	}
}

func autostartEnabled() bool {
	switch runtime.GOOS {
	case "windows":
		return autostartEnabledWindows()
	case "darwin":
		return autostartEnabledDarwin()
	case "linux":
		return autostartEnabledLinux()
	default:
		return false
	}
}

func autostartCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func autostartShortcutTarget() string { return filepath.Base(autostartCommand()) }

// ---------------------------------------------------------------------------
// macOS：LaunchAgent plist
// ---------------------------------------------------------------------------

const darwinPlistID = "com.callacat.x-tunnel-windows"

func darwinPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", darwinPlistID+".plist")
}

func setAutostartDarwin(enabled bool) error {
	path := darwinPlistPath()
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe := autostartCommand()
	if exe == "" {
		return fmt.Errorf("无法确定可执行文件路径")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
`, darwinPlistID, xmlEscape(exe))
	return os.WriteFile(path, []byte(plist), 0o644)
}

func autostartEnabledDarwin() bool {
	_, err := os.Stat(darwinPlistPath())
	return err == nil
}

// ---------------------------------------------------------------------------
// Linux：XDG autostart .desktop
// ---------------------------------------------------------------------------

func linuxAutostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "x-tunnel-windows.desktop")
}

func setAutostartLinux(enabled bool) error {
	path := linuxAutostartPath()
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe := autostartCommand()
	if exe == "" {
		return fmt.Errorf("无法确定可执行文件路径")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=x-tunnel
Exec=%s
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
`, exe)
	return os.WriteFile(path, []byte(desktop), 0o644)
}

func autostartEnabledLinux() bool {
	_, err := os.Stat(linuxAutostartPath())
	return err == nil
}

// xmlEscape plist 字符串转义（& < >）。
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
