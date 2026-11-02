package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/codex"
	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/gateway"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/preflight"
)

const authDiscoveryURL = "https://auth.openai.com/.well-known/openid-configuration"
const providerModelsURL = "https://chatgpt.com/backend-api/codex/models"

type domainCheck struct {
	id   string
	host string
}

func (s *AppService) runTests(ctx context.Context) model.TestReport {
	started := time.Now()
	snapshot := s.snapshotWith(ctx, true, false)
	results := make([]model.TestResult, 0, 18)
	add := func(id, group, name, status, detail string, latencyMs int64, hint string) {
		results = append(results, model.TestResult{
			ID: id, Group: group, Name: name, Status: status, Detail: detail,
			LatencyMs: latencyMs, Hint: hint,
		})
	}

	groupA := "network"
	laneURL := snapshot.Lane.InterfaceURL
	if snapshot.Lane.Running {
		address := fmt.Sprintf("127.0.0.1:%d", snapshot.Lane.InterfacePort)
		latency, err := gateway.CheckTCPService(ctx, address)
		if err != nil {
			add("A1", groupA, "Lane 本地监听", "FAIL", err.Error(), latency.Milliseconds(), "重启 Lane 或检查本地端口占用")
		} else {
			add("A1", groupA, "Lane 本地监听", "PASS", address+" 可建立连接", latency.Milliseconds(), "")
		}
	} else {
		add("A1", groupA, "Lane 本地监听", "FAIL", "Lane 未运行", 0, "先在总览页开启通道")
	}

	upstreamAddress := fmt.Sprintf("%s:%d", snapshot.Settings.Host, snapshot.Settings.Port)
	latency, err := gateway.CheckTCPService(ctx, upstreamAddress)
	if err != nil {
		add("A2", groupA, "上游端口监听", "FAIL", err.Error(), latency.Milliseconds(), "确认代理软件正在监听所选端口")
	} else {
		add("A2", groupA, "上游端口监听", "PASS", upstreamAddress+" 握手成功", latency.Milliseconds(), "")
	}

	upstream := model.Upstream{
		Protocol: snapshot.Settings.Protocol,
		Host:     snapshot.Settings.Host,
		Port:     snapshot.Settings.Port,
		NoProxy:  snapshot.Settings.NoProxy,
	}
	latency, err = gateway.CheckUpstreamTunnel(ctx, upstream, "auth.openai.com:443")
	if err != nil {
		add("A3", groupA, "上游 CONNECT 响应", "FAIL", err.Error(), latency.Milliseconds(), "检查代理协议、端口和节点状态")
	} else {
		add("A3", groupA, "上游 CONNECT 响应", "PASS", "auth.openai.com:443 隧道已建立", latency.Milliseconds(), "")
	}

	authStatus, authBody, authLatency, authErr := gateway.HTTPBodyThroughProxy(ctx, laneURL, authDiscoveryURL)
	if authErr != nil {
		authStatus, authBody, authLatency, authErr = gateway.HTTPBodyThroughProxy(ctx, laneURL, authDiscoveryURL)
	}
	authValid := authStatus == http.StatusOK && json.Valid(authBody) && strings.Contains(string(authBody), `"issuer"`)
	if authErr != nil {
		add("A4", groupA, "Lane HTTPS 端到端", "FAIL", authErr.Error(), authLatency.Milliseconds(), "检查 Lane 与上游代理链路")
	} else if authValid {
		add("A4", groupA, "Lane HTTPS 端到端", "PASS", "OAuth 发现端点 200 · JSON 有效", authLatency.Milliseconds(), "")
	} else {
		add("A4", groupA, "Lane HTTPS 端到端", "FAIL", fmt.Sprintf("HTTP %d · JSON 或 issuer 缺失", authStatus), authLatency.Milliseconds(), "代理端口可连但无法出网时，先切换 Clash 节点")
	}

	directStatus, directLatency, directErr := directHTTPStatus(ctx, authDiscoveryURL)
	switch {
	case authValid && directErr != nil:
		add("A5", groupA, "直连对照", "PASS", "直连失败，经 Lane 可访问 · 符合隔离预期", directLatency.Milliseconds(), "")
	case authValid && directStatus != http.StatusOK:
		add("A5", groupA, "直连对照", "PASS", fmt.Sprintf("直连 HTTP %d，经 Lane HTTP %d", directStatus, authStatus), directLatency.Milliseconds(), "")
	case authValid:
		add("A5", groupA, "直连对照", "PASS", "当前网络直连也可达，不影响 Lane 独立转发", directLatency.Milliseconds(), "")
	case directErr == nil:
		add("A5", groupA, "直连对照", "FAIL", fmt.Sprintf("直连 HTTP %d 可达，经 Lane 反而失败", directStatus), directLatency.Milliseconds(), "问题在 Lane 或上游代理，不在本地网络")
	default:
		add("A5", groupA, "直连对照", "SKIP", "直连与经 Lane 均不可达", directLatency.Milliseconds(), "先检查本机网络")
	}

	domainChecks := []domainCheck{
		{"A6", "openai.com"},
		{"A7", "chatgpt.com"},
		{"A8", "oaistatic.com"},
		{"A9", "files.oaiusercontent.com"},
	}
	domainResults := make([]model.TestResult, len(domainChecks))
	var domainWG sync.WaitGroup
	for index, check := range domainChecks {
		domainWG.Add(1)
		go func(index int, check domainCheck) {
			defer domainWG.Done()
			probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			statusCode, version, latency, err := gateway.CheckHostTLSViaProxy(probeCtx, laneURL, check.host)
			if err != nil && transientProbeError(err) && authErr == nil {
				retryCtx, retryCancel := context.WithTimeout(ctx, 8*time.Second)
				statusCode, version, latency, err = gateway.CheckHostTLSViaProxy(retryCtx, laneURL, check.host)
				retryCancel()
			}
			result := model.TestResult{
				ID: check.id, Group: groupA, Name: check.host + " 连通",
				LatencyMs: latency.Milliseconds(),
			}
			switch {
			case err != nil:
				result.Status = "FAIL"
				result.Detail = err.Error()
				result.Hint = "在 Clash 规则中把 " + check.host + " 指向非 DIRECT 的代理组"
			case statusCode >= 500:
				result.Status = "WARN"
				result.Detail = fmt.Sprintf("HTTP %d · 源站返回服务端错误", statusCode)
				result.Hint = "更换节点或稍后重试"
			case statusCode == 0:
				result.Status = "PASS"
				result.Detail = version + " 握手成功 · 源站已接受连接"
			default:
				result.Status = "PASS"
				result.Detail = fmt.Sprintf("%s · HTTP %d · 经 Lane 到达源站", version, statusCode)
			}
			domainResults[index] = result
		}(index, check)
	}
	domainWG.Wait()
	for _, result := range domainResults {
		add(result.ID, result.Group, result.Name, result.Status, result.Detail, result.LatencyMs, result.Hint)
	}

	groupB := "login"
	loginDetail, loginOK, loginWarn := s.loginStatus(ctx, snapshot)
	switch {
	case loginOK:
		add("B1", groupB, "Codex 登录状态", "PASS", loginDetail, 0, "")
	case loginWarn:
		add("B1", groupB, "Codex 登录状态", "WARN", loginDetail, 0, "打开 Codex 完成登录后重试")
	default:
		add("B1", groupB, "Codex 登录状态", "FAIL", loginDetail, 0, "检查 Codex 登录状态")
	}

	configPath := filepath.Join(snapshot.Environment.CodexHome, "config.toml")
	if _, err := codex.ReadEngine(configPath); err != nil {
		add("B2", groupB, "配置加载", "FAIL", err.Error(), 0, "备份并修复 config.toml 后重试")
	} else {
		add("B2", groupB, "配置加载", "PASS", "config.toml 可读取，桌面段解析正常", 0, "")
	}

	if snapshot.Engine.Error != "" {
		add("B3", groupB, "内核环境", "FAIL", snapshot.Engine.Error, 0, "检查 config.toml 的 desktop 段")
	} else {
		detail := "解析为 " + strings.ToUpper(snapshot.Engine.Resolved)
		if snapshot.Engine.Mode == model.EngineAuto {
			detail += " · 自动跟随配置"
		}
		add("B3", groupB, "内核环境", "PASS", detail, 0, "")
	}

	providerStatus, providerLatency, providerErr := gateway.HTTPThroughProxy(ctx, laneURL, providerModelsURL)
	switch {
	case providerErr != nil:
		add("B4", groupB, "Provider 请求", "FAIL", providerErr.Error(), providerLatency.Milliseconds(), "检查 chatgpt.com 是否可达")
	case providerStatus == http.StatusOK || providerStatus == http.StatusUnauthorized || providerStatus == http.StatusForbidden:
		add("B4", groupB, "Provider 请求", "PASS", fmt.Sprintf("HTTP %d · 路由可达", providerStatus), providerLatency.Milliseconds(), "")
	default:
		add("B4", groupB, "Provider 请求", "WARN", fmt.Sprintf("HTTP %d · 非预期响应", providerStatus), providerLatency.Milliseconds(), "检查代理节点与区域策略")
	}

	dnsCtx, dnsCancel := context.WithTimeout(ctx, 5*time.Second)
	addresses, dnsErr := net.DefaultResolver.LookupHost(dnsCtx, "auth.openai.com")
	dnsCancel()
	if dnsErr != nil {
		add("B5", groupB, "登录域解析", "FAIL", dnsErr.Error(), 0, "检查 DNS 设置，或让 Clash 接管域名解析")
	} else if len(addresses) == 0 {
		add("B5", groupB, "登录域解析", "FAIL", "auth.openai.com 没有解析结果", 0, "检查 DNS 设置")
	} else {
		add("B5", groupB, "登录域解析", "PASS", fmt.Sprintf("auth.openai.com → %s", addresses[0]), 0, "")
	}

	groupC := "session"
	proxyTarget := strings.ToLower(snapshot.Lane.InterfaceURL)
	if !snapshot.Codex.Running {
		add("C1", groupC, "Codex 主进程", "SKIP", "Codex 当前未运行", 0, "通过 Lane 启动 Codex 后重试")
	} else if snapshot.Codex.ProxyMatched && strings.Contains(strings.ToLower(snapshot.Codex.CommandLine), proxyTarget) {
		add("C1", groupC, "Codex 主进程", "PASS", fmt.Sprintf("PID %d · 代理参数指向 Lane", snapshot.Codex.MainPID), 0, "")
	} else {
		add("C1", groupC, "Codex 主进程", "WARN", "主进程存在，但代理参数不是当前 Lane", 0, "保存设置后重启 Codex")
	}

	childCount := 0
	childOwned := 0
	parents := make(map[int]int, len(snapshot.Codex.Processes))
	for _, process := range snapshot.Codex.Processes {
		parents[process.PID] = process.ParentPID
	}
	for _, process := range snapshot.Codex.Processes {
		if process.IsMain {
			continue
		}
		childCount++
		if descendantOf(parents, process.PID, snapshot.Codex.MainPID) {
			childOwned++
		}
	}
	switch {
	case !snapshot.Codex.Running:
		add("C2", groupC, "界面子进程归属", "SKIP", "Codex 当前未运行", 0, "")
	case childCount == 0:
		add("C2", groupC, "界面子进程归属", "WARN", "未找到界面子进程", 0, "打开 Codex 窗口后重试")
	case childOwned == childCount:
		add("C2", groupC, "界面子进程归属", "PASS", fmt.Sprintf("%d 个子进程均属于 Codex 主进程树", childCount), 0, "")
	default:
		add("C2", groupC, "界面子进程归属", "WARN", fmt.Sprintf("%d/%d 个子进程属于 Codex 主进程树", childOwned, childCount), 0, "确认是否有同名进程混入")
	}

	if snapshot.Engine.Resolved != model.EngineWSL {
		add("C3", groupC, "WSL 内核预检", "SKIP", "当前使用 Windows 内核", 0, "")
	} else if err := preflight.ValidateMode(snapshot.WSL); err != nil {
		add("C3", groupC, "WSL 内核预检", "FAIL", err.Error(), 0, "修复 WSL 后重启 Codex")
	} else {
		add("C3", groupC, "WSL 内核预检", "PASS", snapshot.WSL.Distro+" · mirrored · interop 正常", 0, "")
	}

	kernelSeen := false
	if s.store != nil {
		for _, request := range s.store.List(100) {
			if request.Source == model.SourceKernel && recentRecord(request) {
				kernelSeen = true
				break
			}
		}
	}
	if kernelSeen {
		add("C4", groupC, "活动会话流量", "PASS", "最近 5 分钟捕获到内核请求", 0, "")
	} else {
		add("C4", groupC, "活动会话流量", "SKIP", "最近 5 分钟没有内核请求", 0, "打开一个 Codex 会话后重试")
	}

	groupD := "safety"
	if snapshot.Environment.SystemProxyOn {
		add("D1", groupD, "系统代理保持关闭", "WARN", "检测到 Windows 系统代理开启", 0, "Codex Lane 不会修改它；如需纯净隔离请手动关闭")
	} else {
		add("D1", groupD, "系统代理保持关闭", "PASS", "ProxyEnable=0 · 系统代理未修改", 0, "")
	}
	noProxy := strings.ToLower(snapshot.Settings.NoProxy)
	if strings.Contains(noProxy, "localhost") && strings.Contains(noProxy, "127.0.0.1") && strings.Contains(noProxy, "::1") {
		add("D2", groupD, "NO_PROXY 回环", "PASS", "localhost、127.0.0.1、::1 均已保留", 0, "")
	} else {
		add("D2", groupD, "NO_PROXY 回环", "WARN", "回环地址列表不完整", 0, "恢复默认 NO_PROXY")
	}
	if snapshot.Environment.Administrator {
		add("D3", groupD, "权限边界", "WARN", "当前进程以管理员身份运行", 0, "普通权限即可运行 Codex Lane")
	} else {
		add("D3", groupD, "权限边界", "PASS", "非管理员 · 未安装服务或驱动", 0, "")
	}
	if err := atomicConfigProbe(snapshot.Settings); err != nil {
		add("D4", groupD, "配置写入保护", "FAIL", err.Error(), 0, "检查本地临时目录权限")
	} else {
		add("D4", groupD, "配置写入保护", "PASS", "临时文件 + 原子替换可用", 0, "")
	}

	finished := time.Now()
	report := model.TestReport{
		StartedAt:  started.Format(time.RFC3339),
		FinishedAt: finished.Format(time.RFC3339),
		DurationMs: finished.Sub(started).Milliseconds(),
		Results:    results,
	}
	report.Score, report.Healthy = scoreResults(results)
	s.addEvent(map[bool]string{true: "ok", false: "error"}[report.Healthy], "连通测试完成", fmt.Sprintf("%d 项 · %d 分", len(results), report.Score))
	return report
}

