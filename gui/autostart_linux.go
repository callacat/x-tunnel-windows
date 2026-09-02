//go:build linux

package main

// Linux 自启实现：XDG autostart（~/.config/autostart，GNOME/KDE 通用）。

import (
	"fmt"
	"os"
	"path/filepath"
)

func linuxAutostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "x-tunnel-windows.desktop")
}

func setAutostart(enabled bool) error {
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

func autostartEnabled() bool {
	_, err := os.Stat(linuxAutostartPath())
	return err == nil
}
