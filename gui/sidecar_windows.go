//go:build windows

package main

// sidecar 子进程窗口隐藏（东哥 09-04 反馈①：启动时 CMD 黑框藏起来）。
// CREATE_NO_WINDOW = 0x08000000：为子进程不新建控制台；GUI 自身无控制台，
// sidecar 也不应有——之前每次连接会闪一个黑框。

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func hideSidecarWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
