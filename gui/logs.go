package main

// 日志环形缓冲：GUI 日志页的数据源。service.go 的 logWriter 把 log.Printf
// 的输出同时写入这里；GetLogs 供前端轮询最近 N 条。

import (
	"log"
	"strings"
	"sync"
	"time"
)

// init 在包加载时就把标准库 log 路由到环形缓冲：早期启动日志（main 之前
// 可能已有 log 输出）也不丢，GUI 日志页能看到完整启动过程。
func init() {
	log.SetOutput(logWriter{})
	log.SetFlags(0)
}

// LogEntry 是日志页展示的单条记录。
type LogEntry struct {
	Time  string `json:"time"`
	Level string `json:"level"` // info | warn | error | debug
	Msg   string `json:"msg"`
}

// ringLogger 是有界环形缓冲，容量固定，新日志覆盖最旧。
type ringLogger struct {
	mu   sync.Mutex
	buf  []LogEntry
	next int
	full bool
}

const ringCap = 500

var ringLog = &ringLogger{buf: make([]LogEntry, ringCap)}

// Append 追加一条日志；自动按前缀推断级别。
func (r *ringLogger) Append(line string) {
	level := "info"
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "error"), strings.Contains(l, "失败"), strings.Contains(l, "无法"):
		level = "error"
	case strings.Contains(l, "warn"), strings.Contains(l, "⚠"), strings.Contains(l, "警告"):
		level = "warn"
	case strings.Contains(l, "debug"):
		level = "debug"
	}

	entry := LogEntry{
		Time:  time.Now().Format("15:04:05"),
		Level: level,
		Msg:   line,
	}

	r.mu.Lock()
	r.buf[r.next] = entry
	r.next = (r.next + 1) % ringCap
	if r.next == 0 {
		r.full = true
	}
	r.mu.Unlock()
}

// Clear 清空环形缓冲（日志页"清空"按钮调用）。
func (r *ringLogger) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = make([]LogEntry, ringCap)
	r.next = 0
	r.full = false
}

// Snapshot 返回最近 n 条（按时间正序）。
func (r *ringLogger) Snapshot(n int) []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	size := r.next
	if r.full {
		size = ringCap
	}
	if n > size {
		n = size
	}
	out := make([]LogEntry, n)
	for i := 0; i < n; i++ {
		idx := (r.next - n + i) % ringCap
		if idx < 0 {
			idx += ringCap
		}
		out[i] = r.buf[idx]
	}
	return out
}

// getLogs 返回最近 limit 条日志（limit<=0 返回全部）。
// 必须按实际写入条数取（环形数组预分配 ringCap，len(buf) 恒等于容量，
// 未写满时尾部空槽位会被当空日志返回——前端把空 level 归一化成 info 刷屏）。
func getLogs(limit int) []LogEntry {
	if limit <= 0 || limit > ringCap {
		limit = ringCap
	}
	return ringLog.Snapshot(limit)
}

// clearLogs 清空日志环（重置写入游标，保留预分配容量）。
func clearLogs() {
	ringLog.Clear()
}
