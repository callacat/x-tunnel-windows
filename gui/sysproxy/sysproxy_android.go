//go:build android

package sysproxy

// set 在 Android 上是无操作：Android 系统代理由 VPN 服务接管，没有
// gsettings / networksetup / 注册表可用。返回 nil（视为成功，无副作用），
// 使 sysproxy.Set 在 Android 上安全通过。
func set(httpHost, httpPort, socksHost, socksPort string, enabled bool) error {
	_ = socksHost // 非 Windows 平台单代理语义，socks 段忽略
	_, _ = socksPort, httpHost
	_ = httpPort
	host, port := httpHost, httpPort
	return nil
}

// enabled 在 Android 上恒 false：VPN 接管全部流量，无系统代理概念。
func enabled(host, port string) (bool, error) {
	return false, nil
}