func (s *AppService) diagnose(ctx context.Context) model.DiagnosticReport {
	started := time.Now()
	snapshot := s.snapshot(ctx, true)
	items := make([]model.DiagnosticItem, 0, 10)
	add := func(label, value, state, detail string) {
		items = append(items, model.DiagnosticItem{Label: label, Value: value, State: state, Detail: detail})
	}
	if snapshot.Upstream.Reachable {
		add("上游代理", fmt.Sprintf("%s · %d ms", snapshot.Upstream.URL, snapshot.Upstream.LatencyMs), "pass", "TCP 与 CONNECT 均可用")
	} else {
		add("上游代理", snapshot.Upstream.URL, "fail", snapshot.Upstream.Error)
	}
	if snapshot.Lane.Running {
		add("Lane 入口", fmt.Sprintf("UI :%d / Kernel :%d", snapshot.Lane.InterfacePort, snapshot.Lane.KernelPort), "pass", "仅监听本机回环")
	} else {
		add("Lane 入口", "未运行", "fail", snapshot.Lane.Error)
	}
	if snapshot.Codex.Installed {
		add("Codex 安装", "v"+snapshot.Codex.Version, "pass", snapshot.Codex.ExecutablePath)
	} else {
		add("Codex 安装", "未找到", "fail", snapshot.Codex.Error)
	}
	if snapshot.Engine.Error == "" {
		add("内核环境", strings.ToUpper(snapshot.Engine.Resolved), "pass", snapshot.Engine.Source)
	} else {
		add("内核环境", snapshot.Engine.Mode, "fail", snapshot.Engine.Error)
	}
	if snapshot.Engine.Resolved == model.EngineWSL {
		if err := preflight.ValidateMode(snapshot.WSL); err != nil {
			add("WSL 网络", snapshot.WSL.NetworkMode, "fail", err.Error())
		} else {
			add("WSL 网络", snapshot.WSL.Distro+" · "+snapshot.WSL.NetworkMode, "pass", "interop 与内核文件正常")
		}
	} else {
		add("WSL 网络", "未使用", "skip", "当前内核运行在 Windows")
	}
	if snapshot.Environment.SystemProxyOn {
		add("系统代理", "开启", "warn", "Codex Lane 不修改系统代理")
	} else {
		add("系统代理", "关闭", "pass", "ProxyEnable=0")
	}
	if snapshot.Environment.Administrator {
		add("权限", "管理员", "warn", "普通用户权限即可")
	} else {
		add("权限", "标准用户", "pass", "未请求 UAC")
	}
	add("日志目录", "本地 JSONL", "pass", snapshot.Environment.LogDirectory)
	report := model.DiagnosticReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
		DurationMs:  time.Since(started).Milliseconds(),
		Items:       items,
		Healthy:     true,
	}
	total := 0
	count := 0
	for _, item := range items {
		switch item.State {
		case "pass":
			total += 100
			count++
		case "warn":
			total += 70
			count++
		case "fail":
			total += 0
			count++
			report.Healthy = false
		}
	}
	if count > 0 {
		report.Score = total / count
	}
	return report
}

