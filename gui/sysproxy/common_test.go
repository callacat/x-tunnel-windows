package sysproxy

import (
	"runtime"
	"testing"
)

// TestSetDisableEmptyAddr 回归（东哥 v0.1.4 最高优反馈）：停止内核后关闭
// 系统代理走 Set("", false)，此前 splitAddr 在禁用路径也校验地址，空串报
// 「missing port in address」→ ProxyEnable 永远清不掉，开关卡死。
// 清除只动开关不动地址，空串必须放行。
//
// 平台注意：windows 实现只动 HKCU 注册表（CI 可跑）；darwin/linux 需要
// networksetup/gsettings，本测试仅验证「不再报地址非法」这一契约——非
// windows 平台 set() 返回的任何错误都不允许是 splitAddr 的地址校验错误。
func TestSetDisableEmptyAddr(t *testing.T) {
	err := Set("", false)
	if err == nil {
		return // 平台真实执行成功（windows CI / 有桌面环境）
	}
	// 失败信息里不允许出现地址校验错误（那正是卡死根因）。
	for _, bad := range []string{"missing port", "非法", "缺少主机名", "缺少端口"} {
		if contains(err.Error(), bad) {
			t.Fatalf("禁用路径不应做地址校验，却报：%v", err)
		}
	}
	// 其余错误（如 gsettings 未装）属环境问题，允许存在。
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// TestEnabledDualEmptyAddr 回归：无激活配置时 GetSystemProxyEnabled 传空地址，
// 此前直接报错（前端开关状态失真）。空地址必须返回 false 不报错。
func TestEnabledDualEmptyAddr(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows 注册表语义，其他平台跳过")
	}
	on, err := EnabledDual("", "")
	if err != nil {
		t.Fatalf("空地址不应报错：%v", err)
	}
	if on {
		t.Fatal("空地址不应报告启用")
	}
}

// TestSetDualEnableStillValidates 回归：启用路径必须继续校验地址——
// 放开禁用路径不能把启用路径的校验也松掉。
func TestSetDualEnableStillValidates(t *testing.T) {
	err := SetDual("", "", true)
	if err == nil {
		t.Fatal("启用路径空地址应报错")
	}
	err = SetDual("127.0.0.1", "127.0.0.1:11080", true) // http 缺端口
	if err == nil {
		t.Fatal("启用路径缺端口应报错")
	}
}
