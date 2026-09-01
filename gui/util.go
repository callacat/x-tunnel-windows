package main

// util：atomicWriteFile / logWriter / jsonMarshalIndent / openBrowser /
// runtimeGOOS 等小工具（自 warp-go service.go 移植裁剪）。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func runtimeGOOS() string { return runtime.GOOS }

func jsonMarshalIndent(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

// atomicWriteFile 先写临时文件再原子改名，避免半写文件被读取。
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".xt-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// logWriter 把 log.Printf 的输出同时送入环形缓冲（logs.go）。
type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	ringLog.Append(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// openBrowser 用系统默认浏览器打开 URL（Windows: rundll32 url.dll 协议处理）。
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return execCommand("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return execCommand("open", url)
	default:
		return execCommand("xdg-open", url)
	}
}
