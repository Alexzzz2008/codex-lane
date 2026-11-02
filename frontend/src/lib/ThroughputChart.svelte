<script lang="ts">
  import type { RequestSeriesPoint } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import { formatBytes } from "./format";

  let { series = [] }: { series?: RequestSeriesPoint[] | null } = $props();

  const width = 760;
  const left = 48;
  const right = 710;
  const top = 28;
  const bottom = 170;

  let stage = $state<HTMLDivElement | null>(null);
  let hoverIndex = $state<number | null>(null);

  function smoothPath(points: Array<{ x: number; y: number }>): string {
    if (!points.length) return "";
    if (points.length === 1) return `M${points[0].x} ${points[0].y}`;
    let path = `M${points[0].x.toFixed(1)} ${points[0].y.toFixed(1)}`;
    for (let i = 0; i < points.length - 1; i++) {
      const p0 = points[Math.max(0, i - 1)];
      const p1 = points[i];
      const p2 = points[i + 1];
      const p3 = points[Math.min(points.length - 1, i + 2)];
      const c1x = p1.x + (p2.x - p0.x) / 6;
      const c1y = p1.y + (p2.y - p0.y) / 6;
      const c2x = p2.x - (p3.x - p1.x) / 6;
      const c2y = p2.y - (p3.y - p1.y) / 6;
      path += ` C${c1x.toFixed(1)} ${c1y.toFixed(1)} ${c2x.toFixed(1)} ${c2y.toFixed(1)} ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`;
    }
    return path;
  }

  const values = $derived((series ?? []).filter((point) => point && point.time));
  const hasTraffic = $derived(values.some((point) => (point.bytesDown || 0) > 0 || (point.latencyMs || 0) > 0));
  const maxDown = $derived(Math.max(1, ...values.map((point) => point.bytesDown || 0)));
  const maxLatency = $derived(Math.max(1, ...values.map((point) => point.latencyMs || 0)));
  const downPoints = $derived(
    values.map((point, index) => ({
      x: left + (index * (right - left)) / Math.max(1, values.length - 1),
      y: bottom - ((point.bytesDown || 0) / maxDown) * (bottom - top),
    })),
  );
  const latencyPoints = $derived(
    values.map((point, index) => ({
      x: left + (index * (right - left)) / Math.max(1, values.length - 1),
      y: bottom - ((point.latencyMs || 0) / maxLatency) * (bottom - top),
    })),
  );
  const linePath = $derived(smoothPath(downPoints));
  const latencyPath = $derived(smoothPath(latencyPoints));
  const areaPath = $derived(
    linePath ? `${linePath} L${downPoints.at(-1)?.x ?? right} ${bottom} L${downPoints[0]?.x ?? left} ${bottom} Z` : "",
  );
  const maxLabel = $derived(maxDown > 1 ? formatBytes(maxDown, 0) : "0 B");
  const midLabel = $derived(formatBytes(maxDown > 1 ? maxDown / 2 : 0, 0));
  const maxLatencyLabel = $derived(maxLatency > 1 ? `${Math.round(maxLatency)} ms` : "0 ms");
  const midLatencyLabel = $derived(maxLatency > 1 ? `${Math.round(maxLatency / 2)} ms` : "0 ms");
  const hovered = $derived(hoverIndex !== null ? values[hoverIndex] : null);
  const hoverPoint = $derived(hoverIndex !== null ? downPoints[hoverIndex] : null);
  const hoverLatency = $derived(hoverIndex !== null ? latencyPoints[hoverIndex] : null);
  const hoverPercent = $derived(hoverPoint ? (hoverPoint.x / width) * 100 : 50);
  const tipPercent = $derived(Math.max(17, Math.min(83, hoverPercent)));
  const firstLabel = $derived(values[0]?.time ?? "");
  const middleLabel = $derived(values[Math.floor(values.length / 2)]?.time ?? "");
  const lastLabel = $derived(values.at(-1)?.time ?? "");

  function moveReadout(event: PointerEvent) {
    if (!stage || values.length === 0) return;
    const rect = stage.getBoundingClientRect();
    if (!rect.width) return;
    const x = ((event.clientX - rect.left) / rect.width) * width;
    const ratio = Math.max(0, Math.min(1, (x - left) / (right - left)));
    hoverIndex = Math.round(ratio * Math.max(0, values.length - 1));
  }

  function clearReadout() {
    hoverIndex = null;
  }
</script>

