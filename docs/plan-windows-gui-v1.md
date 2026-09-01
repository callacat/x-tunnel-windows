# x-tunnel Windows GUI 客户端 v1.0 方案（方案A：复用 warp-go Wails 架构）

> 任务：东哥 2026-08-31 拍板方案A。先评审后动手。
> 作者：码农。状态：**待评审**（东哥/老马过目）。

## 1. 目标

产出 **x-tunnel Windows GUI 客户端**（x64 安装/绿色单 exe）：图形界面管理 x-tunnel 客户端连接（生产端点 wss://xt.dsdog.eu.cc 等自填），一键启停、系统代理托管、多配置管理、GEO 分流开关、日志收集导出。

## 2. 关键结论（决定架构的事实，均已核实）

| # | 事实 | 证据 |
|---|---|---|
| 1 | x-tunnel 客户端=Go 用户态单二进制：SOCKS5 监听(127.0.0.1:11080) + v2 协议隧道 + GEO 分流引擎 + control HTTP(状态/统计/热更新) | `internal/app/config.go` flag 面（-config/-l/-ip/-ech/-ips/-fallback/-sni/-route-enabled/-geo-dir/-rules-path/-control/-ready-file 等）+ CT107 实测日志（socks5:11080 / control:37757 / route stats） |
| 2 | **Windows 上无需 TUN/驱动**：客户端数据面=本地 SOCKS5，GUI 用「系统代理」接管全应用流量（WinINET 注册表，warp-go 已有实现） | `warp-go/sysproxy/windows.go`（registry CURRENT_USER Internet Settings，含 per-user 生效+刷新） |
| 3 | warp-go GUI 为 Wails v3 桌面壳（主窗口 960x680 + 托盘常驻 + Service 绑定），Windows 构建矩阵现成 | `warp-go/gui/main.go`、`gui/Taskfile.yml`（windows/darwin/android 三平台）、`versioninfo.json` |
| 4 | warp-go Service 模式：Start/Stop/GetStatus/配置读写/GEO 更新/系统代理开关/日志诊断，前端 api.ts 消费 | `warp-go/gui/service.go`（28+ 方法）、`gui/frontend` |
| 5 | 上游 Windows 命令行版已存在（v0.4.1 exe），证明 Windows 兼容性无底层障碍 | `6Kmfi6HP/x-tunnel` releases |
| 6 | 多配置/锁定编辑/GEO 状态可视化/诊断导出 的 Compose 交互范式已在 Android 端实现，可直接移植到 Web 前端 | `x-tunnel-android MainActivity.kt`（ProfileStore/RouteCard/RuntimeCard/DialogicExporter） |

## 3. 架构方案

```
┌──────────────────────────────────────────────┐
│ x-tunnel-gui.exe (Wails v3, x64)             │
│  ┌────────────┐  ┌─────────────────────────┐ │
│  │ 前端(React │  │ Service (Go 绑定层)      │ │
│  │ +Tailwind) │←→│  配置管理(ProfileStore)  │ │
│  └────────────┘  │  生命周期 Start/Stop     │ │
│                  │  状态轮询(control HTTP)  │ │
│  系统托盘:状态/   │  GEO 开关/规则/更新      │ │
│  启停/退出       │  日志环 + 诊断包导出      │ │
│                  └───────────┬─────────────┘ │
│  sysproxy (WinINET 注册表)    │ 子进程管理     │
└──────────────────────────────┼───────────────┘
                               ▼
              x-tunnel.exe (sidecar 子进程, 同目录)
              -config <gui生成> -control :0 -ready-file
              socks5 127.0.0.1:<port> ← sysproxy 指向这里
```

**核心决策① sidecar 以子进程方式打包（非 import 成库）**：
- x-tunnel 的 route 引擎/flag 解析在 `internal/app/config.go` 与 GUI 期望强耦合，import 改造会动 feat/route-engine 主线，违反「冲突即停」；
- 子进程方式=sidecar 零改动（用现成 flag 面），GUI 只管「生成 config 文件 + 拉起进程 + 轮询 control + 停止」；
- 版本升级：替换 x-tunnel.exe 即可，GUI 与 sidecar 解耦。
- 依赖：CI 构建时从 `callacat/x-tunnel` feat/route-engine 构建 windows exe 并嵌入产物（两二进制打包 zip / NSIS 安装包）。

