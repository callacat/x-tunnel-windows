//go:build !windows

package main

// sysproxy 非 Windows 平台的编译兜底（GUI 实际只在 Windows 发布；
// 本机 Linux 只做代码级验证编译）。复用仓库根 sysproxy 包的 linux 实现亦可，
// 这里保持接口最小。

import "errors"

func setSysProxy(host, port string, enabled bool) error {
	return errors.New("系统代理仅支持 Windows")
}
