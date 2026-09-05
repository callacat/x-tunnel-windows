package main

import (
	"log"
	"regexp"
	"strings"
	"testing"
)

// TestInitLoggingNoStdTimestampPrefix 验证 initLogging() 设置 log.SetFlags(0)
// 后，环形缓冲中的日志消息不再携带标准库 log 的日期/时间前缀
// （"2026/08/01 04:55:44 "），只保留 ringLog 自身按系统时间生成的
// HH:MM:SS 时间戳——避免前端日志页出现双时间戳。
func TestInitLoggingNoStdTimestampPrefix(t *testing.T) {
	// 不调用 newService()（它会启动 InitDefaults 协程做网络 GEO 下载），
	// 只测 initLogging() 的日志标志行为。
	ringLog = &ringLogger{buf: make([]LogEntry, ringCap)}
	initLogging()
	log.Printf("内核启动测试 %d", 42)

	entries := ringLog.Snapshot(1)
	if len(entries) != 1 {
		t.Fatalf("期望 1 条日志，得到 %d", len(entries))
	}
	e := entries[0]

	// 标准库 log 默认 LstdFlags 前缀形如 "2026/08/01 04:55:44 "。
	stdDatePrefix := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)
	if stdDatePrefix.MatchString(e.Msg) {
		t.Errorf("消息仍带标准库日期前缀：%q", e.Msg)
	}
	if !strings.Contains(e.Msg, "内核启动测试 42") {
		t.Errorf("消息内容缺失：%q", e.Msg)
	}
	// 环形缓冲自己的时间戳格式：HH:MM:SS。
	timeFormat := regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`)
	if !timeFormat.MatchString(e.Time) {
		t.Errorf("环形缓冲时间戳格式异常：%q", e.Time)
	}
}

// TestGetLogsNoEmptySlots 回归（东哥 09-05 反馈②）：环形数组预分配 ringCap，
// 未写满时 getLogs 必须只返回实际写入条数——把空槽位返回会让前端把空 level
// 归一化成 info，日志页被空 info 行刷屏。
func TestGetLogsNoEmptySlots(t *testing.T) {
	ringLog = &ringLogger{buf: make([]LogEntry, ringCap)}
	log.Printf("第一条")
	log.Printf("第二条")
	log.Printf("第三条")

	got := getLogs(0) // 0=全部
	if len(got) != 3 {
		t.Fatalf("期望 3 条，得到 %d（空槽位泄漏）", len(got))
	}
	for i, e := range got {
		if e.Msg == "" && e.Time == "" && e.Level == "" {
			t.Errorf("第 %d 条是空槽位", i)
		}
	}
	if got[0].Msg == "" || !strings.Contains(got[2].Msg, "第三条") {
		t.Errorf("日志顺序异常：%q..%q", got[0].Msg, got[2].Msg)
	}
}

// TestClearLogsThenAppend 回归：clearLogs 重置游标后继续 Append 不得越界
// 或 resurrect 旧数据。
func TestClearLogsThenAppend(t *testing.T) {
	ringLog = &ringLogger{buf: make([]LogEntry, ringCap)}
	log.Printf("旧日志")
	clearLogs()
	log.Printf("新日志")

	got := getLogs(0)
	if len(got) != 1 {
		t.Fatalf("清空后应只有 1 条，得到 %d", len(got))
	}
	if !strings.Contains(got[0].Msg, "新日志") {
		t.Errorf("内容异常：%q", got[0].Msg)
	}
}
