<script lang="ts">
  import type { Snapshot, TestResult } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import Icon from "./Icon.svelte";
  import { clockDate, formatDuration } from "./format";

  let {
    snapshot,
    busy = "",
    onRun,
    onExport,
  }: {
    snapshot: Snapshot;
    busy?: string;
    onRun: () => void;
    onExport: () => void;
  } = $props();

  const groups = [
    { key: "network", code: "A", title: "网络与代理" },
    { key: "login", code: "B", title: "登录与配置" },
    { key: "session", code: "C", title: "会话与进程" },
    { key: "safety", code: "D", title: "隔离与安全" },
  ];

  const catalog: Array<{ group: string; id: string; name: string; detail: string }> = [
    { group: "network", id: "A1", name: "Lane 本地监听", detail: "双入口可建立 TCP 连接" },
    { group: "network", id: "A2", name: "上游端口监听", detail: "所选代理端口可握手" },
    { group: "network", id: "A3", name: "上游 CONNECT 响应", detail: "代理是否接受隧道请求" },
    { group: "network", id: "A4", name: "Lane HTTPS 端到端", detail: "OAuth 发现端点实际请求" },
    { group: "network", id: "A5", name: "直连对照", detail: "代理与原生路径差异" },
    { group: "network", id: "A6", name: "openai.com 连通", detail: "经 Lane 访问源站" },
    { group: "network", id: "A7", name: "chatgpt.com 连通", detail: "经 Lane 访问源站" },
    { group: "network", id: "A8", name: "oaistatic.com 连通", detail: "经 Lane 访问源站" },
    { group: "network", id: "A9", name: "files.oaiusercontent.com 连通", detail: "经 Lane 访问源站" },
    { group: "login", id: "B1", name: "Codex 登录状态", detail: "只检查状态与凭据元数据" },
    { group: "login", id: "B2", name: "配置加载", detail: "config.toml 解析" },
    { group: "login", id: "B3", name: "内核环境", detail: "自动 / Windows / WSL" },
    { group: "login", id: "B4", name: "Provider 请求", detail: "chatgpt.com 路由可达" },
    { group: "login", id: "B5", name: "登录域解析", detail: "auth.openai.com 域名解析" },
    { group: "session", id: "C1", name: "Codex 主进程", detail: "代理参数指向当前 Lane" },
    { group: "session", id: "C2", name: "界面子进程归属", detail: "子进程均属于 Codex 主进程树" },
    { group: "session", id: "C3", name: "WSL 内核预检", detail: "interop、内核文件与网络模式" },
    { group: "session", id: "C4", name: "活动会话流量", detail: "最近 5 分钟内核请求" },
    { group: "safety", id: "D1", name: "系统代理保持关闭", detail: "只读 ProxyEnable" },
    { group: "safety", id: "D2", name: "NO_PROXY 回环", detail: "localhost / 127.0.0.1 / ::1" },
    { group: "safety", id: "D3", name: "权限边界", detail: "不需要管理员权限" },
    { group: "safety", id: "D4", name: "配置写入保护", detail: "临时文件 + 原子替换" },
  ];

  const domainIDs = ["A6", "A7", "A8", "A9"];
  const domainHosts = ["openai.com", "chatgpt.com", "oaistatic.com", "files.oaiusercontent.com"];

  const report = $derived(snapshot.testReport);
  const results = $derived(report?.results ?? []);
  const hasReport = $derived(results.length > 0);
  const passCount = $derived(results.filter((item) => item.status === "PASS").length);
  const warnCount = $derived(results.filter((item) => item.status === "WARN").length);
  const failCount = $derived(results.filter((item) => item.status === "FAIL").length);
  const skipCount = $derived(results.filter((item) => item.status === "SKIP").length);

  const domains = $derived(
    domainHosts.map((host, index) => {
      const found = results.find((item) => item.id === domainIDs[index]);
      return (
        found ?? {
          id: domainIDs[index],
          name: `${host} 连通`,
          status: "PENDING",
          detail: "尚未运行",
          latencyMs: 0,
          hint: "",
          group: "network",
        }
      );
    }),
  );
  const domainOK = $derived(domains.filter((item) => item.status === "PASS").length);

  function resultsFor(group: string): TestResult[] {
    if (hasReport) return results.filter((item) => item.group === group);
    return catalog
      .filter((item) => item.group === group)
      .map((item) => ({ ...item, status: "PENDING", latencyMs: 0, hint: "" }));
  }

  function groupCounts(group: string): { pass: number; total: number; skip: number; status: string } {
    const items = resultsFor(group);
    const pass = items.filter((item) => item.status === "PASS").length;
    const skip = items.filter((item) => item.status === "SKIP").length;
    const failed = items.some((item) => item.status === "FAIL");
    const warned = items.some((item) => item.status === "WARN");
    const status = !hasReport ? "pending" : failed ? "fail" : warned ? "warn" : pass === 0 ? "skip" : "pass";
    return { pass, total: items.length, skip, status };
  }

  function pillClass(status: string): string {
    const normalized = (status || "pending").toLowerCase();
    return normalized === "fail" ? "fail" : normalized;
  }