<div class="chart-wrap">
  <div class="legend">
    <span><i></i>下行流量</span>
    <span><i class="green"></i>连接延迟</span>
    <span class="hint">悬停查看读数</span>
  </div>
  <div class="chart-stage" role="group" aria-label="流量与延迟图表，移动指针查看精确读数" bind:this={stage} onpointermove={moveReadout} onpointerleave={clearReadout}>
    <svg class="chart-svg" viewBox="0 0 760 205" preserveAspectRatio="none" role="img" aria-label="最近请求吞吐和延迟趋势">
      <defs>
        <linearGradient id="areaFill" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stop-color="#5648d8" stop-opacity=".2" />
          <stop offset=".68" stop-color="#5648d8" stop-opacity=".045" />
          <stop offset="1" stop-color="#5648d8" stop-opacity="0" />
        </linearGradient>
      </defs>
      <g stroke="#ededf2" stroke-width="1">
        <line x1={left} y1={top} x2={right} y2={top} />
        <line x1={left} y1="99" x2={right} y2="99" />
        <line x1={left} y1={bottom} x2={right} y2={bottom} />
        <line x1={left} y1={top} x2={left} y2={bottom} stroke="#f2f2f6" />
        <line x1={right} y1={top} x2={right} y2={bottom} stroke="#f2f2f6" />
      </g>
      {#if hasTraffic && areaPath}
        <path d={areaPath} fill="url(#areaFill)" />
        <path d={linePath} fill="none" stroke="#5648d8" stroke-width="2.2" stroke-linecap="round" />
        <path d={latencyPath} fill="none" stroke="#148b52" stroke-width="1.5" stroke-dasharray="4 4" stroke-linecap="round" />
        {#if hoverPoint && hoverLatency}
          <line x1={hoverPoint.x} y1={top} x2={hoverPoint.x} y2={bottom} stroke="#aaa3e8" stroke-width="1" stroke-dasharray="3 3" />
          <circle cx={hoverPoint.x} cy={hoverPoint.y} r="4.5" fill="white" stroke="#5648d8" stroke-width="2.2" />
          <circle cx={hoverLatency.x} cy={hoverLatency.y} r="3.5" fill="white" stroke="#148b52" stroke-width="1.8" />
        {:else if downPoints.length}
          <circle cx={downPoints.at(-1)?.x} cy={downPoints.at(-1)?.y} r="4" fill="white" stroke="#5648d8" stroke-width="2" />
        {/if}
      {:else}
        <line x1={left} y1="160" x2={right} y2="160" stroke="#d9d9e0" stroke-width="1.2" stroke-dasharray="4 5" />
      {/if}
    </svg>

    <div class="axis-y left" aria-hidden="true">
      <span style:top={`${(top / 205) * 100}%`}>{maxLabel}</span>
      <span style:top={`${(99 / 205) * 100}%`}>{midLabel}</span>
      <span style:top={`${(bottom / 205) * 100}%`}>0 B</span>
    </div>
    <div class="axis-y right" aria-hidden="true">
      <span style:top={`${(top / 205) * 100}%`}>{maxLatencyLabel}</span>
      <span style:top={`${(99 / 205) * 100}%`}>{midLatencyLabel}</span>
      <span style:top={`${(bottom / 205) * 100}%`}>0 ms</span>
    </div>
    <div class="axis-x" aria-hidden="true">
      <span>{firstLabel || "等待流量"}</span>
      <span>{middleLabel}</span>
      <span>{lastLabel}</span>
    </div>

    {#if hovered}
      <div class="chart-tip" style:left={`${tipPercent}%`}>
        <b>{hovered.time}</b>
        <span><i class="purple"></i>下行<strong>{formatBytes(hovered.bytesDown || 0, 1)}</strong></span>
        <span><i class="green"></i>延迟<strong>{hovered.latencyMs ? `${Math.round(hovered.latencyMs)} ms` : "—"}</strong></span>
      </div>
    {:else if !hasTraffic}
      <div class="chart-empty">还没有可绘制的请求流量</div>
    {/if}
  </div>
</div>

<style>
  .chart-wrap {
    min-height: 0;
    flex: 1;
    padding: 0 16px 10px;
    display: flex;
    flex-direction: column;
  }
  .legend {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 0 3px 7px;
    color: var(--text-3);
    font-size: 11.5px;
  }
  .legend span { display: inline-flex; align-items: center; gap: 6px; }
  .legend .hint { margin-left: auto; color: var(--text-4); }
  .legend i { width: 8px; height: 8px; border-radius: 2px; background: var(--accent); }
  .legend i.green { width: 14px; height: 2px; border-radius: 2px; background: var(--green); }
  .chart-stage {
    position: relative;
    min-height: 158px;
    flex: 1;
    cursor: crosshair;
    touch-action: none;
  }
  .chart-svg {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    overflow: visible;
  }
  .axis-y,
  .axis-x {
    position: absolute;
    pointer-events: none;
    color: var(--text-3);
    font: 11px var(--sans);
    font-variant-numeric: tabular-nums;
  }
  .axis-y { inset: 0; }
  .axis-y span {
    position: absolute;
    left: 0;
    width: 40px;
    transform: translateY(-50%);
    text-align: right;
    white-space: nowrap;
  }
  .axis-y.right span {
    right: 0;
    left: auto;
    width: 46px;
    text-align: left;
  }
  .axis-x {
    right: 6.6%;
    bottom: 0;
    left: 6.3%;
    display: flex;
    justify-content: space-between;
    font-size: 11px;
  }
  .chart-tip {
    min-width: 122px;
    position: absolute;
    top: 2px;
    z-index: 3;
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border: 1px solid rgba(36, 36, 48, .12);
    border-radius: 7px;
    background: rgba(32, 33, 36, .96);
    box-shadow: 0 7px 20px rgba(24, 24, 35, .18);
    color: #f7f7fa;
    font-size: 11.5px;
    pointer-events: none;
    transform: translateX(-50%);
    white-space: nowrap;
  }
  .chart-tip b {
    color: #c9c4ff;
    font-size: 11px;
    font-weight: 650;
    letter-spacing: .02em;
  }
  .chart-tip span { display: flex; align-items: center; gap: 6px; color: #c8c8d1; }
  .chart-tip strong { margin-left: auto; color: white; font-weight: 650; font-variant-numeric: tabular-nums; }
  .chart-tip i { width: 6px; height: 6px; border-radius: 50%; background: #8d82f2; }
  .chart-tip i.green { background: #43c88b; }
  .chart-empty {
    position: absolute;
    inset: 28px 6% 12% 6%;
    display: grid;
    place-items: center;
    color: var(--text-3);
    font-size: 12px;
    pointer-events: none;
  }
</style>
