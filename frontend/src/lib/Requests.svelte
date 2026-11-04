<script lang="ts">
  import type { RequestRecord, Snapshot } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import { AppService } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/appservice";
  import Icon from "./Icon.svelte";
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
    onClear,
    onExport,
  }: {
    snapshot: Snapshot;
    busy?: string;
    onClear: () => Promise<void>;
    onExport: () => void;
  } = $props();

  let query = $state("");
  let filter = $state<"all" | "error">("all");
  let paused = $state(false);
  let frozen = $state<RequestRecord[]>([]);
  let selectedId = $state("");
  let harBusy = $state(false);
  let harCopied = $state(false);
  let commandCopied = $state(false);
  let actionError = $state("");

  function isAbnormal(request: RequestRecord): boolean {
    return Number(request.status) >= 400 || request.result === "failed" || request.result === "blocked" || Boolean(request.error);
  }

  $effect(() => {
    if (!paused) frozen = snapshot.requests ?? [];
  });

  const stats = $derived(snapshot.stats);
  const filtered = $derived(
    frozen.filter((request) => {
      const haystack = `${request.host} ${request.source} ${sourceLabel(request.source)} ${request.process} ${request.method} ${request.status} ${request.error}`.toLowerCase();
      const matchesQuery = !query.trim() || haystack.includes(query.trim().toLowerCase());
      const abnormal = isAbnormal(request);
      return matchesQuery && (filter === "all" || abnormal);
    }),
  );
  const selected = $derived(filtered.find((request) => request.id === selectedId) ?? filtered[0] ?? null);
  const errorCount = $derived(frozen.filter(isAbnormal).length);

  function statusText(request: RequestRecord): string {
    if (!request.status) return "连接中";
    if (request.status === 200 && request.method === "CONNECT") return "200";
    return String(request.status);
  }

  function statusPill(request: RequestRecord): string {
    if (!request.status && !isAbnormal(request)) return "run";
    if (request.result === "failed" || Number(request.status) >= 400) return "fail";
    if (request.result !== "ok") return "warn";
    return "pass";
  }

  function routeLabel(request: RequestRecord): string {
    if (request.result === "blocked") return "已拦截";
    if (isAbnormal(request)) return "连接失败";
    if (request.status < 200) return "连接中";
    return request.upstream === "DIRECT" ? "按规则直连" : "经上游代理";
  }

  function bytesLabel(request: RequestRecord): string {
    return formatBytes((request.bytesUp || 0) + (request.bytesDown || 0));
  }

  async function exportHAR() {
    harBusy = true;
    actionError = "";
    try {
      const har = await AppService.ExportHAR();
      await navigator.clipboard.writeText(har);
      harCopied = true;
      window.setTimeout(() => (harCopied = false), 2000);
    } catch (error) {
      actionError = `导出失败：${error instanceof Error ? error.message : String(error)}`;
    } finally {
      harBusy = false;
    }
  }

  function testCommand(request: RequestRecord): string {
    const port = request.source === "kernel" ? snapshot.lane.kernelPort : snapshot.lane.interfacePort;
    return `curl.exe -x http://127.0.0.1:${port} ${request.scheme || "https"}://${request.host}:${request.port}`;
  }

  async function copyTestCommand(request: RequestRecord) {
    actionError = "";
    try {
      await navigator.clipboard.writeText(testCommand(request));
      commandCopied = true;
      window.setTimeout(() => (commandCopied = false), 2000);
    } catch (error) {
      actionError = `复制失败：${error instanceof Error ? error.message : String(error)}`;
    }
  }
</script>

