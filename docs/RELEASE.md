# 发布清单

Codex Lane 的发布形态是 Windows x64 单文件便携版：

- `CodexLane.exe`
- `CodexLane-<version>-win-x64.zip`
- `CodexLane-windows-amd64.exe`
- `SHA256SUMS.txt`
- `latest.json`

不包含安装器、服务、驱动或后台常驻组件。NSIS/MSIX 仍然可以后续增加，
但发布主产物始终保留单 exe。

运行时配置写入 `%LOCALAPPDATA%\CodexLane\launcher.json`，不会随 exe 放在桌面。

## 版本来源

版本号位于 `internal/appservice/service.go` 的 `Version` 常量，同时必须与
`build/windows/info.json` 保持一致。打 tag 前先同步这两个位置。

## 构建

在仓库根目录运行：

```powershell
pwsh -File scripts/release.ps1
```

脚本会依次执行：

1. 从 Go 服务重新生成 Wails TypeScript 绑定。
2. `svelte-check` 和 Vite 生产构建，确保前端匹配最新服务接口。
3. 生成 Windows 图标/版本资源。
4. 使用 `CGO_ENABLED=0`、`-trimpath`、`-tags production` 构建 x64 单文件。
5. 生成自动更新直装 exe、便携 zip、`SHA256SUMS.txt` 与 `latest.json`。

已经构建过 `bin/CodexLane.exe` 时，可以只重新打包：

```powershell
pwsh -File scripts/release.ps1 -SkipBuild
```

## 发布前验收

```powershell
.\bin\CodexLane.exe --version
.\bin\CodexLane.exe --selftest
.\bin\CodexLane.exe --selftest --json
```

自检不会启动桌面窗口，不会打开或重启 Codex。退出码 `0` 表示通过，
`1` 表示存在失败项，`2` 表示自检无法运行。

桌面端还需要手工确认：

- 截图中的总览、请求日志、检查、设置和关于卡片布局正常。
- 窗口可以最小化、最大化、缩放，页面只有一个滚动条。
- 任务栏图标与标题栏图标正常。
- 请求日志能显示经过 Lane 的域名、代理路径、状态和耗时。
- Windows 系统代理仍为关闭，Clash 规则与节点未被修改。

## 发布到 GitHub

1. 提交并推送版本变更。
2. 创建与版本一致的 tag，例如 `v0.1.0`。
3. 新建 GitHub Release，上传 `dist` 下的直装 exe、zip、`SHA256SUMS.txt` 和 `latest.json`。
4. 把 Release 标记为 Latest；应用会从
   `https://github.com/Alexzzz2008/codex-lane/releases/latest/download/latest.json`
   读取版本、下载地址、大小和 SHA-256。

更新包下载到 `%LOCALAPPDATA%\CodexLane\updates`，校验通过后才能安装。
Windows 上会先把当前 exe 改名为 `.old`，放入新版本后启动新进程；新进程等待
旧进程退出再打开窗口和 Lane 端口。启动失败时自动恢复 `.old`，下次启动会清理
已退出的旧版本文件。

更新检查只读取公开 Release。仓库保持私有时，应用会明确显示“无法检查”，
但不会影响代理、请求日志或自检功能。
