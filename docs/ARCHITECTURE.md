# Codex Lane 架构设计

## 1. 结论

采用 **Go 1.26 + Wails v3.0.0 + TypeScript/Svelte 5 + HTML/CSS**，交付一个
`CodexLane.exe` 桌面应用。Go 负责网关、进程控制、检测、配置和日志，前端只负责界面。
Windows 11 自带 WebView2，不依赖 .NET、Electron 或 Node 运行时。

产品定位是 **进程级代理启动器 + 本地观测网关**，不是 VPN、TUN 控制器或系统代理工具。
上游代理必须由 Clash Verge 等现有工具提供。

五个默认决策：

1. `自动` 内核模式只读取 Codex 配置，不擅自修改。
2. Windows / WSL 的显式切换是持久操作，写入前备份 `config.toml`。
3. WSL 优先要求 **mirrored 网络模式**；NAT 模式只有在代理地址从 WSL 可达时才继续。
4. Codex 的两层都指向 CodexLane 的 loopback 入口（首次自动分配，之后固定复用），上游端口由用户选择。
5. 首版只记录 HTTPS 域名/端口和连接元数据，不做 MITM，不读取请求内容。

## 2. 为什么必须有双层注入

Codex Desktop 实际包含两个网络层：

| 层 | 进程 | 生效方式 |
|---|---|---|
| 应用界面、登录页 | `ChatGPT.exe` / `Codex.exe` 主进程 | `--proxy-server=...` |
| Rust 内核 | Windows 或 WSL 中的 `codex app-server` | `HTTP_PROXY` / `HTTPS_PROXY` / `ALL_PROXY` / `NO_PROXY` |

只设置环境变量，界面层仍然直连；只传 `--proxy-server`，内核仍然直连。
两层同时处理才是本工具存在的理由。

Codex 桌面端的外壳始终运行在 Windows。所谓“Windows 环境 / WSL 环境”，准确含义是
**Rust 内核运行在哪里**，由 `config.toml` 的以下键控制：

```toml
[desktop]
runCodexInWindowsSubsystemForLinux = true
```

## 3. 领域模型

不要把“运行环境”“上游代理”和“入口网关”混成一个枚举。内部使用四个独立概念：

```text
EngineMode    = auto | windows | wsl       # 内核位置
ProxyProtocol = http | https | socks5      # 代理协议
UpstreamProxy = host:port                  # 用户代理监听地址
LaneEndpoint  = host:port                  # CodexLane 入口地址，首次分配后固定
```

最终解析出一份不可变的启动计划：

```text
LaunchPlan
├─ executablePath       实际启动的 ChatGPT.exe / Codex.exe
├─ workingDirectory     %USERPROFILE%
├─ engine               windows | wsl
├─ laneEndpoint         CodexLane 监听地址
├─ upstreamProxy        用户代理地址
├─ interfaceProxyURL    给应用界面层的 --proxy-server，指向 laneEndpoint
├─ kernelProxyURL       给 Rust 内核的环境变量，指向 laneEndpoint
└─ noProxy              localhost,127.0.0.1,::1
```

`interfaceProxyURL` 与 `kernelProxyURL` 分开保存，为后续 WSL NAT 和分层路由留出空间。
首版 WSL 使用 mirrored，二者相同。

## 4. 架构选型

| 方案 | 结论 | 原因 |
|---|---|---|
| **Go + Wails v3 + Svelte/TS** | **采用** | Go 做核心，Web UI 适合实时日志、表格和图表；单 exe，Win11 自带 WebView2 |
| Go + `lxn/walk` | 不采用 | 体积最小，但复杂表格、图表和现代界面开发效率低 |
| PowerShell + `.cmd` | 不采用 | 上手快，但 GUI、测试、错误处理和分发体验差 |
| C# / WPF / WinUI | 不采用 | 需要 .NET SDK 或运行时，与用户约束冲突 |
| Rust + egui / Tauri | 不采用 | 技术可行，但双语言和交叉编译成本更高 |
| Electron / Node | 不采用 | 体积大、运行时重，与轻量工具定位不符 |

