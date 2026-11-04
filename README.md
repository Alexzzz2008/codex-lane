# Codex Lane

为 OpenAI Codex Desktop 和内核提供独立代理通道的 Windows 单文件应用。

连接已有 HTTP、HTTPS 或 SOCKS5 代理，不修改系统代理，不需要 TUN，也不改变其他应用的网络配置。

[下载最新版](https://github.com/Alexzzz2008/codex-lane/releases/latest) · [版本记录](https://github.com/Alexzzz2008/codex-lane/releases)

![运行总览](docs/screenshots/overview.png)

## 快速开始

1. 启动本机代理软件，确认监听地址和端口。
2. 下载最新 ZIP，解压后运行 `CodexLane.exe`。缺少 WebView2 时，启动提示可引导安装。
3. 在「设置」填写上游代理，点击「保存并应用」，再点击「测试」。
4. 在「总览」打开 Codex，或接入已经运行的 Codex；随后在「请求日志」查看连接链路。

Lane 与原生 Codex 可以按任意顺序打开。开启「发现原生 Codex 时提示接入」后，
发现尚未接入的 Codex 会询问「现在重启」或「稍后重启」。已有 Codex 需要重启才能
应用代理；确认并通过代理预检后才会重启，预检失败会保留当前 Codex。选择稍后后，仍可从总览接入。

Clash 使用规则模式时，还需让 `openai.com`、`chatgpt.com`、`oaistatic.com`、
`oaiusercontent.com` 命中可用的代理策略。可在「设置 → Clash 规则检查」复制建议片段，
并替换成自己的策略组名；Lane 不会自动修改 Clash 规则。

## 功能

- **独立双入口**：分别连接 Codex 界面与内核，一键开启、停止通道。
- **代理档案**：保存多组地址、测速并切换；新连接使用新代理，已有连接保持原链路。
- **掉线恢复**：上游暂时离线时保留本地入口，代理恢复后新连接可继续使用；支持备用档案自动切换。
- **分流控制**：按域名选择代理或直连，白名单限制可访问的目标域名。
- **实时观测**：查看连接状态、代理或直连链路、延迟与流量，支持筛选、暂停和报告导出。
- **连通检查**：检查本地入口、上游代理、OpenAI 域名及运行环境，不重启 Codex，不消耗模型额度。
- **桌面功能**：托盘、开机自启、Windows/WSL 内核识别与切换，以及应用内更新。

接入发现使用窗口事件，无定时进程扫描。窗口隐藏时暂停界面刷新，后台继续记录连接。

## 设置与运行环境

| 设置 | 保存与生效 |
|---|---|
| 上游地址、端口 | 点击「保存并应用」，新连接生效 |
| Lane 本地入口 | 自动选择并记住端口，后续优先复用 |
| 开关与下拉 | 自动保存；具体生效时机见对应设置说明 |
| Windows / WSL 内核模式 | 下次打开 Codex 时生效，修改前备份 Codex 配置 |
| 关闭窗口到托盘 | 下次启动 Lane 时生效 |
| 日志保留、内存容量 | 保存后应用；默认保留 14 天、缓存 10,000 条 |

自动模式读取 Codex 的内核配置；配置尚不存在时按 Windows 处理。WSL 模式目前要求
mirrored 网络，通过子进程环境传递代理。Lane 的发行版选项用于预检，实际发行版在 Codex 内设置。

## 文件位置与更新

首次从桌面、下载或临时目录（含其子目录）运行时，程序会迁移到固定目录，并创建桌面快捷方式。
迁移成功、窗口显示后才清理原 exe；失败时保留原程序。

```text
程序：%LOCALAPPDATA%\Programs\CodexLane\CodexLane.exe
配置：%LOCALAPPDATA%\CodexLane\launcher.json
日志：%LOCALAPPDATA%\CodexLane\logs\
```

配置独立于程序保存，升级时继续沿用。

应用自动检查并后台下载更新，安装前校验 SHA-256；可在「设置 → 关于」点击「重启并更新」，
已下载更新也会在正常退出时安装。离线时保留已验证缓存，新版本启动失败时尝试恢复旧版。
更新保留桌面快捷方式；重启更新会短暂中断代理连接，不结束 Codex。

手动替换程序前，请先从托盘菜单退出 Lane，避免旧实例仍占用窗口。

## 工作方式与边界

```text
Codex Desktop ── 界面入口 ──┐
                             ├── Codex Lane ── 已有代理 ── OpenAI
Codex 内核 ───── 内核入口 ──┘
```

界面通过启动参数、内核通过代理环境变量连接 Lane。HTTPS 只记录域名、端口、结果、耗时和流量，
不解密请求内容，不做 MITM。

Lane 不提供代理节点或订阅，不管理 Codex 账号；暂不支持需要用户名密码的上游代理认证。

## 命令行

```text
CodexLane.exe                    打开桌面控制台
CodexLane.exe --selftest         无界面自检
CodexLane.exe --selftest --json  输出 JSON 自检结果
CodexLane.exe --version          输出版本号
CodexLane.exe --check-webview2   检查 WebView2 Runtime
```

## 构建

需要 Go 1.26+、Node.js 22.12+ 和 Wails v3 CLI。

```powershell
pwsh -File scripts/build.ps1
```

构建、发布与验收说明见 [docs/RELEASE.md](docs/RELEASE.md)。

## 界面

| 请求日志 | 检查 |
|---|---|
| ![请求日志](docs/screenshots/requests.png) | ![检查](docs/screenshots/checks.png) |

![设置](docs/screenshots/settings.png)