<section class="view active" id="view-requests" aria-labelledby="requests-title">
  <div class="page-head">
    <div class="page-head-copy">
      <h1 id="requests-title">请求日志</h1>
      <p class="page-sub">查看 Codex 的真实网络去向、代理结果与耗时；HTTPS 只记录连接元数据。</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={() => (paused = !paused)}>
        <Icon name={paused ? "play" : "pause"} size={15} />
        {paused ? "继续" : "暂停"}
      </button>
      <button class="btn" onclick={onExport} disabled={busy === "export"}>
        <Icon name="copy" size={15} />
        复制报告
      </button>
      <button class="btn" onclick={exportHAR} disabled={harBusy} title="导出 HAR 到剪贴板">
        {#if harBusy}<span class="spin"><Icon name="refresh" size={15} /></span>{:else}<Icon name="download" size={15} />{/if}
        {harCopied ? "已复制" : "导出 HAR"}
      </button>
      <button class="btn" onclick={async () => { await onClear(); frozen = snapshot.requests ?? []; }} disabled={busy === "clear" || !frozen.length}>
        <Icon name="x" size={15} />
        清空
      </button>
    </div>
  </div>

  {#if actionError}<p class="stat-error" role="alert">{actionError}</p>{/if}

  <div class="request-metrics">
    <article class="panel request-metric">
      <span class="metric-icon purple"><Icon name="route" size={17} /></span>
      <div class="metric-copy"><div class="metric-label">会话请求</div><div class="metric-value">{formatNumber(stats.total)}</div></div>
    </article>
    <article class="panel request-metric">
      <span class="metric-icon green"><Icon name="checkCircle" size={17} /></span>
      <div class="metric-copy"><div class="metric-label">成功率</div><div class="metric-value">{formatPercent(stats.successRate)}<small>%</small></div></div>
    </article>
    <article class="panel request-metric">
      <span class="metric-icon orange"><Icon name="clock" size={17} /></span>
      <div class="metric-copy"><div class="metric-label">P95 延迟</div><div class="metric-value">{stats.p95Ms || 0}<small>ms</small></div></div>
    </article>
    <article class="panel request-metric">
      <span class="metric-icon blue"><Icon name="download" size={17} /></span>
      <div class="metric-copy"><div class="metric-label">累计流量</div><div class="metric-value">{formatBytes(stats.bytesUp + stats.bytesDown)}</div></div>
    </article>
  </div>

  <div class="requests-workspace">
    <article class="panel request-table-card">
      <div class="request-toolbar">
        <label class="search">
          <Icon name="search" size={14} />
          <input bind:value={query} aria-label="搜索域名或进程" placeholder="搜索域名、来源或状态…" />
        </label>
        <button class="filter-btn" class:active={filter === "all"} onclick={() => (filter = "all")}>全部</button>
        <button class="filter-btn" class:active={filter === "error"} onclick={() => (filter = "error")}>异常 {errorCount}</button>
        <span class="live-chip"><i class="live-dot" style:background={paused ? "#92929c" : null}></i>{paused ? "PAUSED" : snapshot.lane.running ? "LIVE" : "OFFLINE"}</span>
      </div>
      <div class="request-table-wrap">
        <table aria-label="请求日志列表">
          <colgroup>
            <col class="col-time" />
            <col class="col-target" />
            <col class="col-source" />
            <col class="col-protocol" />
            <col class="col-result" />
            <col class="col-traffic" />
            <col class="col-duration" />
          </colgroup>
          <thead>
            <tr><th>时间</th><th>请求目标</th><th>来源</th><th>协议</th><th>结果</th><th>流量</th><th>耗时</th></tr>
          </thead>
          <tbody>
            {#each filtered as request}
              <tr class:selected={selected?.id === request.id} aria-selected={selected?.id === request.id} tabindex="0" onclick={() => (selectedId = request.id)} onkeydown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); selectedId = request.id; } }}>
                <td class="cell-time">{request.time}</td>
                <td class="cell-host" title={`${request.host}:${request.port}`}><b>{compactHost(request.host, 38)}:{request.port}</b><span>{request.method === "CONNECT" ? "TLS 隧道" : "HTTP"} · {routeLabel(request)}</span></td>
                <td><span class="source-cell"><i class:wsl={request.source === "kernel"} class:test={request.source === "self-test"}></i>{sourceLabel(request.source)}</span></td>
                <td><span class={`protocol ${request.method === "CONNECT" ? "tunnel" : "plain"}`}>{request.method}</span></td>
                <td><span class={`result-pill ${statusPill(request)}`}>{statusText(request)}</span></td>
                <td>{bytesLabel(request)}</td>
                <td>{formatDuration(request.durationMs)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if !filtered.length}
          <div class="empty-state">
            <Icon name="route" size={25} />
            <b>{frozen.length ? "没有匹配的请求" : "等待第一笔请求"}</b>
            <span>{frozen.length ? "调整搜索或筛选条件。" : "启动 Codex 或运行连通测试后，真实连接会实时出现在这里。"}</span>
          </div>
        {/if}
      </div>
      <div class="request-foot">
        <span>显示 <b>{filtered.length}</b> / {formatNumber(frozen.length)}</span><span>·</span><span>内存保留最近 {formatNumber(snapshot.settings.maxMemoryRows)} 条</span>
        <span class="spacer"></span>
        <span>{paused ? "已暂停刷新" : "实时更新"}</span>
      </div>
    </article>

    <aside class="panel request-detail">
      {#if selected}
        <div class="detail-head">
          <div class="detail-kicker">连接详情 · {routeLabel(selected)}</div>
          <h2>{compactHost(selected.host, 32)}:{selected.port}</h2>
          <p>{selected.id}</p>
        </div>
        <div class="detail-body">
          <div class="detail-section">
            <div class="section-label">请求概览</div>
            <div class="pv-grid">
              <div class="pv-item"><div class="k">时间</div><div class="v mono">{selected.time}</div></div>
              <div class="pv-item"><div class="k">结果</div><div class="v"><span class={`result-pill ${statusPill(selected)}`}>{statusText(selected)} · {selected.result || "pending"}</span></div></div>
              <div class="pv-item"><div class="k">来源</div><div class="v">{sourceLabel(selected.source)}</div></div>
              <div class="pv-item"><div class="k">协议</div><div class="v mono">{selected.scheme?.toUpperCase()} / {selected.method}</div></div>
              <div class="pv-item"><div class="k">上行</div><div class="v mono">{formatBytes(selected.bytesUp)}</div></div>
              <div class="pv-item"><div class="k">下行</div><div class="v mono">{formatBytes(selected.bytesDown)}</div></div>
            </div>
          </div>
          <div class="detail-section">
            <div class="section-label">代理链路</div>
            <div class="chain-box">
              <div class="chain-row"><i></i><span>{sourceLabel(selected.source)}</span><span class="right">{selected.process || "本机连接"}</span></div>
              <div class="chain-row"><i></i><span>Lane :{selected.source === "kernel" ? snapshot.lane.kernelPort : snapshot.lane.interfacePort}</span><span class="right">local</span></div>
              <div class="chain-row"><i></i><span>{selected.upstream || `${upstreamProtocol(snapshot.settings.protocol)} :${snapshot.settings.port}`}</span><span class="right">{formatDuration(selected.durationMs)}</span></div>
              <div class="chain-row"><i class:green={selected.result === "ok"}></i><span>{selected.host}</span><span class="right">{selected.result === "ok" ? "OK" : selected.error ? "ERROR" : selected.result}</span></div>
            </div>
            <div class="privacy-note" style="margin-top:10px">
              <b>连通性验证</b>
              <button class="btn small" style="margin-top:6px" onclick={() => void copyTestCommand(selected)}>
                {#if commandCopied}<Icon name="check" size={12} />已复制{:else}<Icon name="copy" size={12} />复制测试命令{/if}
              </button>
            </div>
          </div>
          <div class="detail-section">
            <div class="section-label">耗时</div>
            <div class="latency-bars">
              <div class="latency-row"><span>端到端</span><div class="latency-track"><span style="width:100%"></span></div><b>{formatDuration(selected.durationMs)}</b></div>
            </div>
            {#if selected.error}<div class="privacy-note" style="color:var(--red)"><b>连接错误</b><br>{selected.error}</div>{/if}
          </div>
          <div class="privacy-note"><b>隐私边界</b><br>Lane 不启用 MITM，不读取 HTTPS 路径、Header、Cookie 或请求体。这里只展示域名、端口、状态、流量和耗时。</div>
        </div>
      {:else}
        <div class="empty-state" style="height:100%">
          <Icon name="search" size={25} />
          <b>选择一条请求</b>
          <span>右侧会显示代理链路、状态、流量与耗时。</span>
        </div>
      {/if}
    </aside>
  </div>
</section>

<style>
  .col-time { width: 112px; }
  .col-target { width: auto; }
  .col-source { width: 132px; }
  .col-protocol { width: 92px; }
  .col-result { width: 82px; }
  .col-traffic { width: 86px; }
  .col-duration { width: 76px; }
  .request-table-wrap { overflow: auto; }
  .request-table-wrap thead { position: sticky; top: 0; z-index: 2; }
  .request-foot { min-height: 38px; display: flex; align-items: center; gap: 6px; padding: 0 13px; border-top: 1px solid var(--line); color: var(--text-3); font-size: var(--fs-xs); background: #fbfbfc; }
  .request-foot .spacer { flex: 1; }
  .request-foot b { color: var(--text-2); }
</style>
