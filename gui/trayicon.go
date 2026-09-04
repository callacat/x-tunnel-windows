package main

// trayicon：托盘图标加载（自建资源，与 warp-go 默认图标区分——东哥 09-04 反馈③）。
// 资源源文件 gui/build/appicon.png（512px，橙环+青弧+橙心设计，
// scripts/icongen 生成），embed 进二进制，Wails SetIcon 直接收 PNG 字节。

import (
	"embed"
	"log"
)

//go:embed build/appicon.png
var appIconFS embed.FS

// appIconPNG 返回内嵌图标 PNG 字节；失败返回 nil（调用方回退 Wails 默认图标）。
func appIconPNG() []byte {
	raw, err := appIconFS.ReadFile("build/appicon.png")
	if err != nil {
		log.Printf("托盘图标资源读取失败：%v", err)
		return nil
	}
	return raw
}
