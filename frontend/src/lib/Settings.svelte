<script lang="ts">
  import { onMount } from "svelte";
  import type { Dependency, SettingsInput, Snapshot, UpdateStatus, UpstreamProbeResult, UpstreamStatus } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import { AppService } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/appservice";
  import Icon from "./Icon.svelte";
  import { clockDate, clampPercent, formatBytes, formatDuration, upstreamProtocol } from "./format";

  let {
    snapshot,
    busy = "",
    onSave,
    onProbe,
    onCheckUpdate,
    onDownloadUpdate,
    onRestartUpdate,
    onOpenProject,
    onOpenConfigFolder,
    onSwitchUpstream,
    onSelectFastest,
  }: {
    snapshot: Snapshot;
    busy?: string;
    onSave: (input: SettingsInput) => Promise<boolean>;
    onProbe: (input: SettingsInput) => Promise<UpstreamStatus | null>;
    onCheckUpdate: (silent?: boolean) => Promise<UpdateStatus | null>;
    onDownloadUpdate: () => Promise<void>;
    onRestartUpdate: () => Promise<void>;
    onOpenProject: (target: "repository" | "releases") => Promise<void>;
    onOpenConfigFolder: () => Promise<void>;
    onSwitchUpstream: (name: string) => Promise<void>;
    onSelectFastest: () => Promise<boolean>;
  } = $props();

  const defaultDraft: SettingsInput = {
    protocol: "http",
    host: "127.0.0.1",
    port: 7897,
    noProxy: "localhost,127.0.0.1,::1",
    engine: "auto",
    wslDistro: "",
    openCodex: false,
    stopCodexOnExit: false,
    retentionDays: 14,
    maxMemoryRows: 10000,
    upstreams: [],
    activeUpstream: "",
    autoFailover: false,
    hostFilterMode: "off",
    allowedHosts: [],
    routes: [],
    minimizeToTray: true,
    launchAtLogin: false,
  };

  function fromSnapshot(value: Snapshot): SettingsInput {
    return {
      protocol: value.settings.protocol,
      host: value.settings.host,
      port: value.settings.port,
      noProxy: value.settings.noProxy,
      engine: value.settings.engine,
      wslDistro: value.settings.wslDistro,
      openCodex: value.settings.openCodex,
      stopCodexOnExit: value.settings.stopCodexOnExit,
      retentionDays: value.settings.retentionDays,
      maxMemoryRows: value.settings.maxMemoryRows,
      upstreams: value.settings.upstreams ?? [],
      activeUpstream: value.settings.activeUpstream,
      autoFailover: value.settings.autoFailover,
      hostFilterMode: value.settings.hostFilterMode,
      allowedHosts: value.settings.allowedHosts ?? [],
      routes: value.settings.routes ?? [],
      minimizeToTray: value.settings.minimizeToTray,
      launchAtLogin: value.settings.launchAtLogin,
    };
  }

  let draft = $state<SettingsInput>({ ...defaultDraft });
  let baseline = $state("");
  let dirty = $state(false);
  let lastIncoming = $state("");
  let probe = $state<UpstreamStatus | null>(null);
  let copied = $state(false);
  let autosaving = $state(false);
  let pendingSave = false;
  let saveTimer: number | undefined;
  let installing = $state("");
  let installNote = $state("");
  let newProfileName = $state("");
  let newProfileProtocol = $state("http");
  let newProfileHost = $state("");
  let newProfilePort = $state(7897);
  let newRoutePattern = $state("");
  let newRouteAction = $state("proxy");
  let newHost = $state("");
  let profileError = $state("");
  let routeError = $state("");
  let hostError = $state("");
  let speedResults = $state<UpstreamProbeResult[] | null>(null);
  let speedBusy = $state(false);

  const speedByName = $derived(new Map((speedResults ?? []).map((item) => [item.name, item])));
  const fastestName = $derived.by(() => {
    let best: string | null = null;
    let bestLatency = Number.POSITIVE_INFINITY;
    for (const item of speedResults ?? []) {
      if (item.reachable && item.latencyMs < bestLatency) {
        best = item.name;
        bestLatency = item.latencyMs;
      }
    }
    return best;
  });

  const ruleGroup = "🤖AI网站";
  const requiredDomains: Array<{ domain: string; purpose: string; optional?: boolean }> = [
    { domain: "openai.com", purpose: "API、登录鉴权、平台接口" },
    { domain: "chatgpt.com", purpose: "桌面端主站与会话后端" },
    { domain: "oaistatic.com", purpose: "前端静态资源与脚本" },
    { domain: "oaiusercontent.com", purpose: "文件、图片与附件上传下载" },
    { domain: "livekit.cloud", purpose: "语音模式实时通道", optional: true },
    { domain: "sora.com", purpose: "Sora 页面与素材", optional: true },
  ];
  const ruleSnippet = requiredDomains
    .map((item) => `- DOMAIN-SUFFIX,${item.domain},${ruleGroup}`)
    .join("\n");

  async function copyRules() {
    try {
      await navigator.clipboard.writeText(ruleSnippet);
      copied = true;
      window.setTimeout(() => (copied = false), 2000);
    } catch {
      copied = false;
    }
  }

  $effect(() => {
    const incoming = JSON.stringify(fromSnapshot(snapshot));
    if (incoming !== lastIncoming) {
      lastIncoming = incoming;
      if (!dirty) {
        draft = JSON.parse(incoming) as SettingsInput;
        baseline = incoming;
      }
    }
  });

  const keys: Array<keyof SettingsInput> = [
    "protocol",
    "host",
    "port",
    "noProxy",
    "engine",
    "wslDistro",
    "openCodex",
    "stopCodexOnExit",
    "retentionDays",
    "maxMemoryRows",
    "upstreams",
    "activeUpstream",
    "autoFailover",
    "hostFilterMode",
    "allowedHosts",
    "routes",
    "minimizeToTray",
    "launchAtLogin",
  ];
  const changeCount = $derived(
    (() => {
      if (!baseline) return 0;
      const original = JSON.parse(baseline) as SettingsInput;
      return keys.filter((key) => JSON.stringify(original[key]) !== JSON.stringify(draft[key])).length;
    })(),
  );
  const distros = $derived(snapshot.wsl?.distros ?? []);
  const profiles = $derived(draft.upstreams ?? []);
  const routes = $derived(draft.routes ?? []);
  const allowedHosts = $derived(draft.allowedHosts ?? []);
  let deps = $state<Dependency[]>([]);
  let depsLoading = $state(false);
  const missingDeps = $derived(deps.filter((item) => !item.ready).length);

  async function loadDependencies() {
    depsLoading = true;
    try {
      deps = (await AppService.GetDependencies()) ?? [];
    } catch {
      deps = [];
    } finally {
      depsLoading = false;
    }
  }

  onMount(() => {
    newProfileProtocol = snapshot.settings.protocol || "http";
    newProfileHost = snapshot.settings.host || "127.0.0.1";
    newProfilePort = snapshot.settings.port || 7897;
    void loadDependencies();
  });

  async function runSpeedTest() {
    speedBusy = true;
    profileError = "";
    try {
      speedResults = (await AppService.ProbeAllUpstreams()) ?? [];
      if (speedResults.length === 0) profileError = "还没有档案可测速";
    } catch (error) {
      profileError = error instanceof Error ? error.message : String(error);
    } finally {
      speedBusy = false;
    }
  }

  async function installDependency(dep: Dependency) {
    installNote = "";
    if (!dep.action) {
      installNote = dep.hint || "请手动处理后重新检查";
      return;
    }
    installing = dep.id;
    try {
      installNote = await AppService.InstallDependency(dep.id);
      await loadDependencies();
    } catch (error) {
      installNote = error instanceof Error ? error.message : String(error);
    } finally {
      installing = "";
    }
  }

  function touch() {
    dirty = true;
  }

  function updateTopField(field: "protocol" | "host" | "port", value: string | number) {
    if (field === "protocol") draft.protocol = String(value);
    if (field === "host") draft.host = String(value);
    if (field === "port") draft.port = Number(value);
    const active = draft.activeUpstream;
    if (active) {
      draft.upstreams = profiles.map((profile) =>
        profile.name === active
          ? { ...profile, protocol: draft.protocol, host: draft.host, port: draft.port }
          : profile,
      );
    }
    touch();
  }

  function updateProfile(
    index: number,
    patch: Partial<{ name: string; protocol: string; host: string; port: number }>,
  ) {
    const current = profiles[index];
    const active = current?.name === draft.activeUpstream;
    draft.upstreams = profiles.map((profile, itemIndex) =>
      itemIndex === index ? { ...profile, ...patch } : profile,
    );
    if (!active) return touch();
    if (patch.name !== undefined) draft.activeUpstream = patch.name;
    if (patch.protocol !== undefined) draft.protocol = patch.protocol;
    if (patch.host !== undefined) draft.host = patch.host;
    if (patch.port !== undefined) draft.port = patch.port;
    touch();
  }

  function scheduleAutoSave() {
    dirty = true;
    if (saveTimer) window.clearTimeout(saveTimer);
    saveTimer = window.setTimeout(() => {
      void autoSave();
    }, 250);
  }

  async function autoSave() {
    if (autosaving) {
      pendingSave = true;
      return;
    }
    autosaving = true;
    const payload = prunedDraft();
    const success = await onSave(payload);
    autosaving = false;
    if (success) {
      dirty = false;
      baseline = JSON.stringify(payload);
      lastIncoming = baseline;
    }
    if (pendingSave) {
      pendingSave = false;
      void autoSave();
    }
  }

  async function save() {
    const payload = prunedDraft();
    const success = await onSave(payload);
    if (success) {
      dirty = false;
      baseline = JSON.stringify(payload);
      lastIncoming = baseline;
    }
  }

  function prunedDraft(): SettingsInput {
    return {
      ...draft,
      upstreams: profiles.filter((item) => item.name.trim() && item.host.trim() && item.port >= 1 && item.port <= 65535),
      routes: routes.filter((item) => item.pattern.trim()),
      allowedHosts: allowedHosts.filter((item) => item.trim()),
    };
  }

  async function testUpstream() {
    probe = null;
    probe = await onProbe(prunedDraft());
  }

  function cancel() {
    draft = JSON.parse(baseline) as SettingsInput;
    dirty = false;
  }

  function addProfile() {
    const name = newProfileName.trim() || `档案 ${profiles.length + 1}`;
    const host = newProfileHost.trim();
    profileError = "";
    if (!host) {
      profileError = "请先填写档案地址";
      return;
    }
    if (!(newProfilePort >= 1 && newProfilePort <= 65535)) {
      profileError = "端口需在 1-65535 之间";
      return;
    }
    if (profiles.some((item) => item.name === name)) {
      profileError = `档案名称「${name}」已存在`;
      return;
    }
    draft.upstreams = [...profiles, { name, protocol: newProfileProtocol, host, port: newProfilePort }];
    newProfileName = "";
    speedResults = null;
    touch();
  }

  function removeProfile(index: number) {
    const removed = profiles[index];
    draft.upstreams = profiles.filter((_, i) => i !== index);
    if (draft.activeUpstream === removed?.name) draft.activeUpstream = "";
    speedResults = null;
    scheduleAutoSave();
  }

  function addRoute() {
    const pattern = newRoutePattern.trim();
    routeError = "";
    if (!pattern) {
      routeError = "请先填写域名模式，例如 *.openai.com";
      return;
    }
    if (routes.some((item) => item.pattern === pattern)) {
      routeError = `规则「${pattern}」已存在`;
      return;
    }
    draft.routes = [...routes, { pattern, action: newRouteAction }];
    newRoutePattern = "";
    touch();
  }

  function removeRoute(index: number) {
    draft.routes = routes.filter((_, i) => i !== index);
    scheduleAutoSave();
  }

  function addAllowedHost() {
    const host = newHost.trim();
    hostError = "";
    if (!host) {
      hostError = "请先填写要放行的域名";
      return;
    }
    if (allowedHosts.includes(host)) {
      hostError = `域名「${host}」已在白名单中`;
      return;
    }
    draft.allowedHosts = [...allowedHosts, host];
    newHost = "";
    touch();
  }

  function removeAllowedHost(index: number) {
    draft.allowedHosts = allowedHosts.filter((_, i) => i !== index);
    scheduleAutoSave();
  }

  function toggleAllowlist() {
    draft.hostFilterMode = draft.hostFilterMode === "allowlist" ? "off" : "allowlist";
    scheduleAutoSave();
  }

  function moveRoute(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= routes.length) return;
    const next = [...routes];
    const [item] = next.splice(index, 1);
    next.splice(target, 0, item);
    draft.routes = next;
    scheduleAutoSave();
  }

  function submitOnEnter(event: KeyboardEvent, action: () => void) {
    if (event.key === "Enter") {
      event.preventDefault();
      action();
    }
  }

  async function enableProfile(name: string) {
    if (busy === "switch" || busy === "save") return;
    const saved = await onSave(prunedDraft());
    if (!saved) return;
    await onSwitchUpstream(name);
  }

  async function selectFastest() {
    if (busy === "switch" || busy === "save") return;
    const saved = await onSave(prunedDraft());
    if (!saved) return;
    await onSelectFastest();
  }

  function restoreDefaults() {
    draft = { ...defaultDraft };
    dirty = true;
    probe = null;
  }

  function updateLabel(status: UpdateStatus | null | undefined): string {
    switch (status?.status) {
      case "available":
        return `发现 v${status.latestVersion}`;
      case "downloading":
        return `下载 ${updatePercent(status)}%`;
      case "ready":
        return "可更新";
      case "installing":
      case "restarting":
        return "安装中";
      case "checking":
        return "检查中";
      case "current":
        return "已是最新";
      case "unreleased":
        return "暂无 Release";
      case "unavailable":
        return "无法检查";
      case "error":
        return "检查失败";
      default:
        return "尚未检查";
    }
  }

  function updateTone(status: UpdateStatus | null | undefined): string {
    switch (status?.status) {
      case "available":
      case "downloading":
      case "ready":
      case "installing":
      case "restarting":
        return "run";
      case "current":
        return "pass";
      case "unavailable":
        return "warn";
      case "error":
        return "fail";
      default:
        return "skip";
    }
  }

  function updateDetail(status: UpdateStatus | null | undefined): string {
    if (!status || status.status === "unknown") {
      return "启动后会自动检查 GitHub Releases，也可以手动检查。";
    }
    if (status.status === "available") {
      return `当前 v${status.currentVersion}，新版本 v${status.latestVersion} 已发布，正在后台下载。`;
    }
    if (status.status === "downloading") {
      return `正在下载 v${status.latestVersion} · ${formatBytes(status.downloaded)} / ${formatBytes(status.total)}`;
    }
    if (status.status === "ready") {
      return `v${status.latestVersion} 已下载并通过 SHA-256 校验。重启时通道会短暂中断，Codex 不会被结束。`;
    }
    if (status.status === "installing" || status.status === "restarting") {
      return "正在替换程序并重新启动，旧版本会在失败时自动回滚。";
    }
    if (status.status === "current") {
      return `当前 v${status.currentVersion} 已是频道内最新版本。`;
    }
    return status.error || "暂时无法读取发布信息。";
  }

  function updatePercent(status: UpdateStatus | null | undefined): number {
    if (!status?.total) return 0;
    return Math.round(clampPercent((status.downloaded / status.total) * 100));
  }

  async function checkUpdates() {
    await onCheckUpdate(false);
  }