</script>

<section class="view active" id="view-checks" aria-labelledby="checks-title">
  <div class="page-head">
    <div class="page-head-copy">
      <h1 id="checks-title">检查</h1>
      <p class="page-sub">代理链路、OpenAI 域名与 Codex 接入一次跑完；只读检查，不重启 Codex。</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={onExport} disabled={!hasReport}><Icon name="copy" size={15} />复制摘要</button>
      <button class="btn accent" onclick={onRun} disabled={busy === "tests"}>
        {#if busy === "tests"}<span class="spin"><Icon name="refresh" size={15} /></span>{:else}<Icon name="play" size={15} />{/if}
        {busy === "tests" ? "正在检查…" : "运行全部检查"}
      </button>
    </div>
  </div>

  <div class:ok={hasReport && report?.healthy} class:warn={hasReport && !report?.healthy} class="check-banner">
    <div class="check-score">
      {#if busy === "tests"}<span class="spin banner-spin"><Icon name="refresh" size={20} /></span>{:else}{hasReport ? report?.score : "—"}<small>分</small>{/if}
    </div>
    <div class="check-copy">
      <b>{busy === "tests" ? "正在重新检查…" : !hasReport ? "尚未运行检查" : report?.healthy ? "链路状态良好" : "有检查项需要处理"}</b>
      <span>
        {#if busy === "tests"}
          {hasReport ? "下方为上次结果，完成后自动替换。" : "正在访问代理与 OpenAI 域名，请稍候。"}
        {:else if !hasReport}
          检查代理、OpenAI 域名和本机配置，不会发起模型会话。
        {:else}
          {passCount} 通过 · {warnCount} 警告 · {failCount} 失败 · {skipCount} 跳过
        {/if}
      </span>
    </div>
    <div class="check-time">{report ? `${clockDate(report.finishedAt)} · 用时 ${formatDuration(report.durationMs)}` : "只读 · 不消耗模型额度"}</div>
  </div>

  <div class:stale={busy === "tests" && hasReport} class="check-body">
  <article class="panel domain-card">
    <div class="panel-head">
      <div><h2 class="panel-title">OpenAI 域名连通</h2><p class="panel-sub">经当前 Lane 真实访问。失败时检查节点、分流规则或目标服务状态。</p></div>
      <div class="right"><span class={`result-pill ${domainOK === domains.length && hasReport ? "pass" : hasReport ? "warn" : "pending"}`}>{domainOK} / {domains.length}</span></div>
    </div>
    <div class="domain-rows">
      {#each domains as item}
        <div class="domain-row">
          <i class:ok={item.status === "PASS"} class:bad={item.status === "FAIL"}></i>
          <code>{item.name.replace(" 连通", "")}</code>
          <span>{item.status === "PENDING" ? "尚未运行" : item.detail || item.hint}</span>
          <b>{item.latencyMs ? formatDuration(item.latencyMs) : "—"}</b>
          <em class={pillClass(item.status)}>{item.status === "PENDING" ? "待运行" : item.status}</em>
        </div>
      {/each}
    </div>
  </article>

  <article class="panel groups-card">
    <div class="panel-head"><div><h2 class="panel-title">分组检查</h2><p class="panel-sub">展开查看每一项，默认只看结论。</p></div><div class="right"><span class="mono-chip">{results.length || catalog.length} 项</span></div></div>
    {#each groups as group}
      {@const counts = groupCounts(group.key)}
      <details class="check-group" open={hasReport && counts.status !== "pass" ? true : undefined}>
        <summary>
          <span class="group-code">{group.code}</span>
          <b>{group.title}</b>
          <span class="group-meta">{hasReport ? `${counts.pass} / ${counts.total} 通过${counts.skip ? ` · ${counts.skip} 跳过` : ""}` : `${counts.total} 项待运行`}</span>
          <span class={`result-pill ${pillClass(counts.status)}`}>{counts.status.toUpperCase()}</span>
        </summary>
        <div class="group-body">
          {#each resultsFor(group.key) as item}
            <div class="test-row" title={item.hint || item.detail}>
              <span class="code">{item.id}</span>
              <span class="test-name"><b>{item.name}</b><span>{item.detail || item.hint || "等待运行"}</span></span>
              <span class="latency">{item.latencyMs ? formatDuration(item.latencyMs) : "—"}</span>
              <span class={`result-pill ${pillClass((item.status || "PENDING").toLowerCase())}`}>{item.status || "PENDING"}</span>
            </div>
          {/each}
        </div>
      </details>
    {/each}
  </article>

  <article class="panel env-card">
    <div class="panel-head"><div><h2 class="panel-title">环境</h2><p class="panel-sub">只读快照</p></div></div>
    <div class="env-grid">
      <div class="env-item"><span class="k">Codex</span><span class="v mono">{snapshot.codex.version || "未检测"}</span></div>
      <div class="env-item"><span class="k">内核</span><span class="v green">{snapshot.engine.resolved === "wsl" ? `WSL 2 · ${snapshot.wsl.distro}` : "Windows"}</span></div>
      <div class="env-item"><span class="k">上游</span><span class="v mono">{snapshot.settings.protocol.toUpperCase()} · {snapshot.settings.host}:{snapshot.settings.port}</span></div>
      <div class="env-item"><span class="k">Lane</span><span class="v mono">UI {snapshot.lane.interfacePort || "—"} / K {snapshot.lane.kernelPort || "—"}</span></div>
      <div class="env-item"><span class="k">主进程</span><span class="v mono">{snapshot.codex.running ? `PID ${snapshot.codex.mainPid}` : "未运行"}</span></div>
      <div class="env-item"><span class="k">系统代理</span><span class="v" class:green={!snapshot.environment.systemProxyOn}>{snapshot.environment.systemProxyOn ? "保持开启" : "保持关闭"}</span></div>
    </div>
  </article>
  </div>
</section>

<style>
  .check-body { display: flex; flex-direction: column; gap: 12px; }
  .check-body.stale { opacity: .5; pointer-events: none; }
  .banner-spin { display: inline-flex; color: var(--accent); }
  .check-banner {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-bottom: 12px;
    padding: 12px 16px;
    border: 1px solid var(--line);
    border-radius: var(--radius-lg);
    background: var(--surface);
  }
  .check-banner.ok { border-color: #d7ecdf; background: linear-gradient(90deg, #f6fdf8, #fbfefc); }
  .check-banner.warn { border-color: var(--orange-line); background: linear-gradient(90deg, #fff8eb, #fffdf9); }
  .check-score { flex: none; color: var(--text); font-size: 26px; font-weight: 500; letter-spacing: -.02em; }
  .check-score small { margin-left: 2px; color: var(--text-3); font-size: var(--fs-xs); font-weight: 600; }
  .check-copy { min-width: 0; flex: 1; }
  .check-copy b { display: block; font-size: 13px; font-weight: 500; }
  .check-copy span { display: block; margin-top: 2px; color: var(--text-3); font-size: var(--fs-xs); }
  .check-time { flex: none; color: var(--text-3); font: var(--fs-xs) var(--mono); }
  .domain-card { margin-bottom: 12px; }
  .domain-rows { border-top: 1px solid var(--line); }
  .domain-row { min-height: 38px; display: flex; align-items: center; gap: 10px; padding: 0 18px; border-bottom: 1px solid #f2f2f6; font-size: var(--fs-xs); }
  .domain-row i { width: 6px; height: 6px; flex: none; border-radius: 50%; background: #cbcbd6; }
  .domain-row i.ok { background: var(--green); }
  .domain-row i.bad { background: var(--red); }
  .domain-row code { width: 172px; flex: none; font: var(--fs-xs) var(--mono); color: var(--text); }
  .domain-row span { min-width: 0; flex: 1; overflow: hidden; color: var(--text-3); white-space: nowrap; text-overflow: ellipsis; }
  .domain-row b { flex: none; color: var(--text-2); font: var(--fs-xs) var(--mono); font-weight: 500; }
  .domain-row em { flex: none; min-height: 22px; display: inline-flex; align-items: center; padding: 0 8px; border: 1px solid var(--line); border-radius: var(--radius-sm); font-size: var(--fs-xs); font-style: normal; font-weight: 500; background: #f1f1f4; color: var(--text-3); }
  .domain-row em.pass { border-color: var(--green-line); background: var(--green-soft); color: var(--green); }
  .domain-row em.fail { border-color: #f1cccc; background: #fdeceb; color: var(--red); }
  .domain-row em.warn { border-color: var(--orange-line); background: var(--orange-soft); color: var(--orange); }
  .check-group { border-top: 1px solid #eeeef2; }
  .check-group summary {
    min-height: 42px;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 18px;
    cursor: pointer;
    list-style: none;
  }
  .check-group summary::-webkit-details-marker { display: none; }
  .check-group summary:hover { background: #fbfbfc; }
  .group-code { width: 18px; flex: none; color: var(--accent); font: var(--fs-xs) var(--mono); font-weight: 600; }
  .check-group summary b { flex: 1; font-size: 13px; font-weight: 610; }
  .group-meta { color: var(--text-3); font-size: var(--fs-xs); }
  .check-group[open] summary { border-bottom: 1px solid #f2f2f6; }
  .group-body { background: #fcfcfd; }
  .group-body .test-row { padding-left: 46px; }
</style>
