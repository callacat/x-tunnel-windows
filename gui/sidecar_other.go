//go:build !windows

package main

import "os/exec"

// 非 Windows 无控制台窗口概念，no-op。
func hideSidecarWindow(cmd *exec.Cmd) {}
