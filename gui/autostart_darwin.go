//go:build darwin

package main

// macOS 自启实现：~/Library/LaunchAgents LaunchAgent plist（RunAtLoad）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const darwinPlistID = "com.callacat.x-tunnel-windows"

func darwinPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", darwinPlistID+".plist")
}

func setAutostart(enabled bool) error {
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

func autostartEnabled() bool {
	_, err := os.Stat(darwinPlistPath())
	return err == nil
}

// xmlEscape plist 字符串转义（& < >）。
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