**核心决策② 数据面=系统代理（非 TUN）**：
- x-tunnel 客户端本来只提供 SOCKS5，无 TUN 数据面；Windows 上套 TUN 需引 wintun 驱动+自行实现 tun2socks，复杂度爆炸且超出本轮目标；
- 系统代理覆盖绝大多数 GUI 应用（浏览器等走 WinINET）；命令行/DNS 不走（与 warp-go Windows 版行为一致，已知可接受）；
- GUI 提供「系统代理」开关（默认开），停止时自动还原。

## 4. 功能清单（v1）

| 模块 | 内容 | 复用来源 |
|---|---|---|
| 主页状态卡 | 运行状态/版本号/当前配置/核心 PID/流量统计 | Android StatusCard+RuntimeCard |
| 连接管理 | 连接/断开按钮（busy 态锁定）、启动后配置锁定 | Android ActionRow |
| 多配置 | 配置列表页 CRUD + 激活切换；server/token/本地端口/CIDR/DNS/ECH/UDP阻断/dialIPs/ipStrategy/dnsCacheTtl | Android ProfileStore 全字段 |
| GEO 分流 | 开关+规则编辑+GEO 库更新+运行状态(规则数/命中计数) | Android RouteCard + warp-go GetGeo/UpdateGeo |
| 系统代理 | 开关（默认跟随连接）、地址端口自动指向 sidecar | warp-go sysproxy |
| 日志页 | 实时滚动 + 过滤 + 导出 txt | Android LogStore + warp-go logs.go |
| 诊断包 | diagnostics.json(stats/status)+logs.zip 一键导出 | Android DialogicExporter |
| 托盘 | 状态菜单/启停/打开主窗口/退出（关窗最小化） | warp-go gui/main.go |
| 检查更新 | 跳 GitHub Releases latest | Android 同款 |

**不做（v1 明确排除）**：TUN 模式、分应用代理（Windows 系统代理无进程级分流；后续 v2 可用 Proxifier 式 LSP 或强制路由方案再议）、自动更新（只检查）。

## 5. 仓库与版本

- **新仓库 `callacat/x-tunnel-windows`**（或放 x-tunnel-android 同组织独立仓）：GUI + 构建脚本 + CI；
- 版本号单源：git tag `v0.2.0` 起（x-tunnel 客户端协议线对齐 v0.2 命名空间），`XTUNNEL_WIN_VERSION_NAME` CI 注入（对齐东哥 8-29 递增纪律：debug 轮自动 +1，正式版由 tag 决定，versionName/文件名/关于页三处一致）；
- CHANGELOG.md + Conventional Commits + CI 门禁（gofmt/vet/test/-race）全按项目标准。

## 6. CI/构建（GitHub Actions，本机不做重构建）

| Job | 内容 |
|---|---|
| lint-test | gofmt/vet/go test -race（GUI service 层） |
| build-windows | Taskfile windows 构建 GUI exe（nsis-mode 可选）；同时从 callacat/x-tunnel@feat/route-engine 构建 x-tunnel.exe；产出 zip（两二进制+README）+ 可选安装包 |
| release | tag 触发：上传 Release（exe/zip/SHA256SUMS），签名 secrets 沿用 x-tunnel-android 体系另配 |

## 7. 里程碑（评审通过后）

| 阶段 | 内容 | 验收 |
|---|---|---|
| M1 | 仓库初始化 + warp-go GUI 骨架移植（Service/托盘/前端壳） + sidecar 子进程拉起/停止/崩溃回收 | CI 绿；本机 win 虚拟化验证或 CI 产物 |
| M2 | 配置管理+连接生命周期+状态轮询+系统代理 | 配置→连接→浏览器走代理访问成功（CI windows runner 内 curl 验证数据面） |
| M3 | GEO 分流+日志页+诊断包+托盘完整 | 全功能走查；出第一个调试版 |
| M4 | 正式版 v0.2.0 发布（CHANGELOG/Release/东哥真机验收） | 东哥验收通过 |

