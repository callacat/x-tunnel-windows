//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// execCommand Windows 隐藏窗口执行（rundll32 打开浏览器不弹黑框）。
func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