func (s *AppService) loginStatus(ctx context.Context, snapshot model.Snapshot) (string, bool, bool) {
	if native := codex.FindNativeCodex(ctx); native != "" {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		result, err := platform.Run(checkCtx, native, "login", "status")
		if err == nil {
			return "codex login status 返回成功", true, false
		}
		if result.Code != 0 {
			detail := strings.TrimSpace(result.Stdout)
			if detail == "" {
				detail = strings.TrimSpace(result.Stderr)
			}
			if detail != "" && !strings.Contains(strings.ToLower(detail), "token") {
				return trimForDisplay(detail, 120), false, true
			}
		}
	}
	meta, err := codex.ReadAuthMetadata(snapshot.Environment.CodexHome)
	if err != nil {
		return "auth.json 元数据读取失败", false, false
	}
	if !meta.Exists {
		return "尚未找到 Codex 登录凭据", false, true
	}
	if meta.Mode != "" {
		return "凭据元数据存在 · mode=" + meta.Mode + " · 未读取 Token", true, false
	}
	return "auth.json 已存在 · 未读取 Token", true, false
}

func directHTTPStatus(ctx context.Context, target string) (int, time.Duration, error) {
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:               nil,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: 6 * time.Second,
		},
		Timeout: 8 * time.Second,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, 0, err
	}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return 0, time.Since(started), err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 16*1024))
	return response.StatusCode, time.Since(started), nil
}