Wails v3.0.0 已于 2026-10-26 稳定发布。Go 后端直接绑定方法、发送事件，前端不需要
本地 HTTP 管理接口。WebView2 在 Windows 11 上默认存在；Windows 10 需要 Evergreen Runtime，
程序会在 `app.Run()` 前读取注册表预检，缺失时用原生弹窗引导安装。
Node 只在构建时使用。预期语言比例为 Go 65%–75%、TS/JS 20%–30%、HTML/CSS 5% 左右。

### 进程形态

- 单个桌面进程，内含 Wails GUI 和转发网关 goroutine；无服务、无驱动。
- 网关必须持续运行；关闭主窗口默认最小化到托盘。
- 真正退出时提示“退出并恢复原生 Codex”，避免 Codex 失去网络。
- GUI 单实例通过命名 Mutex 实现。
- 可选 headless 模式只运行网关和诊断，供高级用户和 CI 使用。

## 5. 模块划分

```text
cmd/codexlane/              参数解析、退出码
internal/app/               启动编排、LaunchPlan 生成
internal/config/            launcher.json、校验、默认值、迁移
internal/codex/
  locate.go                 MSIX 包与 exe 定位
  engine.go                 config.toml 读取、切换、备份
  instance.go               主进程检测、命令行匹配、安全结束
internal/gateway/           入口 HTTP 代理、CONNECT 隧道、上游转发、限流与统计
internal/proxy/             上游协议探测、HTTP CONNECT、SOCKS5 握手
internal/observe/           请求元数据、脱敏、环形缓冲、JSONL 持久化
internal/launch/            环境变量、参数、直接 CreateProcess
internal/preflight/         Windows / WSL 启动前检查
internal/diagnose/          --doctor 与 JSON 输出
internal/platform/windows/  AppX、Win32 API、路径、编码
internal/platform/wsl/      distro、interop、网络模式、WSL 内探测
internal/ui/wails/          Wails 应用、绑定、事件、托盘
frontend/                   Svelte 5 + TypeScript + CSS
```

依赖方向固定为：

```text
CLI / Wails -> app -> ports <- platform adapters
                       |
                       +-> codex / gateway / proxy / preflight / launch / observe
```

`locate`、`config`、`process`、`wsl`、`proxy probe` 都定义接口，便于在 Linux 上跑纯逻辑测试，
在 Windows CI 上跑集成测试。

## 6. 启动流程

```text
读取 launcher.json 与命令行参数
  ↓
定位 OpenAI.Codex MSIX 包，查找 app\ChatGPT.exe 或 app\Codex.exe
  ↓
解析 EngineMode
  ├─ auto    -> 读取 config.toml，跟随现有值
  ├─ windows -> 确保配置键为 false
  └─ wsl     -> 确保配置键为 true，并执行 WSL 预检
  ↓
解析代理协议和监听地址
  ↓
启动本地 gateway，绑定 127.0.0.1 的空闲端口，并写入配置以便重启复用
  ↓
探测上游代理，并通过 gateway 做一次端到端自检
  ↓
检查已有 Codex 主实例
  ├─ 已是目标代理 -> 激活窗口并退出 0
  ├─ 未带代理或代理不同 -> 确认后结束，等待进程退出
  └─ --native 且已有代理实例 -> 确认后结束，按原生模式启动
  ↓
组合环境变量和 --proxy-server 参数，全部指向 gateway
  ↓
直接启动 exe，cwd = %USERPROFILE%
  ↓
GUI 保持运行，实时接收请求元数据并写日志
```

必须直接启动 exe。通过 Explorer、开始菜单或 `shell:AppsFolder` 激活时，
MSIX 进程不会继承启动器设置的环境变量。

## 7. 自动识别设计

### 7.1 内核位置

`auto` 的含义是“尊重 Codex 当前配置”，不是根据 CodexLane 自身运行在哪个 shell 来猜：

1. 读取 `CODEX_HOME`，未设置则使用 `%USERPROFILE%\.codex`。
2. 解析 `config.toml` 的 `[desktop].runCodexInWindowsSubsystemForLinux`。
3. 键存在且为 `true` -> WSL；为 `false` 或键不存在 -> Windows。
4. 文件无法读取或 TOML 非法 -> 报错并建议运行 `--doctor`，不静默回退。

显式 `windows` / `wsl` 会修改该键。写入必须满足：

