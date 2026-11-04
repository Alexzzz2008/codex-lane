export function formatBytes(bytes: number, digits = 1): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  const precision = unit === 0 ? 0 : digits;
  return `${value.toFixed(precision)} ${units[unit]}`;
}

export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "—";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)} s`;
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value || 0);
}

export function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return "0.0";
  return value.toFixed(1);
}

export function shortTime(iso: string): string {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso.slice(0, 8);
  return date.toLocaleTimeString("zh-CN", { hour12: false });
}

export function clockDate(iso: string): string {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return `${date.toLocaleTimeString("zh-CN", { hour12: false })} · ${date.toLocaleDateString("zh-CN")}`;
}

export function engineLabel(engine: string): string {
  if (engine === "wsl") return "WSL";
  if (engine === "windows") return "Windows";
  return "自动";
}

export function sourceLabel(source: string): string {
  if (source === "kernel") return "内核入口";
  if (source === "interface") return "界面入口";
  if (source === "self-test") return "诊断";
  return source || "未知";
}

export function upstreamProtocol(protocol: string): string {
  return (protocol || "http").toUpperCase();
}

export function compactHost(host: string, max = 28): string {
  if (!host || host.length <= max) return host || "—";
  return `${host.slice(0, max - 1)}…`;
}

export function median(values: number[]): number {
  if (!values.length) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  if (sorted.length % 2) return sorted[middle];
  return Math.round((sorted[middle - 1] + sorted[middle]) / 2);
}

export function clampPercent(value: number): number {
  return Math.max(0, Math.min(100, value || 0));
}