</script>

<section class="view active" id="view-settings" aria-labelledby="settings-title">
  <div class="page-head">
    <div class="page-head-copy">
      <h1 id="settings-title">设置</h1>
      <p class="page-sub">修改只作用于 Codex 进程和所选内核环境，不会更改 Windows 系统代理。</p>
    </div>
    <div class="page-actions">
      <button class="btn" onclick={restoreDefaults}><Icon name="refresh" size={15} />恢复默认</button>
    </div>
  </div>

  <div class="settings-layout">
    <div class="settings-stack">
      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">代理入口</h2><p class="panel-sub">Codex 看到的是 Lane 双入口，Lane 再转发到你已有的代理。</p></div><div class="right"><span class={`result-pill ${snapshot.upstream.reachable ? "pass" : "pending"}`}>{snapshot.upstream.reachable ? "上游可达" : "待检测"}</span></div></div>
        <div class="setting-row">
          <div class="setting-copy"><b>上游协议</b><span>支持 HTTP、HTTPS 与 SOCKS5；不保存用户名和密码。</span></div>
          <div class="setting-control">
            <select class="native-select" value={draft.protocol} onchange={(event) => updateTopField("protocol", event.currentTarget.value)} aria-label="上游协议">
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
              <option value="socks5">SOCKS5</option>
            </select>
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>上游地址</b><span>本机 Clash、Mihomo 或其他代理服务的监听地址。</span></div>
          <div class="setting-control">
            <input class="text-input" style="width:132px" value={draft.host} oninput={(event) => updateTopField("host", event.currentTarget.value)} aria-label="上游地址" />
            <input class="text-input" style="width:88px" type="number" min="1" max="65535" value={draft.port} oninput={(event) => updateTopField("port", event.currentTarget.value)} aria-label="上游端口" />
            <button class="btn small" onclick={testUpstream} disabled={busy === "probe"}>
              {#if busy === "probe"}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="activity" size={13} />{/if}
              测试
            </button>
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>Lane 本地入口</b><span>双入口只监听回环地址；首次自动选空闲端口，之后固定复用，重启 Lane 不会改变 Codex 的代理端口。</span></div>
          <div class="setting-control inline-actions">
            <span class="input-box mono">{snapshot.lane.running ? `UI 127.0.0.1:${snapshot.lane.interfacePort}` : "UI 自动分配"}</span>
            <span class="input-box mono">{snapshot.lane.running ? `K 127.0.0.1:${snapshot.lane.kernelPort}` : "内核自动分配"}</span>
          </div>
        </div>
        {#if probe}
          <div class="setting-row" style="background:#fbfbfc">
            <div class="setting-copy"><b>{probe.reachable ? "上游探测成功" : "上游探测失败"}</b><span class:green={probe.reachable} class:red={!probe.reachable}>{probe.reachable ? `${upstreamProtocol(draft.protocol)} CONNECT · ${formatDuration(probe.latencyMs)}` : probe.error}</span></div>
          </div>
          {#if probe.domains?.length}
            <div class="probe-list">
              {#each probe.domains as item}
                <div class="probe-item">
                  <i class:ok={item.ok} class:warn={!item.ok && item.optional} class:bad={!item.ok && !item.optional}></i>
                  <code>{item.host}</code>
                  <span>{item.ok ? `${item.tls}${item.status ? ` · HTTP ${item.status}` : ""} · ${formatDuration(item.latencyMs)}` : item.error}</span>
                  <em class:optional={item.optional}>{item.ok ? "可连通" : item.optional ? "可选" : "失败"}</em>
                </div>
              {/each}
            </div>
          {/if}
        {/if}
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">上游档案</h2><p class="panel-sub">保存多套代理地址，一键启用或自动选最快；启用会立即重启 Lane。</p></div></div>
        <div class="entry-list">
          <div class="entry-head grid-profiles"><span>档案名称</span><span>协议</span><span>地址</span><span>端口</span><span>操作</span></div>
          {#each profiles as profile, index (index)}
            <div class="entry-row grid-profiles">
              <input class="text-input" value={profile.name} oninput={(event) => updateProfile(index, { name: event.currentTarget.value })} aria-label="档案名称" />
              <select class="native-select" value={profile.protocol} onchange={(event) => updateProfile(index, { protocol: event.currentTarget.value })} aria-label="档案协议">
                <option value="http">HTTP</option>
                <option value="https">HTTPS</option>
                <option value="socks5">SOCKS5</option>
              </select>
              <input class="text-input" value={profile.host} oninput={(event) => updateProfile(index, { host: event.currentTarget.value })} aria-label="档案地址" />
              <input class="text-input" type="number" min="1" max="65535" value={profile.port} oninput={(event) => updateProfile(index, { port: Number(event.currentTarget.value) })} aria-label="档案端口" />
              <div class="entry-actions">
                {#if speedByName.get(profile.name)}
                  {@const result = speedByName.get(profile.name)}
                  {#if result && result.reachable}
                    <span class="result-pill pass" title="CONNECT 延迟">{result.latencyMs} ms{fastestName === profile.name ? " · 最快" : ""}</span>
                  {:else if result}
                    <span class="result-pill fail" title={result.error}>不可达</span>
                  {:else}
                    <span class="result-pill pending">未测速</span>
                  {/if}
                {/if}
                {#if profile.name === draft.activeUpstream}
                  <span class="result-pill pass">使用中</span>
                {:else}
                  <button class="btn small" onclick={() => void enableProfile(profile.name)} disabled={busy === "switch" || busy === "save"}>启用</button>
                {/if}
                <button class="icon-btn danger" onclick={() => removeProfile(index)} aria-label={`删除档案 ${profile.name}`} title="删除档案"><Icon name="x" size={12} /></button>
              </div>
            </div>
          {:else}
            <div class="entry-empty">还没有档案。在下方把当前代理存为第一套档案，也可以继续只使用单代理配置。</div>
          {/each}
          <div class="entry-foot grid-profiles">
            <input class="text-input" placeholder="名称" bind:value={newProfileName} aria-label="新档案名称" />
            <select class="native-select" bind:value={newProfileProtocol} aria-label="新档案协议">
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
              <option value="socks5">SOCKS5</option>
            </select>
            <input class="text-input" placeholder="地址，如 127.0.0.1" bind:value={newProfileHost} aria-label="新档案地址" />
            <input class="text-input" type="number" min="1" max="65535" placeholder="端口" bind:value={newProfilePort} aria-label="新档案端口" />
            <button class="btn small" onclick={addProfile}><Icon name="plus" size={13} />添加</button>
          </div>
        </div>
        {#if profileError}<p class="entry-error">{profileError}</p>{/if}
        <div class="setting-row">
          <div class="setting-copy"><b>故障自动切换</b><span>连续两次探测失败时，自动切到第一个可用的其他档案。</span></div>
          <div class="setting-control">
            <button class:off={!draft.autoFailover} class="switch" aria-label="故障自动切换" onclick={() => { draft.autoFailover = !draft.autoFailover; scheduleAutoSave(); }}></button>
            <button class="btn small" onclick={() => void runSpeedTest()} disabled={speedBusy || busy === "switch" || busy === "save"}>
              {#if speedBusy}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="activity" size={13} />{/if}
              测速
            </button>
            <button class="btn small" onclick={() => void selectFastest()} disabled={busy === "switch" || busy === "save"}>
              {#if busy === "switch"}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="activity" size={13} />{/if}
              测速选优
            </button>
          </div>
        </div>
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">分流规则</h2><p class="panel-sub">从上到下按序匹配，第一条命中的规则生效；未命中的域名走当前上游。支持 *.domain.com、精确域名或 *。</p></div><div class="right"><span class="result-pill skip">{routes.length} 条</span></div></div>
        <div class="entry-list">
          <div class="entry-head grid-routes"><span>#</span><span>域名模式</span><span>动作</span><span>操作</span></div>
          {#each routes as route, index (index)}
            <div class="entry-row grid-routes">
              <span class="entry-index">{index + 1}</span>
              <input class="text-input" placeholder="*.openai.com" bind:value={route.pattern} oninput={touch} aria-label={`规则 ${index + 1} 域名模式`} />
              <select class="native-select" bind:value={route.action} onchange={touch} aria-label={`规则 ${index + 1} 动作`}>
                <option value="proxy">走代理</option>
                <option value="direct">直连</option>
              </select>
              <div class="entry-actions">
                <button class="icon-btn" disabled={index === 0} onclick={() => moveRoute(index, -1)} aria-label={`上移规则 ${route.pattern}`} title="上移"><span class="flip"><Icon name="chevron" size={13} /></span></button>
                <button class="icon-btn" disabled={index === routes.length - 1} onclick={() => moveRoute(index, 1)} aria-label={`下移规则 ${route.pattern}`} title="下移"><Icon name="chevron" size={13} /></button>
                <button class="icon-btn danger" onclick={() => removeRoute(index)} aria-label={`删除规则 ${route.pattern}`} title="删除"><Icon name="x" size={12} /></button>
              </div>
            </div>
          {:else}
            <div class="entry-empty">暂无规则，所有域名都会走当前上游。</div>
          {/each}
          <div class="entry-foot grid-routes">
            <span class="entry-index">+</span>
            <input class="text-input" placeholder="*.openai.com 或 api.example.com" bind:value={newRoutePattern} onkeydown={(event) => submitOnEnter(event, addRoute)} aria-label="新规则域名模式" />
            <select class="native-select" bind:value={newRouteAction} aria-label="新规则动作">
              <option value="proxy">走代理</option>
              <option value="direct">直连</option>
            </select>
            <button class="btn small" onclick={addRoute}><Icon name="plus" size={13} />添加</button>
          </div>
        </div>
        {#if routeError}<p class="entry-error">{routeError}</p>{/if}
        <div class="setting-row">
          <div class="setting-copy"><b>域名白名单</b><span>开启后只放行列出的域名，防止本机其他程序借用 Lane 出网。</span></div>
          <div class="setting-control">
            <button class:off={draft.hostFilterMode !== "allowlist"} class="switch" aria-label="域名白名单" onclick={toggleAllowlist}></button>
            <span class={`result-pill ${draft.hostFilterMode === "allowlist" ? "run" : "skip"}`}>{draft.hostFilterMode === "allowlist" ? "已开启" : "已关闭"}</span>
          </div>
        </div>
        {#if draft.hostFilterMode === "allowlist"}
          <div class="entry-list">
            <div class="entry-head grid-hosts"><span>放行域名（{allowedHosts.length}）</span><span>操作</span></div>
            {#each allowedHosts as host, index (host + index)}
              <div class="entry-row grid-hosts">
                <code class="host-code">{host}</code>
                <div class="entry-actions">
                  <button class="icon-btn danger" onclick={() => removeAllowedHost(index)} aria-label={`删除白名单 ${host}`} title="删除"><Icon name="x" size={12} /></button>
                </div>
              </div>
            {:else}
              <div class="entry-empty">白名单已开启但还没有域名，未列出的域名都会被 Lane 拒绝。</div>
            {/each}
            <div class="entry-foot grid-hosts">
              <input class="text-input" placeholder="*.openai.com" bind:value={newHost} onkeydown={(event) => submitOnEnter(event, addAllowedHost)} aria-label="新白名单域名" />
              <button class="btn small" onclick={addAllowedHost}><Icon name="plus" size={13} />添加</button>
            </div>
          </div>
          {#if hostError}<p class="entry-error">{hostError}</p>{/if}
        {/if}
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">环境依赖</h2><p class="panel-sub">启动 Lane 前自动检查；缺失项可以直接安装。</p></div><div class="right"><button class="btn small" onclick={loadDependencies} disabled={depsLoading}>{depsLoading ? "检查中…" : "重新检查"}</button><span class={`result-pill ${missingDeps ? "warn" : "pass"}`}>{depsLoading ? "检测中" : missingDeps ? `${missingDeps} 项待处理` : "全部就绪"}</span></div></div>
        <div class="rule-table">
          {#each deps as dep}
            <div class="rule-row">
              <code>{dep.name}</code>
              <span>{dep.detail}{dep.hint ? ` · ${dep.hint}` : ""}</span>
              {#if dep.ready}
                <em class="ok">就绪</em>
              {:else}
                <button class="btn small" onclick={() => installDependency(dep)} disabled={installing === dep.id}>{installing === dep.id ? "安装中…" : dep.action ? "安装" : "去处理"}</button>
              {/if}
            </div>
          {/each}
        </div>
        {#if installNote}
          <div class="setting-row" style="background:#fbfbfc"><div class="setting-copy"><b>安装结果</b><span>{installNote}</span></div></div>
        {/if}
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">Clash 规则检查</h2><p class="panel-sub">规则模式下，只有命中的域名才会走策略组，其余由 MATCH 兜底决定。</p></div><div class="right"><span class="result-pill skip">需人工确认</span></div></div>
        <div class="rule-table">
          {#each requiredDomains as item}
            <div class="rule-row">
              <code>{item.domain}</code>
              <span>{item.purpose}</span>
              <em class:optional={item.optional}>{item.optional ? "可选" : "必需"}</em>
            </div>
          {/each}
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>复制规则片段</b><span>粘贴到配置的 rules 里，把组名替换成你自己的策略组。</span></div>
          <div class="setting-control">
            <button class="btn small" onclick={copyRules}>
              {#if copied}<Icon name="check" size={13} />已复制{:else}<Icon name="copy" size={13} />复制规则{/if}
            </button>
          </div>
        </div>
        <pre class="rule-snippet mono">{ruleSnippet}</pre>
        <div class="setting-row" style="background:#fbfbfc">
          <div class="setting-copy"><b>还要确认两件事</b><span>策略组当前选中的必须是节点而不是 DIRECT；MATCH 兜底若不是 DIRECT，未命中的域名也会走代理。</span></div>
        </div>
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">运行环境</h2><p class="panel-sub">决定 Codex 主程序与内核如何继承代理。</p></div></div>
        <div class="setting-row">
          <div class="setting-copy"><b>内核模式</b><span>自动只读取 config.toml；手动切换会先备份再原子替换。</span></div>
          <div class="setting-control">
            <div class="mode-seg" role="group" aria-label="内核模式">
              <button class:active={draft.engine === "auto"} onclick={() => { draft.engine = "auto"; scheduleAutoSave(); }}><i class="mini"></i>自动</button>
              <button class:active={draft.engine === "windows"} onclick={() => { draft.engine = "windows"; scheduleAutoSave(); }}>Windows</button>
              <button class:active={draft.engine === "wsl"} onclick={() => { draft.engine = "wsl"; scheduleAutoSave(); }}>WSL</button>
            </div>
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>检测到的 WSL</b><span>首版要求 mirrored 网络模式，NAT 会在启动前给出明确错误。</span></div>
          <div class="setting-control">
            <select class="native-select" bind:value={draft.wslDistro} onchange={scheduleAutoSave} aria-label="WSL 发行版">
              <option value="">自动选择</option>
              {#each distros as distro}<option value={distro}>{distro}</option>{/each}
            </select>
            <span class={`result-pill ${snapshot.wsl?.interopOk ? "pass" : snapshot.engine.resolved === "wsl" ? "warn" : "skip"}`}>{snapshot.wsl?.networkMode || "未检测"}</span>
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>Codex 安装位置</b><span>按完整 MSIX 包路径定位，不使用通用进程名扫描。</span></div>
          <div class="setting-control">
            <span class="path-box" title={snapshot.codex.executablePath}><Icon name="folder" size={14} /><span>{snapshot.codex.executablePath || "未找到 OpenAI.Codex"}</span></span>
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>配置文件</b><span>保存在系统本地配置目录；exe 放在桌面也不会生成 launcher.json。</span></div>
          <div class="setting-control">
            <button type="button" class="path-box path-button" title="在资源管理器中显示配置文件" onclick={onOpenConfigFolder}><Icon name="file" size={14} /><span>{snapshot.environment.configPath || "未检测"}</span></button>
            <button class="btn small" onclick={onOpenConfigFolder}><Icon name="folder" size={13} />打开</button>
          </div>
        </div>
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">启动、隔离与记录</h2><p class="panel-sub">默认最小权限，所有变更都可见、可撤销。</p></div></div>
        <div class="setting-row">
          <div class="setting-copy"><b>启动 Lane 时打开 Codex</b><span>只有显式开启后才会拉起 Codex Desktop。</span></div>
          <div class="setting-control"><button class:off={!draft.openCodex} class="switch" aria-label="启动 Lane 时打开 Codex" onclick={() => { draft.openCodex = !draft.openCodex; scheduleAutoSave(); }}></button></div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>退出 Lane 时同步停止 Codex</b><span>只处理当前检测到的主进程，不影响其他应用。</span></div>
          <div class="setting-control"><button class:off={!draft.stopCodexOnExit} class="switch" aria-label="退出 Lane 时同步停止 Codex" onclick={() => { draft.stopCodexOnExit = !draft.stopCodexOnExit; scheduleAutoSave(); }}></button></div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>关闭窗口时最小化到托盘</b><span>点关闭按钮隐藏到系统托盘，从托盘菜单退出；重启应用后生效。</span></div>
          <div class="setting-control"><button class:off={!draft.minimizeToTray} class="switch" aria-label="关闭窗口时最小化到托盘" onclick={() => { draft.minimizeToTray = !draft.minimizeToTray; scheduleAutoSave(); }}></button></div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>开机自动启动</b><span>写入当前用户的注册表 Run 项；改动即时生效。</span></div>
          <div class="setting-control"><button class:off={!draft.launchAtLogin} class="switch" aria-label="开机自动启动" onclick={() => { draft.launchAtLogin = !draft.launchAtLogin; scheduleAutoSave(); }}></button></div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>请求元数据日志</b><span>记录域名、端口、状态、流量和耗时，不读取 HTTPS 内容。</span></div>
          <div class="setting-control">
            <select class="native-select" bind:value={draft.retentionDays} onchange={scheduleAutoSave} aria-label="日志保留天数">
              <option value={7}>保留 7 天</option>
              <option value={14}>保留 14 天</option>
              <option value={30}>保留 30 天</option>
            </select>
          </div>
        </div>
      </article>
    </div>

    <aside class="settings-side">
      <div class="panel apply-card">
        <div class="panel-head"><div><h2 class="panel-title">应用配置</h2><p class="panel-sub">开关与下拉即时保存；地址和端口确认后应用。</p></div><div class="right"><span class={`result-pill ${autosaving ? "run" : changeCount ? "run" : "pass"}`}>{autosaving ? "保存中…" : changeCount ? `${changeCount} 项待应用` : "已保存"}</span></div></div>
        <div class="apply-body">
          <div class="apply-summary">
            <div class="apply-row"><span class="k">上游代理</span><span class="v mono">{upstreamProtocol(draft.protocol)} · {draft.host}:{draft.port}</span></div>
            <div class="apply-row"><span class="k">内核模式</span><span class="v green">{draft.engine === "auto" ? `自动识别 · ${snapshot.engine.resolved === "wsl" ? "WSL" : "Windows"}` : draft.engine === "wsl" ? "WSL" : "Windows"}</span></div>
            <div class="apply-row"><span class="k">WSL 预检</span><span class="v green">{snapshot.wsl?.networkMode || "未使用"} · {snapshot.wsl?.interopOk ? "已通过" : "未检测"}</span></div>
            <div class="apply-row"><span class="k">系统代理</span><span class="v green">不修改</span></div>
            <div class="apply-row"><span class="k">配置备份</span><span class="v green">写入前自动创建</span></div>
          </div>
          <div class="apply-actions">
            <button class="btn dark" onclick={save} disabled={!dirty || busy === "save"}>
              {#if busy === "save"}<span class="spin"><Icon name="refresh" size={14} /></span>{:else}<Icon name="check" size={14} />{/if}
              保存并应用
            </button>
            <button class="btn" onclick={cancel} disabled={!dirty}>取消更改</button>
          </div>
        </div>
      </div>

      <article class="panel settings-card about-card" id="settings-about">
        <div class="panel-head">
          <div><h2 class="panel-title">关于</h2><p class="panel-sub">版本、仓库与更新信息。</p></div>
          <div class="right"><span class="result-pill pass">v{snapshot.version}</span></div>
        </div>
        <div class="about-project">
          <span class="about-mark"><Icon name="mark" size={26} /></span>
          <div><b>Codex Lane</b><span>Codex 的独立进程级代理通道</span></div>
        </div>
        <button class="about-link" onclick={() => onOpenProject("repository")} title="打开 GitHub 仓库">
          <span class="about-link-icon"><Icon name="code" size={15} /></span>
          <span class="about-link-copy"><b>GitHub 仓库</b><code>{snapshot.repositoryUrl}</code></span>
          <Icon name="external" size={14} />
        </button>
        <div class="about-update">
          <div class="about-update-head">
            <div><b>更新检查</b><span>通过当前上游代理访问 GitHub Releases，不经过 Lane。</span></div>
            <span class={`result-pill ${updateTone(snapshot.update)}`}>{updateLabel(snapshot.update)}</span>
          </div>
          <p>{updateDetail(snapshot.update)}</p>
          {#if snapshot.update?.checkedAt}
            <span class="about-checked">上次检查 · {clockDate(snapshot.update.checkedAt)}</span>
          {/if}
          <div class="about-update-actions">
            <button class="btn small" onclick={checkUpdates} disabled={busy === "update" || busy === "update-download" || busy === "update-restart"}>
              {#if busy === "update"}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="refresh" size={13} />{/if}
              检查更新
            </button>
            {#if snapshot.update?.status === "ready"}
              <button class="btn small dark" onclick={onRestartUpdate} disabled={busy === "update-restart"}>
                {#if busy === "update-restart"}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="power" size={13} />{/if}
                重启并更新
              </button>
            {:else if snapshot.update?.status === "downloading"}
              <button class="btn small" disabled><span class="spin"><Icon name="download" size={13} /></span>下载 {updatePercent(snapshot.update)}%</button>
            {:else if snapshot.update?.available}
              <button class="btn small" onclick={onDownloadUpdate} disabled={busy === "update-download"}>
                {#if busy === "update-download"}<span class="spin"><Icon name="download" size={13} /></span>{:else}<Icon name="download" size={13} />{/if}
                下载更新
              </button>
            {/if}
            {#if snapshot.update?.available}
              <button class="btn small dark" onclick={() => onOpenProject("releases")}><Icon name="download" size={13} />打开 Release</button>
            {/if}
          </div>
        </div>
        <div class="about-meta"><span>Windows x64 · 单文件便携版</span><span>不安装服务或驱动</span></div>
      </article>
    </aside>
  </div>
</section>

<style>
  .entry-list {
    margin: 0 18px 12px;
    border: 1px solid var(--line);
    border-radius: 10px;
    overflow: hidden;
    background: #fff;
  }
  .entry-head,
  .entry-row,
  .entry-foot {
    display: grid;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
  }
  .entry-head {
    height: 34px;
    background: #fbfbfc;
    border-bottom: 1px solid var(--line);
    color: var(--text-3);
    font-size: 11px;
    font-weight: 650;
  }
  .entry-row {
    min-height: 46px;
    border-bottom: 1px solid #f0f0f4;
  }
  .entry-row:hover {
    background: #fafafd;
  }
  .entry-foot {
    min-height: 50px;
    background: #fbfbfc;
    border-top: 1px solid var(--line);
  }
  .entry-empty {
    padding: 18px 12px;
    color: var(--text-3);
    font-size: 12px;
    text-align: center;
  }
  .entry-index {
    color: var(--text-4);
    font: 11px var(--mono);
    text-align: center;
  }
  .entry-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 6px;
  }
  .entry-error {
    margin: -4px 20px 10px;
    color: var(--red);
    font-size: 11.5px;
  }
  .grid-profiles {
    grid-template-columns: 118px 88px 1fr 76px auto;
  }
  .grid-routes {
    grid-template-columns: 28px 1fr 104px auto;
  }
  .grid-hosts {
    grid-template-columns: 1fr auto;
  }
  .icon-btn {
    width: 28px;
    height: 28px;
    flex: none;
    display: grid;
    place-items: center;
    border: 1px solid var(--line);
    border-radius: 7px;
    background: #fff;
    color: var(--text-3);
  }
  .icon-btn:hover {
    color: var(--text-1);
    border-color: #d9d9e0;
  }
  .icon-btn.danger:hover {
    color: var(--red);
    border-color: var(--red-line);
    background: var(--red-soft);
  }
  .icon-btn:disabled {
    opacity: 0.4;
    pointer-events: none;
  }
  .flip {
    display: inline-flex;
    transform: rotate(180deg);
  }
  .host-code {
    font: 12px var(--mono);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