- 先备份为 `config.toml.bak-YYYYMMDD-HHmmss`。
- 只改这一个键，保留注释、缩进和其他配置。
- 写入前重新解析验证，使用临时文件 + 原子替换。
- 失败时不启动 Codex，避免“以为切了、实际没切”。

### 7.2 代理地址

用户选择的是上游代理端口，例如 `127.0.0.1:7897`。Codex 实际连接的是
CodexLane 入口 `127.0.0.1:<固定端口>`，两者在界面中明确区分。

WSL 模式按网络类型解析：

| WSL 网络 | 首版行为 |
|---|---|
| mirrored | gateway 监听 `127.0.0.1`，界面层和 WSL 内核都访问该地址 |
| NAT | 预检失败并给出明确说明；首版要求切换 mirrored |

NAT 下不能假设 `127.0.0.1` 能访问 Windows 网关。把 gateway 暴露到局域网会扩大攻击面，
因此不作为首版默认行为。失败时提示：

- 首选：把 WSL 切换为 mirrored。
- 或：让代理允许局域网连接，并放行对应端口。
- 或：在 WSL 内单独运行代理，再使用该地址。

`interfaceProxyURL` 和 `kernelProxyURL` 分开，为后续分层路由和 WSL NAT 留出扩展点。

### 7.3 代理协议

默认 `http`。对 Clash mixed-port，HTTP 代理同样可以通过 `CONNECT` 承载 HTTPS 流量，
兼容性最好。用户可显式选择：

- `http://host:port`
- `https://host:port`
- `socks5://host:port`（首版不支持认证）

HTTPS 探测需要完成到代理的 TLS 握手；SOCKS5 探测需要完成方法协商和 CONNECT。
探测失败时默认阻止启动，用户可在确认警告后强制启动。

## 8. 本地网关与请求观测

仅设置代理启动参数无法产生请求日志。CodexLane 必须在路径中：

```text
Codex 界面层 ─┐
              ├─> CodexLane Gateway -> 上游代理 -> 网络
Codex Rust/WSL ┘
```

入口只接受 HTTP 代理请求，支持 HTTP 明文转发和 HTTPS `CONNECT` 隧道。上游可以是
HTTP、HTTPS 或 SOCKS5。这样界面层始终使用稳定的 HTTP 入口，上游协议由 Go 适配。

可记录：

- 时间、来源（界面 / 内核 / 自检）、目标主机、端口、耗时。
- CONNECT 结果、连接失败原因、上下行字节。
- 明文 HTTP 的方法、路径、状态码。
- 入口到上游的代理路径和当前上游地址。

不可记录（首版）：

- HTTPS 的路径、查询参数、Header、Cookie、请求体和响应体。
- TLS 内层状态码；只能记录 CONNECT 是否成功。
- 未经过 gateway 的直连流量。

要记录完整 HTTPS URL，需要本地 CA + MITM，并解决界面层与 Rust 两侧的信任问题。
这会改变安全模型，首版明确不做。

日志默认写 `%LOCALAPPDATA%\CodexLane\logs\requests-YYYYMMDD.jsonl`，内存保留最近
10,000 条供界面展示。日志按天和大小轮转，默认保留 14 天。

## 9. 环境变量与启动参数

代理模式注入：

```text
HTTP_PROXY=<kernelProxyURL>
HTTPS_PROXY=<kernelProxyURL>
ALL_PROXY=<kernelProxyURL>
NO_PROXY=localhost,127.0.0.1,::1
```

Windows 环境变量名大小写不敏感。实现时以规范大写字为准，避免重复键造成未定义行为。
`NO_PROXY` 必须保留 loopback，确保 magpie（`127.0.0.1:3425`）、追踪面板
（`127.0.0.1:3426`）和 OAuth 本地回调不被代理接管。

界面层参数：

```text
--proxy-server=<interfaceProxyURL>
--disable-quic
```

`--disable-quic` 防止界面层绕过 HTTP 代理使用 UDP/QUIC。不设置系统代理，
不修改 `ProxyEnable`，不调用 TUN。工作目录固定为 `%USERPROFILE%`，避免 UNC 路径导致
`cmd.exe` 和子进程启动异常。