func atomicConfigProbe(settings model.Settings) error {
	dir, err := os.MkdirTemp("", "codex-lane-write-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cfg := config.Default()
	cfg.Upstream.Protocol = settings.Protocol
	cfg.Upstream.Host = settings.Host
	cfg.Upstream.Port = settings.Port
	cfg.Upstream.NoProxy = settings.NoProxy
	cfg.Engine = settings.Engine
	cfg.WSLDistro = settings.WSLDistro
	if err := config.Save(filepath.Join(dir, config.FileName), cfg); err != nil {
		return err
	}
	return nil
}

func recentRecord(record model.RequestRecord) bool {
	if record.FinishedAt == "" {
		return false
	}
	value, err := time.Parse(time.RFC3339Nano, record.FinishedAt)
	return err == nil && time.Since(value) < 5*time.Minute
}

func scoreResults(results []model.TestResult) (int, bool) {
	total := 0
	count := 0
	healthy := true
	for _, result := range results {
		switch result.Status {
		case "PASS":
			total += 100
			count++
		case "WARN":
			total += 70
			count++
		case "FAIL":
			count++
			healthy = false
		}
	}
	if count == 0 {
		return 0, false
	}
	return total / count, healthy
}

func trimForDisplay(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

func descendantOf(parents map[int]int, pid int, root int) bool {
	for depth := 0; depth < 8 && pid > 0; depth++ {
		parent, ok := parents[pid]
		if !ok || parent == pid {
			return false
		}
		if parent == root {
			return true
		}
		pid = parent
	}
	return false
}

func transientProbeError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "eof") ||
		strings.Contains(message, "timeout") ||
		strings.Contains(message, "deadline exceeded") ||
		strings.Contains(message, "connection reset")
}
