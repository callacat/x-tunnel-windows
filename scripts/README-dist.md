# x-tunnel Windows GUI

绿色版：解压即用，无需安装。

## 使用

1. 解压到任意目录（建议非系统盘，如 `D:\x-tunnel\`）
2. 运行 `x-tunnel-windows.exe`
3. 「配置」页新增配置：填服务器地址（`wss://…`）与 Token，保存并设为使用
4. 首页点「连接」；系统代理默认随连接开启（浏览器等自动走隧道）
5. 不用了点「关闭」——系统代理会自动还原

## 文件说明

| 文件 | 用途 |
|---|---|
| `x-tunnel-windows.exe` | GUI 主程序 |
| `config/x-tunnel.exe` | x-tunnel 客户端内核（GUI 自动拉起，勿单独运行） |
| `config/rules.txt` | 路由规则（GUI「规则」页可编辑，保存即生效） |
| `README.md` | 本说明 |

数据目录：`%APPDATA%\x-tunnel-windows`（配置 profiles.json、GEO 库、日志）。

## 已知边界（v1）

- 系统代理模式：命令行程序与部分不走系统代理的应用不经过隧道
- 不含 TUN 模式与分应用代理（后续版本评估）