Codex 会把代理环境变量通过 `WSLENV` 传给 WSL 内核，因此不需要修改 `.bashrc`。
这一行为依赖 Codex 自身实现，`--doctor` 会在内核运行时验证实际进程环境。

## 10. 单实例守卫

Codex Desktop 是单实例应用。已有实例运行时再启动，只会激活旧窗口，不会重新读取环境变量。

检测规则：

1. 通过 WMI/CIM 查询 `ChatGPT.exe`、`Codex.exe`。
2. 排除命令行含 `--type=` 的界面子进程。
3. 读取主进程命令行中的 `--proxy-server=`。
4. 目标一致 -> 激活窗口，退出 0。
5. 目标不一致 -> 默认弹窗确认；确认后只结束主进程，等待退出后重新启动。

CLI 在非交互模式必须要求 `--yes` 才能结束现有实例。不要使用 `taskkill /IM` 盲目结束所有进程。

### 10.1 进程边界

Codex Desktop 的主进程和内部界面子进程都属于同一个 MSIX 安装包：

- 主进程：`ChatGPT.exe --proxy-server=...`
- 内部子进程：`ChatGPT.exe --type=renderer|utility|gpu-process|...`

内部界面子进程必须继承 Codex 的代理参数；否则界面和 OAuth 页面会重新直连。
CodexLane 不按通用进程名扫描或操作进程，只处理从 Codex 包路径启动的进程树。

实现时必须：

1. 只直接启动 `Get-AppxPackage OpenAI.Codex` 安装目录下的 exe。
2. 代理参数只传给该次 `CreateProcess` 的主进程。
3. 不写用户级或系统级 `HTTP_PROXY` / `HTTPS_PROXY`。
4. 单实例检测只匹配 Codex 包路径和 Codex 主进程。
5. CodexLane 自己使用 Wails/WebView2 时，其 UI 进程不接入代理。
6. `--doctor` 输出 Codex 进程树和代理参数，不枚举无关系统进程。

验收时必须同时确认：

- Codex 主进程含 `--proxy-server=127.0.0.1:<lanePort>`。
- Codex 内部子进程均由同一 MSIX 包路径启动并继承代理。
- 关闭 CodexLane 不会修改 Windows 网络设置。

## 11. 配置模型

配置文件位于系统本地配置目录，名称为 `launcher.json`：

```text
%LOCALAPPDATA%\CodexLane\launcher.json
```

旧版本放在 exe 同目录的配置会在首次启动时校验并迁移；迁移成功后删除旧文件，
不会在桌面或安装目录继续生成配置文件。

```json
{
  "schemaVersion": 1,
  "upstreamProxy": {
    "protocol": "http",
    "host": "127.0.0.1",
    "port": 7897,
    "noProxy": "localhost,127.0.0.1,::1"
  },
  "lane": {
    "listenHost": "127.0.0.1",
    "listenPort": 0
  },
  "engine": "auto",
  "askBeforeKill": true,
  "requireHealthyProxy": true,
  "logging": {
    "requestMetadata": true,
    "retentionDays": 14,
    "maxMemoryRows": 10000
  }
}
```

校验规则：

- 端口范围 `1..65535`，协议只允许 `http`、`https`、`socks5`。
- `host` 只接受主机名或 IP，不接受 URL、路径和命令行参数。
- 配置损坏时保留原文件并报错，不覆盖用户配置。
- 配置写入采用临时文件 + rename，避免断电或崩溃留下半个文件。
- 首版不保存代理用户名和密码。
- `listenPort=0` 表示由系统分配随机 loopback 端口，避免冲突。

`engine` 的语义：

| 值 | 行为 |
|---|---|
| `auto` | 只读取 `config.toml`，不修改 |
| `windows` | 确保内核在 Windows 运行 |
| `wsl` | 确保内核在 WSL 运行 |

显式值代表用户意图；若与当前 `config.toml` 不一致，启动时应用并提示 Codex 需要重启。

## 12. 命令行与 GUI

### CLI

```text
CodexLane.exe                         按配置启动
CodexLane.exe --native                不注入代理
CodexLane.exe --port 7897             修改端口并启动
CodexLane.exe --protocol socks5       选择代理协议
CodexLane.exe --engine auto|windows|wsl
CodexLane.exe --headless              只运行网关和诊断，不开窗口
CodexLane.exe --yes                   非交互确认重启
CodexLane.exe --doctor [--json]       诊断
CodexLane.exe --verbose
```

