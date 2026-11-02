# Codex Lane

为 OpenAI Codex Desktop 提供独立进程级代理通道的 Windows 轻量应用。

Codex Lane 只接管 Codex 的网络出口，不修改 Windows 系统代理、不启用 TUN，也不影响其他应用。启动前需要一个可用的 HTTP、HTTPS 或 SOCKS5 上游代理，例如 Clash 的 mixed-port。

![运行总览](docs/screenshots/overview.png)

## 工作方式

```text
Codex Desktop ── UI 入口 ──┐
                           ├── Codex Lane ── 已有代理 ── OpenAI
Codex 内核 ─── 内核入口 ───┘
```

- 界面层通过 `--proxy-server` 指向 Lane，不依赖系统代理。
- 内核层通过 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、`NO_PROXY` 指向 Lane。
- Lane 将请求转发到用户配置的上游代理，并记录连接元数据。
- 支持 Windows 与 WSL2 内核；默认自动识别，也可以手动指定。

HTTPS 只记录域名、端口、结果、耗时和流量，不记录路径、Header、Cookie 或请求体，也不做 MITM。

## 功能

- **一键开关**：启动、停止和重新接入 Codex，不修改系统网络设置。
- **双入口隔离**：界面与内核分别使用独立的回环端口，端口自动复用。
- **多上游档案**：保存多套代理地址，支持测速、切换和故障自动切换。
- **分流与白名单**：按域名设置直连或代理；白名单可阻止未授权程序借用 Lane。
- **请求日志**：查看来源、目标、代理路径、状态、延迟和上下行流量，支持报告与 HAR 导出。
- **连通检查**：检查代理、Codex、WebView2、WSL2 和 OpenAI 域名连通性。
- **桌面体验**：单文件 exe、托盘运行、开机自启和内置更新提示。

## 快速开始

1. 从 [Releases](https://github.com/Alexzzz2008/codex-lane/releases) 下载最新 Windows 便携版。
2. 解压并运行 `CodexLane.exe`，确认上游代理地址和端口。
3. 在「总览」点击「打开 Codex」，或对已运行的 Codex 点击「重新打开 Codex」。
4. 在「请求日志」确认请求来源、目标域名和代理链路。

关闭 Codex Lane 后，Codex 会失去 Lane 入口；重新启动 Codex 即可恢复原生网络。

## 命令行

```text
CodexLane.exe                    打开桌面控制台
CodexLane.exe --selftest         无界面自检
CodexLane.exe --selftest --json  输出 JSON 自检结果
CodexLane.exe --version          输出版本号
CodexLane.exe --check-webview2   检查 WebView2 Runtime
```

## 配置与日志

配置文件不会写到 exe 同目录：

```text
%LOCALAPPDATA%\CodexLane\launcher.json
%LOCALAPPDATA%\CodexLane\logs\requests-YYYYMMDD.jsonl
```

旧版本放在 exe 同目录的 `launcher.json` 会自动迁移。日志默认保留 14 天，可在设置页修改。

## 检查与故障定位

- 「检查」页验证代理、Codex 安装、WSL2 和 OpenAI 域名，并给出汇总分数。
- 「Clash 规则检查」列出需要走代理的域名，可复制规则片段到现有配置。
- 「请求日志」中的“已走代理 / 直连 / 被拒绝”用于确认实际网络路径。
- 自检不会启动窗口，也不会启动或重启 Codex。

## 边界

不处理系统代理、TUN、代理节点与订阅，也不管理 Codex 账号登录。暂不支持需要用户名密码的代理认证。

## 构建

需要 Go 1.26+、Node.js 20+ 和 Wails v3 CLI。

```powershell
pwsh -File scripts/build.ps1
```

脚本会执行前端检查、Vite 生产构建、Wails 绑定与图标生成，最后输出单文件 exe、桌面 zip 和 SHA-256。详细说明见 [docs/RELEASE.md](docs/RELEASE.md)。

## 界面

| 请求日志 | 检查 |
|---|---|
| ![请求日志](docs/screenshots/requests.png) | ![检查](docs/screenshots/checks.png) |

| 设置 | 关于 |
|---|---|
| ![设置](docs/screenshots/settings.png) | ![关于](docs/screenshots/about.png) |
