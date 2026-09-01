// Compile-time placeholder for the x-tunnel Wails v3 bindings.
//
// `wails3 generate bindings` 会把真实的 x-tunnel Service 绑定生成到本路径
// （Go module 路径 github.com/callacat/x-tunnel-windows/gui）。在生成前，
// 这里用一个带 __MOCK_BINDINGS__ 标记的占位对象占位：frontend/src/lib/api.ts
// 运行时检测到该标记即进入演示模式（demo data），避免调用不存在的 $Call。
// 本文件会被真实 bindings 覆盖，无需手工维护。

export const Service = {
  __MOCK_BINDINGS__: true as const,
};
