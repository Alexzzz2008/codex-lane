<script lang="ts">
  import { onMount } from "svelte";
  import type { Dependency, SettingsInput, Snapshot, UpdateStatus, UpstreamStatus } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import { AppService } from "../../bindings/github.com/Alexzzz2008/codex-lane/internal/appservice";
  import Icon from "./Icon.svelte";
  import { clockDate, formatDuration, upstreamProtocol } from "./format";

  let {
    snapshot,
    busy = "",
    onSave,
    onProbe,
    onCheckUpdate,
    onOpenProject,
    onOpenConfigFolder,
    onSwitchUpstream,
    onSelectFastest,
  }: {
    snapshot: Snapshot;
    busy?: string;
    onSave: (input: SettingsInput) => Promise<boolean>;
    onProbe: () => Promise<UpstreamStatus | null>;
    onCheckUpdate: (silent?: boolean) => Promise<UpdateStatus | null>;
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
    void loadDependencies();
  });

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
    const payload = { ...draft };
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
    const success = await onSave({ ...draft });
    if (success) {
      dirty = false;
      baseline = JSON.stringify(draft);
      lastIncoming = baseline;
    }
  }

  async function testUpstream() {
    probe = null;
    probe = await onProbe();
  }

  function cancel() {
    draft = JSON.parse(baseline) as SettingsInput;
    dirty = false;
  }

  function addProfile() {
    const index = profiles.length + 1;
    draft.upstreams = [...(draft.upstreams ?? []), { name: `档案 ${index}`, protocol: draft.protocol, host: draft.host, port: draft.port }];
    scheduleAutoSave();
  }

  function removeProfile(index: number) {
    const removed = profiles[index];
    draft.upstreams = profiles.filter((_, i) => i !== index);
    if (draft.activeUpstream === removed?.name) draft.activeUpstream = "";
    scheduleAutoSave();
  }

  function addRoute() {
    draft.routes = [...routes, { pattern: "", action: "proxy" }];
    scheduleAutoSave();
  }

  function removeRoute(index: number) {
    draft.routes = routes.filter((_, i) => i !== index);
    scheduleAutoSave();
  }

  function addAllowedHost() {
    draft.allowedHosts = [...allowedHosts, ""];
    scheduleAutoSave();
  }

  function removeAllowedHost(index: number) {
    draft.allowedHosts = allowedHosts.filter((_, i) => i !== index);
    scheduleAutoSave();
  }

  function toggleAllowlist() {
    draft.hostFilterMode = draft.hostFilterMode === "allowlist" ? "off" : "allowlist";
    scheduleAutoSave();
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
      return `当前 v${status.currentVersion}，新版本 v${status.latestVersion} 已发布。`;
    }
    if (status.status === "current") {
      return `当前 v${status.currentVersion} 已是频道内最新版本。`;
    }
    return status.error || "暂时无法读取发布信息。";
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
            <select class="native-select" bind:value={draft.protocol} onchange={touch} aria-label="上游协议">
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
              <option value="socks5">SOCKS5</option>
            </select>
          </div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>上游地址</b><span>本机 Clash、Mihomo 或其他代理服务的监听地址。</span></div>
          <div class="setting-control">
            <input class="text-input" style="width:132px" bind:value={draft.host} oninput={touch} aria-label="上游地址" />
            <input class="text-input" style="width:88px" type="number" min="1" max="65535" bind:value={draft.port} oninput={touch} aria-label="上游端口" />
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
        <div class="panel-head"><div><h2 class="panel-title">上游档案</h2><p class="panel-sub">保存多套代理地址，一键切换或自动选最快；切换会立即重启 Lane。</p></div><div class="right"><button class="btn small" onclick={addProfile}><Icon name="plus" size={13} />添加档案</button><button class="btn small" onclick={() => void onSelectFastest()} disabled={busy === "switch"}>{#if busy === "switch"}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="activity" size={13} />{/if}测速选优</button></div></div>
        {#if profiles.length}
          <div class="rule-table">
            {#each profiles as profile, index}
              <div class="rule-row">
                <input class="text-input" style="width:110px" bind:value={profile.name} oninput={touch} aria-label="档案名称" />
                <select class="native-select" bind:value={profile.protocol} onchange={touch} aria-label="档案协议">
                  <option value="http">HTTP</option>
                  <option value="https">HTTPS</option>
                  <option value="socks5">SOCKS5</option>
                </select>
                <input class="text-input" style="width:118px" bind:value={profile.host} oninput={touch} aria-label="档案地址" />
                <input class="text-input" style="width:76px" type="number" min="1" max="65535" bind:value={profile.port} oninput={touch} aria-label="档案端口" />
                {#if profile.name === draft.activeUpstream}
                  <em class="ok">使用中</em>
                {:else}
                  <button class="btn small" onclick={() => void onSwitchUpstream(profile.name)} disabled={busy === "switch"}>启用</button>
                {/if}
                <button class="btn small" onclick={() => removeProfile(index)} aria-label={`删除档案 ${profile.name}`}><Icon name="x" size={12} /></button>
              </div>
            {/each}
          </div>
        {:else}
          <div class="setting-row" style="background:#fbfbfc"><div class="setting-copy"><b>还没有档案</b><span>点「添加档案」把当前代理存为第一套档案；也可以直接只使用上方的单代理配置。</span></div></div>
        {/if}
        <div class="setting-row">
          <div class="setting-copy"><b>故障自动切换</b><span>连续两次探测失败时，自动切到第一个可用的其他档案。</span></div>
          <div class="setting-control"><button class:off={!draft.autoFailover} class="switch" aria-label="故障自动切换" onclick={() => { draft.autoFailover = !draft.autoFailover; scheduleAutoSave(); }}></button></div>
        </div>
      </article>

      <article class="panel settings-card">
        <div class="panel-head"><div><h2 class="panel-title">分流与访问控制</h2><p class="panel-sub">命中「直连」的域名绕过上游；开启白名单后，未列出的域名会被 Lane 拒绝。</p></div><div class="right"><span class={`result-pill ${draft.hostFilterMode === "allowlist" ? "run" : "skip"}`}>{draft.hostFilterMode === "allowlist" ? "白名单开启" : "白名单关闭"}</span></div></div>
        <div class="rule-table">
          {#each routes as route, index}
            <div class="rule-row">
              <input class="text-input" style="width:190px" placeholder="*.openai.com" bind:value={route.pattern} oninput={touch} aria-label="分流模式" />
              <select class="native-select" bind:value={route.action} onchange={touch} aria-label="分流动作">
                <option value="proxy">走代理</option>
                <option value="direct">直连</option>
              </select>
              <span class="muted">支持 *.domain.com、精确域名或 *</span>
              <button class="btn small" onclick={() => removeRoute(index)} aria-label={`删除规则 ${route.pattern}`}><Icon name="x" size={12} /></button>
            </div>
          {/each}
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>添加分流规则</b><span>按顺序匹配，第一条命中的规则生效；未命中的域名走当前上游。</span></div>
          <div class="setting-control"><button class="btn small" onclick={addRoute}><Icon name="plus" size={13} />添加规则</button></div>
        </div>
        <div class="setting-row">
          <div class="setting-copy"><b>域名白名单</b><span>只放行列出的域名，防止本机其他程序借用 Lane 出网。</span></div>
          <div class="setting-control">
            <button class:off={draft.hostFilterMode !== "allowlist"} class="switch" aria-label="域名白名单" onclick={toggleAllowlist}></button>
            <button class="btn small" onclick={addAllowedHost} disabled={draft.hostFilterMode !== "allowlist"}><Icon name="plus" size={13} />添加域名</button>
          </div>
        </div>
        {#if draft.hostFilterMode === "allowlist"}
          <div class="rule-table">
            {#each allowedHosts as host, index}
              <div class="rule-row">
                <input class="text-input" style="width:190px" placeholder="*.openai.com" value={host} oninput={(event) => { allowedHosts[index] = (event.currentTarget as HTMLInputElement).value; touch(); }} aria-label="白名单域名" />
                <button class="btn small" onclick={() => removeAllowedHost(index)} aria-label={`删除白名单 ${host}`}><Icon name="x" size={12} /></button>
              </div>
            {/each}
          </div>
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
            <div><b>更新检查</b><span>通过 GitHub Releases 获取版本信息，不经过 Lane。</span></div>
            <span class={`result-pill ${updateTone(snapshot.update)}`}>{updateLabel(snapshot.update)}</span>
          </div>
          <p>{updateDetail(snapshot.update)}</p>
          {#if snapshot.update?.checkedAt}
            <span class="about-checked">上次检查 · {clockDate(snapshot.update.checkedAt)}</span>
          {/if}
          <div class="about-update-actions">
            <button class="btn small" onclick={checkUpdates} disabled={busy === "update"}>
              {#if busy === "update"}<span class="spin"><Icon name="refresh" size={13} /></span>{:else}<Icon name="refresh" size={13} />{/if}
              检查更新
            </button>
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