## 8. 风险与对策

| 风险 | 对策 |
|---|---|
| Wails v3 仍 alpha，Windows webview2 依赖 | warp-go 已在 CT110 Windows 目标机验证过同栈（v0.6.0 发布），风险可控；安装包内置 WebView2Loader |
| sidecar 子进程僵死 | GUI 侧 ready-file 超时判定 + 进程退出监控 + 强杀回收（沿用 Android XTunnelRuntimeManager 的 pid 管理语义） |
| 系统代理对非 WinINET 应用无效 | 明示在 README/关于页（v1 边界），重要场景引导配置浏览器插件 |
| x-tunnel feat/route-engine 分叉未合 main | sidecar 按分支构建锁定，与 Android 同策略；主线合并后切换 |
| 本机无 Windows 构建环境 | 全部走 Actions windows runner（D7 纪律），本机只做代码级验证 |

## 9. 待东哥/老马确认项

1. 仓库名：`x-tunnel-windows` or `x-tunnel-win` or 放进现有仓？（默认建议 `x-tunnel-windows`）
2. 分发形态 v1 先出 **绿色 zip**（解压即用）够不够，安装包(NSIS) 放 M4 可选？（默认建议 zip 先行）
3. 版本号起点 v0.2.0 是否 OK（跟随 x-tunnel 协议 v2 线命名）？

---

## 10. 评审意见（老马 2026-09-02，已转东哥拍板 §9）

**结论：有条件通过**——架构决策合理（sidecar 子进程+系统代理+Wails v3），复用面大、里程碑可验收，风险表对仗。发现 1 个关键技术点需验证 + 2 个建议补充，不阻塞 M1 开工。

**⚠️ P0 技术验证点（M1 必做 spike，M2 验收前置）：**
- 数据面协议匹配：x-tunnel sidecar 只监听 SOCKS5（127.0.0.1:11080），而 WinINET 系统代理注册表（warp-go sysproxy 复用的 Internet Settings）默认写 HTTP 代理格式。需在 M1 确认：①x-tunnel 是否另有 HTTP 代理监听；②若只有 SOCKS5，sysproxy 需写 `socks=127.0.0.1:port` 格式注册表值（Windows 支持），并实测浏览器（Chrome/Edge 走 WinINET）能连通。此点不过则 M2「浏览器走代理访问成功」验收必挂。
- 佐证方式：M1 用 windows runner 或本地 win 虚机跑一个最小 spike（sidecar 拉起 + 注册表写 socks= + curl -x socks5h 与浏览器级验证），结论写进 task-context.md。

**P1 建议补充：**
- 多配置的 token 存储安全：Android ProfileStore 是明文还是加密？Windows 版建议至少说明存储方式（v1 可明文但文档要明示），后续可接 DPAPI。
- 仓库权限：callacat 组织新仓需要东哥 GitHub 组织权限确认（与 x-tunnel-android 同路径先例，见 recvtutH task-context）。

**P2 小提醒：**
- Wails v3 alpha 版本需锁定具体版本号（alpha 迭代快），与 warp-go 已验版本对齐。
- 版本线说明：Windows GUI v0.2.0 与 Android 0.1.0-roundN 并存，README/关于页注明「Windows 独立版本线，协议线对齐 v0.2」，避免东哥混淆。

**§9 待拍板项（老马推荐）：**
1. 仓库名：`x-tunnel-windows`（与 x-tunnel-android 平行，独立仓）✓
2. 分发形态：绿色 zip 先行，NSIS 放 M4 可选 ✓
3. 版本号起点：v0.2.0 ✓（注意与 Android 版本线区分说明）

**流程：** 方案通过后需 create 多维表格任务登记（当前无记录，K6 派单必登记），执行人=码农。