`--doctor` 输出：

- Codex 包名、版本、安装路径、实际 exe。
- 当前内核形态及其配置依据。
- 代理 TCP、HTTP CONNECT / SOCKS5 握手结果。
- 经代理访问 `auth.openai.com` 的状态码和耗时。
- gateway 入口端口、监听状态和自检结果。
- WSL distro、interop、网络模式、内核路径。
- 已有主进程 PID、命令行是否带 `--proxy-server`。
- 系统代理和 TUN 的只读状态。

JSON 模式用于开源 issue 模板，必须对 token、cookie、用户名路径等敏感信息做脱敏。

### GUI

Wails 单窗口，使用侧边栏和四个页面：

1. `概览`：上游状态、网关入口、内核模式、Codex 实例、流量统计和最近错误。
2. `请求日志`：实时表格、按来源/域名/结果过滤、暂停、清空、导出和详情面板。
3. `连通测试`：TCP、CONNECT、经网关 GET、直连对比，结果标记 PASS/WARN/FAIL。
4. `设置`：协议、上游地址/端口、内核模式、日志保留、诊断和重启策略。

请求表列：时间、来源、方法、目标、代理、状态、耗时、上/下行字节。
测试请求标记为 `SELF-TEST`，与 Codex 真实流量区分。

Go 向 Svelte 推送事件时按 100ms 批量刷新，避免高频请求拖慢 WebView；前端使用虚拟列表，
内存只保留配置上限。

“开启 / 关闭代理”的准确行为是“用代理环境重启 Codex / 用原生环境重启 Codex”。
改变端口或内核后，只有重启 Codex 才生效，界面必须明确提示。

## 13. 安全与不变量

必须始终成立：

- 不写 `HKCU\...\Internet Settings` 的代理开关。
- 不调用 `netsh winhttp set proxy`。
- 不启用 TUN，不安装驱动，不请求管理员权限。
- 不修改 Clash 配置、规则、节点或订阅。
- 只从 `Get-AppxPackage OpenAI.Codex` 的可信安装目录启动，不接受任意 exe 路径。
- 结束进程前确认，且只匹配 Codex 主进程。
- 日志不记录环境变量全量内容、token、cookie、账号信息。
- 不把用户输入拼接进 PowerShell 命令；WSL 探测使用固定脚本和整数参数。
- gateway 只监听 loopback，随机端口，不提供局域网代理。
- 不记录 HTTPS 明文内容，不安装根证书，不做 MITM。
- WebView 只加载内嵌资源，设置 CSP，不加载远程页面。

## 14. 错误处理

每个失败都必须回答三个问题：发生了什么、为什么、下一步做什么。

| 场景 | 行为 |
|---|---|
| 未安装 Codex | 提示从 Microsoft Store 安装，不弹通用异常 |
| 代理端口不可达 | 显示协议、地址、端口和探测耗时 |
| WSL 无发行版 | 提示 `wsl --install` 或启动默认发行版 |
| WSL interop 异常 | 提示执行 `wsl --shutdown` 后重试 |
| WSL 内核文件缺失 | 提示先启动一次 Codex 或检查版本 |
| NAT 下代理不可达 | 给出 mirrored / allow LAN / WSL 内代理三条路径 |
| config.toml 非法 | 不写入，提示备份位置和具体行号 |
| 结束实例失败 | 提示手动关闭 Codex，不继续启动 |

统一错误码，如 `CL-1001`，便于 README 和 issue 搜索。退出码 0 表示成功，
2 表示配置错误，3 表示预检失败，4 表示用户取消，5 表示启动失败。

## 15. 测试与发布

### 15.1 分层连通测试

测试不能只判断“端口是否打开”，需要按链路分层。结果统一为
`PASS / WARN / FAIL / SKIP`，并显示依据、耗时、期望值和修复建议。

#### A. 网络与代理

