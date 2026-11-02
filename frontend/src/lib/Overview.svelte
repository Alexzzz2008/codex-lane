<script lang="ts">
  import type { Snapshot } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import Icon from "./Icon.svelte";
  import ThroughputChart from "./ThroughputChart.svelte";
  import {
    compactHost,
    formatBytes,
    formatDuration,
    formatNumber,
    formatPercent,
    sourceLabel,
    upstreamProtocol,
  } from "./format";

  let {
    snapshot,
    busy = "",
    onLaunch,
    onLaneToggle,
    onNavigate,
    onDiagnostics,
    onRunTests,
  }: {
    snapshot: Snapshot;
    busy?: string;
    onLaunch: () => void;
    onLaneToggle: () => void;
    onNavigate: (page: string) => void;
    onDiagnostics: () => void;
    onRunTests: () => void;
  } = $props();

  const lane = $derived(snapshot.lane);
  const stats = $derived(snapshot.stats);
  const codex = $derived(snapshot.codex);
  const upstream = $derived(snapshot.upstream);
  const engine = $derived(snapshot.engine?.resolved === "wsl" ? "WSL" : "Windows");
  const chip = $derived(
    snapshot.engine?.resolved === "wsl"
      ? `WSL · ${snapshot.wsl?.networkMode || "待检测"}`
      : `Windows · ${snapshot.environment?.arch || "amd64"}`,
  );
  const codexConnected = $derived(codex.running && codex.proxyMatched);
  const recent = $derived((snapshot.requests ?? []).slice(0, 5));
  const lastCheck = $derived(
    upstream.checkedAt
      ? new Date(upstream.checkedAt).toLocaleTimeString("zh-CN", { hour12: false })
      : "尚未检测",
  );
  let rangeMinutes = $state(30);
  const ranges = [
    { value: 5, label: "5 分钟" },
    { value: 30, label: "30 分钟" },
    { value: 60, label: "60 分钟" },
  ];
  const visibleSeries = $derived((snapshot.series ?? []).slice(-rangeMinutes));
  const routeTitle = $derived(
    lane.running ? "Codex Lane 正在运行" : lane.error ? "Codex Lane 启动异常" : "Codex Lane 已停止",
  );
  const routeSub = $derived(
    lane.running
      ? codexConnected
        ? `已接管 Codex Desktop 与${engine}内核`
        : codex.running
          ? `Codex 正通过 ${codex.proxyUrl ? compactProxy(codex.proxyUrl) : "其他入口"} 运行，尚未接入 Lane`
          : "网关已就绪，等待启动 Codex"
      : "开启通道后才能将 Codex 的网络请求导入本地网关",
  );

  function compactProxy(url: string): string {
    return url.replace(/^https?:\/\//i, "").replace(/^socks5h?:\/\//i, "");
  }
</script>

<section class="view active" id="view-overview" aria-labelledby="overview-title">
  <div class="page-head">
    <div class="page-head-copy">
      <h1 id="overview-title">运行总览</h1>
      <p class="page-sub">只看 Codex 的独立通道；Windows 系统代理与 TUN 始终保持原样。</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={onDiagnostics} disabled={busy === "diagnostics" || busy === "tests"}>
        {#if busy === "diagnostics" || busy === "tests"}
          <span class="spin"><Icon name="refresh" size={15} /></span>
        {:else}
          <Icon name="activity" size={15} />
        {/if}
        运行检查
      </button>
      <button class="btn dark" onclick={onLaunch} disabled={busy === "launch" || busy === "restart"}>
        {#if busy === "launch" || busy === "restart"}
          <span class="spin"><Icon name="refresh" size={15} /></span>
        {:else}
          <Icon name="play" size={15} />
        {/if}
        {codexConnected ? "重新打开 Codex" : codex.running ? "接入 Codex" : "打开 Codex"}
      </button>
    </div>
  </div>

  <article class="panel route-card">
    <div class="route-top">
      <span class="lane-badge"><Icon name="lane" size={17} /></span>
      <div class="route-heading">
        <b>{routeTitle}</b>
        <span>{routeSub}</span>
      </div>
      <div class="right">
        <span class="mono-chip"><Icon name="globe" size={12} /> {chip}</span>
        <button
          class:off={!lane.running}
          class="switch"
          aria-label={lane.running ? "停止通道" : "开启通道"}
          title={lane.running ? "停止 Lane" : "开启 Lane"}
          onclick={onLaneToggle}
        ></button>
      </div>
    </div>

    <div class="route-map">
      <div class="route-node origin">
        <span class="node-icon"><Icon name="monitor" size={19} /></span>
        <span class="node-copy">
          <small>01 · 客户端</small>
          <b>Codex Desktop</b>
          <span>{codex.running ? `${codex.processes?.find((p) => p.isMain)?.name || "ChatGPT.exe"} · PID ${codex.mainPid}` : "等待启动"}</span>
        </span>
      </div>
      <div class:live={lane.running} class="route-link"><span>PROCESS</span></div>
      <div class="route-node lane">
        <span class="node-icon"><Icon name="lane" size={19} /></span>
        <span class="node-copy">
          <small>02 · 本地入口</small>
          <b>Codex Lane</b>
          <span>{lane.running ? `UI :${lane.interfacePort} · 内核 :${lane.kernelPort}` : "127.0.0.1:—"}</span>
        </span>
      </div>
      <div class:live={upstream.reachable} class="route-link"><span>HTTP</span></div>
      <div class="route-node">
        <span class="node-icon"><Icon name="shield" size={19} /></span>
        <span class="node-copy">
          <small>03 · 上游代理</small>
          <b>{upstream.reachable ? "上游可达" : "上游待连接"}</b>
          <span>{upstreamProtocol(snapshot.settings.protocol)} · {snapshot.settings.host}:{snapshot.settings.port} · {formatDuration(upstream.latencyMs)}</span>
        </span>
      </div>
      <div class:live={upstream.reachable} class="route-link"><span>INTERNET</span></div>
      <div class="route-node destination">
        <span class="node-icon"><Icon name="globe" size={19} /></span>
        <span class="node-copy">
          <small>04 · 目标服务</small>
          <b>OpenAI</b>
          <span>{upstream.reachable ? "CONNECT 已验证" : "等待上游探测"}</span>
        </span>
      </div>
    </div>

    <div class="route-foot">
      <span class="mini-flag" class:safe={lane.running}>{#if lane.running}<Icon name="check" size={12} />已开启{:else}<Icon name="minus" size={12} />未开启{/if}</span>
      <span class="mini-flag off"><Icon name="minus" size={12} />系统代理不修改</span>
      <span class="mini-flag off"><Icon name="minus" size={12} />TUN 不触碰</span>
      <span class="updated">上游检测 <b>{lastCheck}</b></span>
    </div>
  </article>

  <div class="metric-grid">
    <article class="panel metric-card">
      <span class="metric-icon purple"><Icon name="route" size={18} /></span>
      <div class="metric-copy">
        <div class="metric-label purple">本次会话请求</div>
        <div class="metric-value">{formatNumber(stats.total)}</div>
        <div class="metric-foot"><b>{formatNumber(stats.success)}</b> 成功 · {formatNumber(stats.failed)} 异常</div>
      </div>
    </article>
    <article class="panel metric-card">
      <span class="metric-icon green"><Icon name="download" size={18} /></span>
      <div class="metric-copy">
        <div class="metric-label green">累计传输</div>
        <div class="metric-value">{formatBytes(stats.bytesUp + stats.bytesDown, 1)}</div>
        <div class="metric-foot">上行 {formatBytes(stats.bytesUp)} · 下行 {formatBytes(stats.bytesDown)}</div>
      </div>
    </article>
    <article class="panel metric-card">
      <span class="metric-icon orange"><Icon name="clock" size={18} /></span>
      <div class="metric-copy">
        <div class="metric-label orange">请求延迟</div>
        <div class="metric-value">{stats.medianMs || 0}<small>ms</small></div>
        <div class="metric-foot">P95 {formatDuration(stats.p95Ms)}</div>
      </div>
    </article>
    <article class="panel metric-card">
      <span class="metric-icon blue"><Icon name="checkCircle" size={18} /></span>
      <div class="metric-copy">
        <div class="metric-label blue">连接成功率</div>
        <div class="metric-value">{formatPercent(stats.successRate)}<small>%</small></div>
        <div class="metric-foot">{formatNumber(stats.success)} / {formatNumber(stats.total || 0)} 次连接</div>
      </div>
    </article>
  </div>

  <div class="overview-lower">
    <article class="panel chart-card">
      <div class="panel-head">
        <div>
          <h2 class="panel-title">吞吐与延迟</h2>
          <p class="panel-sub">实时会话 · 仅记录元数据</p>
        </div>
        <div class="chart-numbers">
          <div class="chart-stat"><div class="k">下行</div><div class="v">{formatBytes(stats.bytesDown)}</div></div>
          <div class="chart-stat"><div class="k">P95</div><div class="v">{formatDuration(stats.p95Ms)}</div></div>
        </div>
        <div class="segmented" role="group" aria-label="图表时间范围">
          {#each ranges as range}
            <button class:active={rangeMinutes === range.value} onclick={() => (rangeMinutes = range.value)}>{range.label}</button>
          {/each}
        </div>
      </div>
      <ThroughputChart series={visibleSeries} />
      <div class="chart-footer">
        <span>最近 {rangeMinutes} 分钟</span><span>·</span><span>按分钟聚合</span><span>·</span><span>仅域名、状态、耗时与流量</span>
        <span class="spacer"></span><b>无 MITM</b><span>·</span><span>不读取 HTTPS 内容</span>
      </div>
    </article>

    <article class="panel recent-card">
      <div class="panel-head">
        <div>
          <h2 class="panel-title">最近请求</h2>
          <p class="panel-sub">实时元数据 · 不包含路径与内容</p>
        </div>
        <div class="right"><span class="live-chip"><i class="live-dot"></i>{lane.running ? "实时" : "离线"}</span></div>
      </div>
      <div class="recent-list">
        {#if recent.length}
          {#each recent as request}
            <button class="recent-row recent-row-button" onclick={() => onNavigate("requests")}>
              <span class="recent-time">{request.time.slice(0, 5)}</span>
              <span class="recent-host"><b>{compactHost(request.host, 24)}</b><span>{sourceLabel(request.source)} · {request.method}</span></span>
              <span class="recent-latency">{formatDuration(request.durationMs)}</span>
              <span class="recent-status">
                <span class={`result-pill ${request.result === "ok" ? "pass" : request.result === "failed" ? "fail" : "pending"}`}>
                  {request.result === "ok" ? "正常" : request.result === "failed" ? "失败" : "进行中"}
                </span>
              </span>
            </button>
          {/each}
        {:else}
          <div class="empty-state"><Icon name="activity" size={25} /><b>等待 Codex 请求</b><span>打开 Codex 或运行连通测试后，这里会出现真实连接。</span></div>
        {/if}
      </div>
      <button class="recent-foot" onclick={() => onNavigate("requests")}>查看全部 {formatNumber(stats.total)} 条请求 <span style="margin-left:5px">→</span></button>
    </article>
  </div>
</section>

<style>
  .recent-row-button {
    width: 100%;
    padding: 0;
    border-left: 0;
    border-right: 0;
    border-bottom: 0;
    background: transparent;
    text-align: left;
  }
  .recent-row-button:hover { background: #fafafd; }
  .recent-foot { width: 100%; border-left: 0; border-right: 0; border-bottom: 0; }
</style>