| ID | 检查 | 方法 | 通过标准 |
|---|---|---|---|
| A1 | Lane 入口监听 | TCP connect `127.0.0.1:<lanePort>` | 3s 内连接成功 |
| A2 | 上游代理 TCP | 连接用户选择的上游地址 | 2s 内连接成功 |
| A3 | HTTP CONNECT | 经上游 CONNECT `auth.openai.com:443` | 返回 200 |
| A4 | HTTPS 端到端 | 经 Lane GET `https://auth.openai.com/.well-known/openid-configuration` | 200 且 JSON 含 `issuer` |
| A5 | 直连对比 | 不经 Lane 请求 A4 地址 | 允许 403/超时，标记“预期差异”而非失败 |

#### B. 登录与凭证

| ID | 检查 | 方法 | 通过标准 |
|---|---|---|---|
| B1 | Codex 登录状态 | 目标引擎的 `codex login status` | 退出码 0，识别 ChatGPT/API Key 登录 |
| B2 | 凭证文件 | 仅读取 `auth.json` 的键和元数据 | `auth_mode` 存在；不读取、不记录 token |
| B3 | 本地配置 | `codex doctor --json` 的 `config.load` | `status=ok`，TOML 可解析 |
| B4 | 启动配置 | `codex app-server --print-startup-config` | 退出码 0，输出合法 JSON |
| B5 | 登录发现端点 | 经 Lane 请求 A4，并检查 OAuth 字段 | 端点字段完整 |

登录测试不等于自动完成 OAuth。未登录时显示 `WARN`，提供“打开登录”按钮；
绝不读取或导出 access token、refresh token 或账号 Cookie。

#### C. 配置、模型与 Provider

| ID | 检查 | 方法 | 通过标准 |
|---|---|---|---|
| C1 | Provider 环境 | `codex doctor --json` 的 `network.env` | 代理变量和 `NO_PROXY` 可读 |
| C2 | Provider HTTP | `codex doctor --json` 的 `network.provider_reachability` | 路由可达；401/405 视为可达 |
| C3 | Responses WebSocket | `codex doctor --json` 的 `network.websocket_reachability` | 完成 HTTP 101 握手 |
| C4 | 模型目录 | `codex debug models` 或 app-server `model/list` | 返回非空模型列表 |
| C5 | 远程配置 | 监控 Codex 启动日志与 Lane 流量 | 无 `startup policy read failed` |

Provider 测试必须区分本地 magpie 端点和远程 OpenAI。magpie（默认 `3425`）属于
`NO_PROXY`，直连成功是预期；不能把“本地端点 401/405”误判为外网失败。

#### D. 会话与进程

| ID | 检查 | 方法 | 通过标准 |
|---|---|---|---|
| D1 | Codex 主进程 | 读取主进程命令行 | 含 `--proxy-server=127.0.0.1:<lanePort>` |
| D2 | WSL 内核环境 | 读取 `codex app-server` 的 `/proc/<pid>/environ` | 含 HTTP/HTTPS/NO_PROXY |
| D3 | 会话流量 | 打开或恢复一个会话，观察 Lane 日志 | 60s 内出现 `source=kernel` 的连接 |
| D4 | 真实会话（可选） | `codex exec` 发送最小提示词 | 收到模型回复 |
| D5 | 无会话 | app-server 未运行 | `SKIP`，不判失败 |

D4 会消耗模型额度并写入会话记录，必须二次确认，默认不运行。测试提示词固定为
`Reply with exactly OK`，使用只读 sandbox 和短超时，不执行任何工具调用。

#### E. 隔离与安全

| ID | 检查 | 方法 | 通过标准 |
|---|---|---|---|
| E1 | 系统代理 | 只读注册表 `ProxyEnable` | 0 |
| E2 | Clash TUN / 系统代理 | 只读 `verge.yaml` | 均为 false |
| E3 | 进程来源 | 核对主进程路径与 `Get-AppxPackage` 安装目录 | 只操作 Codex 包内进程 |
| E4 | loopback 绕过 | 检查 `NO_PROXY` | 含 localhost/127.0.0.1/::1 |
| E5 | 权限 | 当前进程令牌 | 非管理员，未安装服务/驱动 |

#### 执行策略

- “运行安全测试”执行 A、B、C、E 和 D1/D2；不重启 Codex，不消耗额度。
- D3 需要 Codex 有活动会话；无会话时显示 `SKIP` 和操作提示。
- D4 作为独立按钮，明确显示“消耗额度”。
- 测试结果独立于 `--doctor` 的被动快照，可复制 JSON。
- 每次测试写入一条 `source=self-test` 的 Lane 日志，便于验证日志链路。

### 自动化测试

- 纯逻辑：配置校验、端口边界、引擎解析表、候选地址生成、NO_PROXY、参数构造。
- TOML：保留注释、键存在/缺失、重复键、损坏文件、原子写入、备份轮换。
- 实例匹配：主进程/子进程、不同代理、无命令行权限。
- 代理探测：HTTP 200、407、超时、SOCKS5 成功/拒绝。
- 网关：CONNECT 隧道、半关闭、超时、取消、并发、字节统计、日志脱敏。
- 前端：日志批量渲染、筛选、暂停、空状态、长域名截断。
- Windows 集成：AppX 定位、直接 CreateProcess、cwd、退出码。
- 手工验收：Windows 内核、WSL mirrored、WSL NAT 失败提示、裸实例重启。

Go 纯逻辑在 Linux 上测试，前端使用 Vitest，Windows 集成和 Wails 打包在
`windows-latest` 上执行。Wails 构建依赖 Go、Node 和 WebView2 开发组件。

### 构建

使用 Wails v3.0.0 构建。前端由 Vite 产出静态资源并嵌入 exe；发布物为单 exe，
运行配置保存在 `%LOCALAPPDATA%\CodexLane`。WebView2 在 Windows 11 上无需额外安装，
缺失时启动前会提示安装。首版不强制代码签名，
但 release 提供 SHA-256。

## 16. 功能优先级

### P0 / M1：可用的核心

- 定位 Codex MSIX 包和两种 exe 名称。
- `launcher.json` 读写、端口/协议/主机切换。
- 本地 HTTP/CONNECT 网关和上游 HTTP/HTTPS/SOCKS5 转发。
- 双层注入：环境变量 + `--proxy-server`，均指向网关。
- 内核 `auto / windows / wsl` 解析与安全切换。
- WSL 发行版、interop、内核文件、网络模式预检。
- 上游代理和端到端自检，明确的失败提示。
- 单实例检测、安全重启、工作目录固定。
- CLI 启动与 `--doctor`。
- 请求元数据日志、脱敏、轮转和内存环形缓冲。
- 不修改系统代理、TUN、Clash。

### P1 / M2：桌面体验

- Wails + Svelte 单窗口控制台和托盘。
- 状态卡、一键代理启动、一键原生启动、保存并重启。
- 实时请求日志、筛选、详情、暂停、导出。
- 连通测试和直连/代理对比。
- `--doctor --json` 和脱敏。
- 最近使用的端口/协议，不自动扫描端口。
- 错误码和 README 故障排查。

### P2 / M3：发布与增强

- 双语 README、MIT License、GitHub Actions、首个 release。
- 图标、版本资源、Per-Monitor V2 DPI。
- 可选托盘和开机启动。
- 代码签名、自动更新。
- 代理认证（需要 Windows Credential Manager 和界面层认证策略）。
- WSL 侧代理监听、WSL 内 helper。

## 17. 明确不做

- 系统代理或 TUN 开关。
- Clash 节点、规则、订阅和进程规则管理。
- 启动、停止或安装代理软件。
- Codex 登录、账号、额度和 API Key 管理。
- 登录流程自动化、OAuth 流程控制。
- macOS / Linux 桌面版。
- 首版的代理用户名密码认证。
- 伪装成全局 VPN 或“所有进程自动代理”；这需要 WPF/WFP/TUN 或进程注入，超出本项目定位。

## 18. 验收基线

1. 主进程命令行含目标 `--proxy-server=`。
2. WSL 内核进程环境含目标 `HTTPS_PROXY`。
3. `ProxyEnable=0`、Clash `enable_system_proxy=false`、`enable_tun_mode=false` 保持不变。
4. 切换端口后，未重启时状态明确显示“待重启”；重启后命令行和环境变量同步更新。
5. WSL 异常时 `--doctor` 给出具体原因，Codex 不会静默卡在启动页。
6. 已有裸实例时，经确认后只结束 Codex 主进程并重新启动。
