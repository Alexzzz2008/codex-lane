<script lang="ts">
  import { onMount } from "svelte";
  import { Events } from "@wailsio/runtime";
  import { AppService } from "../bindings/github.com/Alexzzz2008/codex-lane/internal/appservice";
  import type {
    Event,
    LaunchResult,
    RequestRecord,
    SettingsInput,
    Snapshot,
    UpdateStatus,
    UpstreamStatus,
  } from "../bindings/github.com/Alexzzz2008/codex-lane/internal/model";
  import Icon from "./lib/Icon.svelte";
  import Overview from "./lib/Overview.svelte";
  import Requests from "./lib/Requests.svelte";
  import Checks from "./lib/Checks.svelte";
  import Settings from "./lib/Settings.svelte";

  type Page = "overview" | "requests" | "checks" | "settings";
  type Toast = { id: number; kind: "ok" | "error" | "info"; title: string; detail: string };
  type ModalState = {
    title: string;
    body: string;
    note?: string;
    confirm: string;
    danger?: boolean;
    action: () => Promise<void> | void;
  };

  let snapshot = $state<Snapshot | null>(null);
  let loading = $state(true);
  let loadError = $state("");
  let busy = $state("");
  let activePage = $state<Page>("overview");
  let toasts = $state<Toast[]>([]);
  let modal = $state<ModalState | null>(null);
  let toastID = 0;
  let polling = true;
  let readyUpdateNotified = "";
  let loadSequence = 0;
  let requestRefreshTimer: number | undefined;

  const pages: Array<{ id: Page; label: string }> = [
    { id: "overview", label: "总览" },
    { id: "requests", label: "请求日志" },
    { id: "checks", label: "检查" },
    { id: "settings", label: "设置" },
  ];
  const titles: Record<Page, string> = {
    overview: "运行总览",
    requests: "请求日志",
    checks: "检查",
    settings: "设置",
  };

  function notify(kind: Toast["kind"], title: string, detail = "") {
    const id = ++toastID;
    toasts = [...toasts, { id, kind, title, detail }].slice(-4);
    window.setTimeout(() => {
      toasts = toasts.filter((toast) => toast.id !== id);
    }, kind === "error" ? 6500 : 4200);
  }

  function errorText(error: unknown): string {
    if (typeof error === "string") return error;
    if (error instanceof Error) return error.message;
    if (error && typeof error === "object" && "message" in error) return String((error as { message: unknown }).message);
    return String(error ?? "未知错误");
  }

  function navigate(page: string) {
    const next = pages.find((item) => item.id === page)?.id ?? "overview";
    activePage = next;
    if (location.hash !== `#${next}`) history.replaceState(null, "", `#${next}`);
  }

  async function load(deep = true) {
    const sequence = ++loadSequence;
    try {
      const next = deep ? await AppService.Refresh() : await AppService.GetState();
      if (sequence !== loadSequence) return false;
      snapshot = next;
      loadError = "";
      return true;
    } catch (error) {
      if (sequence !== loadSequence) return false;
      loadError = errorText(error);
      if (!snapshot) notify("error", "无法读取 Codex Lane 状态", loadError);
      return false;
    } finally {
      loading = false;
    }
  }

  async function refresh() {
    if (busy) return;
    busy = "refresh";
    const refreshed = await load(true);
    busy = "";
    if (refreshed) notify("info", "状态已刷新", "已重新读取环境、上游与 Codex 进程状态");
    else notify("error", "刷新失败", loadError);
  }

  async function launch(restart = false) {
    if (!snapshot) return;
    if (!restart && snapshot.codex.running && !snapshot.codex.proxyMatched) {
      modal = {
        title: "Codex 正在运行，但未接入当前 Lane",
        body: "Codex Desktop 是单实例应用，只启动新进程不会重新读取代理参数。",
        note: "确认后只会结束当前 Codex 主进程，再按当前 Lane 双入口重新启动；不会结束其他应用。",
        confirm: "重启并接入",
        action: () => launch(true),
      };
      return;
    }
    busy = restart ? "restart" : "launch";
    try {
      const result: LaunchResult = await AppService.LaunchCodex({ restartExisting: restart });
      if (result.needsInstall) {
        modal = {
          title: "需要先安装 Codex Desktop",
          body: result.message,
          note: "将调用 winget 安装；如果本机没有 winget，会自动打开官方下载页。",
          confirm: "安装并打开",
          action: async () => {
            busy = "launch";
            try {
              const note = await AppService.InstallDependency("codex");
              notify("info", "安装结果", note);
            } finally {
              busy = "";
            }
            await launch(false);
          },
        };
        return;
      }
      if (result.needsRestart) {
        modal = {
          title: "Codex 正在运行，但未接入当前 Lane",
          body: result.message,
          note: `确认后只结束 Codex 主进程（PID ${result.pid}），再按当前 Lane 双入口重新启动；不会结束其他应用。`,
          confirm: "重启并接入",
          action: () => launch(true),
        };
        return;
      }
      if (result.alreadyRunning) {
        notify("ok", "Codex 已在通道中运行", `PID ${result.pid}`);
      } else {
        notify("ok", "Codex 已启动", result.message || `PID ${result.pid}`);
      }
      await load(true);
    } catch (error) {
      const message = errorText(error);
      if (message.includes("重启") || message.includes("已在运行")) {
        modal = {
          title: "需要重启 Codex",
          body: message,
          note: "重启前会确认当前主进程，只操作 OpenAI.Codex 包内的程序。",
          confirm: "重启并接入",
          action: () => launch(true),
        };
      } else {
        notify("error", "Codex 启动失败", message);
      }
    } finally {
      busy = "";
    }
  }

  async function toggleLane() {
    if (!snapshot) return;
    if (snapshot.lane.running) {
      modal = {
        title: "停止 Lane 通道？",
        body: "停止后，当前 Codex 会话的后续请求会失去本地入口；Codex 本身不会被自动结束。",
        note: "系统代理、TUN 和 Clash 配置都不会被修改。",
        confirm: "停止通道",
        danger: true,
        action: async () => {
          busy = "lane";
          try {
            await AppService.StopLane();
            await load(true);
            notify("info", "Lane 通道已停止", "本地双入口已关闭");
          } catch (error) {
            notify("error", "通道停止失败", errorText(error));
          } finally {
            busy = "";
          }
        },
      };
      return;
    }
    busy = "lane";
    try {
      await AppService.StartLane();
      await load(true);
      notify("ok", "Lane 通道已开启", "本地双入口仅监听 127.0.0.1");
    } catch (error) {
      notify("error", "通道启动失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  async function runTests() {
    busy = "tests";
    notify("info", "开始运行连通测试", "包含 OpenAI 域名逐个探测；不会重启 Codex，也不消耗模型额度");
    try {
      const report = await AppService.RunTests();
      busy = "";
      notify(
        report.healthy ? "ok" : "error",
        report.healthy ? "检查通过" : "检查发现失败项",
        `${report.results?.length ?? 0} 项 · ${report.score} 分 · ${report.durationMs} ms`,
      );
      await load(false);
    } catch (error) {
      notify("error", "测试运行失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  function openChecks() {
    navigate("checks");
    if (busy !== "tests") runTests();
  }

  async function exportReport() {
    busy = "export";
    try {
      const report = await AppService.ExportReport();
      await navigator.clipboard.writeText(report);
      notify("ok", "诊断报告已复制", "内容已脱敏，可直接粘贴到 Issue");
    } catch (error) {
      notify("error", "导出失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  async function exportSummary() {
    const report = snapshot?.testReport;
    if (!report) {
      notify("error", "还没有检查结果", "先运行一次检查");
      return;
    }
    busy = "export";
    try {
      const groupNames: Record<string, string> = {
        network: "网络与代理",
        login: "登录与配置",
        session: "会话与进程",
        safety: "隔离与安全",
      };
      const results = report.results ?? [];
      const groups = Object.keys(groupNames).map((id) => {
        const rows = results.filter((item) => item.group === id);
        const passed = rows.filter((item) => item.status === "PASS").length;
        return `${groupNames[id]} ${passed}/${rows.length}`;
      });
      const failures = results
        .filter((item) => item.status === "FAIL" || item.status === "WARN")
        .map((item) => `${item.id} ${item.name} · ${item.status} · ${item.detail}`);
      const lines = [
        `Codex Lane ${snapshot?.version ?? ""} 检查摘要`,
        `时间 ${report.finishedAt} · ${report.score} 分 · ${results.length} 项`,
        groups.join(" · "),
        ...(failures.length ? ["", "需要处理：", ...failures] : ["", "全部通过"]),
      ];
      await navigator.clipboard.writeText(lines.join("\n"));
      notify("ok", "检查摘要已复制", `${report.score} 分 · ${results.length} 项`);
    } catch (error) {
      notify("error", "复制失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  async function clearRequests() {
    loadSequence++;
    busy = "clear";
    try {
      snapshot = await AppService.ClearRequests();
      notify("info", "请求日志已清空", "磁盘 JSONL 历史不会被删除");
    } catch (error) {
      notify("error", "清空失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  async function checkForUpdates(silent = false): Promise<UpdateStatus | null> {
    if (!silent) busy = "update";
    try {
      const status = await AppService.CheckForUpdates();
      if (snapshot) snapshot.update = status;
      if (!silent) {
        if (status.available) {
          notify("ok", `发现新版本 v${status.latestVersion}`, status.status === "ready" ? "已下载并校验，可在「关于」中重启更新" : "正在后台下载更新包");
        } else if (status.status === "current") {
          notify("info", "已是最新版本", `当前 v${status.currentVersion}`);
        } else if (status.status === "unreleased") {
          notify("info", "暂时没有 Release", "仓库发布首个版本后即可接收更新提示");
        } else {
          notify("error", "无法检查更新", status.error || "请检查网络连接");
        }
      }
      return status;
    } catch (error) {
      if (!silent) notify("error", "无法检查更新", errorText(error));
      return null;
    } finally {
      if (!silent) busy = "";
    }
  }

  async function downloadUpdate() {
    busy = "update-download";
    try {
      const status = await AppService.DownloadUpdate();
      if (snapshot) snapshot.update = status;
      if (status.status === "ready") notify("ok", "更新已下载", `v${status.latestVersion} 已通过 SHA-256 校验`);
      else notify("info", "正在下载更新", `v${status.latestVersion}`);
    } catch (error) {
      notify("error", "下载更新失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  async function restartUpdate() {
    modal = {
      title: "重启并更新 Codex Lane？",
      body: "程序会先替换自身，再自动重新启动。Codex 不会被结束，但经过 Lane 的请求会短暂中断几秒。",
      note: "如果新版本无法启动，旧版本会自动回滚；当前配置、日志和代理档案不会受影响。",
      confirm: "重启并更新",
      action: async () => {
        busy = "update-restart";
        try {
          const status = await AppService.RestartToUpdate();
          if (snapshot) snapshot.update = status;
        } catch (error) {
          notify("error", "更新安装失败", errorText(error));
          await load(false);
        } finally {
          busy = "";
        }
      },
    };
  }

  async function openProjectPage(target: "repository" | "releases") {
    try {
      await AppService.OpenProjectPage(target);
    } catch (error) {
      notify("error", "无法打开链接", errorText(error));
    }
  }

  async function openConfigFolder() {
    try {
      await AppService.OpenConfigFolder();
      notify("ok", "已打开配置目录", snapshot?.environment.configPath ?? "");
    } catch (error) {
      notify("error", "无法打开配置目录", errorText(error));
    }
  }

  function openAbout() {
    navigate("settings");
    window.setTimeout(() => document.getElementById("settings-about")?.scrollIntoView({ block: "start", behavior: "smooth" }), 0);
  }

  async function saveSettings(input: SettingsInput): Promise<boolean> {
    loadSequence++;
    busy = "save";
    try {
      snapshot = await AppService.SaveSettings(input);
      notify("ok", "配置已保存", "若上游或内核变化，请重启 Codex 使参数生效");
      return true;
    } catch (error) {
      notify("error", "配置保存失败", errorText(error));
      return false;
    } finally {
      busy = "";
    }
  }

  async function probe(input: SettingsInput): Promise<UpstreamStatus | null> {
    busy = "probe";
    try {
      const result = await AppService.ProbeProxy(input);
      const domains = result.domains ?? [];
      const reachable = domains.filter((item) => item.ok).length;
      notify(
        result.reachable ? "ok" : "error",
        result.reachable ? "上游代理可用" : "上游代理不可达",
        result.reachable
          ? `CONNECT ${result.latencyMs} ms${domains.length ? ` · ${reachable}/${domains.length} 个域名可连通` : ""}`
          : result.error,
      );
      await load(false);
      return result;
    } catch (error) {
      notify("error", "探测失败", errorText(error));
      return null;
    } finally {
      busy = "";
    }
  }

  async function switchUpstream(name: string) {
    loadSequence++;
    busy = "switch";
    try {
      snapshot = await AppService.SwitchUpstream(name);
      notify("ok", "已切换上游档案", name);
    } catch (error) {
      notify("error", "切换失败", errorText(error));
    } finally {
      busy = "";
    }
  }

  async function selectFastestUpstream(): Promise<boolean> {
    loadSequence++;
    busy = "switch";
    try {
      snapshot = await AppService.SelectFastestUpstream();
      const active = snapshot.settings.activeUpstream;
      notify("ok", "已选择最快上游", active || "");
      return true;
    } catch (error) {
      notify("error", "测速选优失败", errorText(error));
      return false;
    } finally {
      busy = "";
    }
  }

  function upsertRequest(request: RequestRecord) {
    if (!snapshot) return;
    const rows = snapshot.requests ?? [];
    const index = rows.findIndex((item) => item.id === request.id);
    const next = [...rows];
    if (index >= 0) next[index] = request;
    else next.unshift(request);
    snapshot.requests = next.slice(0, 300);
  }

  onMount(() => {
    activePage = (location.hash.slice(1) as Page) in titles ? (location.hash.slice(1) as Page) : "overview";
    void load(false);

    const offRequest = Events.On("lane:request", (event: { data: { event: string; request: RequestRecord } }) => {
      const payload = event?.data;
      if (!payload?.request) return;
      upsertRequest(payload.request);
      if (payload.event === "request:finished" && requestRefreshTimer === undefined) {
        requestRefreshTimer = window.setTimeout(() => {
          requestRefreshTimer = undefined;
          if (polling && !busy) void load(false);
        }, 450);
      }
    });
    const offEvent = Events.On("lane:event", (event: { data: { kind: string; title: string; detail: string } }) => {
      const payload = event?.data;
      if (!payload || !snapshot) return;
      const entry: Event = {
        time: new Date().toLocaleTimeString("zh-CN", { hour12: false }).slice(0, 5),
        kind: payload.kind,
        title: payload.title,
        detail: payload.detail,
      };
      snapshot.events = [entry, ...(snapshot.events ?? [])].slice(0, 30);
      if (payload.kind === "error") notify("error", payload.title, payload.detail);
    });
    const offUpdate = Events.On("lane:update", (event: { data: UpdateStatus }) => {
      const status = event?.data;
      if (!status || !snapshot) return;
      snapshot.update = status;
      if (status.status === "ready" && status.latestVersion !== readyUpdateNotified) {
        readyUpdateNotified = status.latestVersion;
        notify("ok", `更新 v${status.latestVersion} 已就绪`, "可在设置页的「关于」中重启更新");
      }
    });

    const timer = window.setInterval(() => {
      if (polling && !busy) load(false);
    }, 7000);

    return () => {
      polling = false;
      window.clearInterval(timer);
      if (requestRefreshTimer !== undefined) window.clearTimeout(requestRefreshTimer);
      if (typeof offRequest === "function") offRequest();
      if (typeof offEvent === "function") offEvent();
      if (typeof offUpdate === "function") offUpdate();
    };
  });

  const runStateClass = $derived(snapshot?.lane.error ? "error" : snapshot?.lane.running ? "" : "off");
  const runStateLabel = $derived(snapshot?.lane.error ? "通道异常" : snapshot?.lane.running ? "通道运行中" : "通道已停止");
</script>

{#if loading && !snapshot}
  <div class="loading-screen">
    <div class="loading-card"><Icon name="refresh" size={22} /><span>正在读取 Codex、代理与 WSL 状态…</span></div>
  </div>
{:else if loadError && !snapshot}
  <div class="loading-screen">
    <div class="loading-card" style="display:block;max-width:520px">
      <div style="display:flex;gap:10px;align-items:center;color:var(--red)"><Icon name="alert" size={20} /><b>Codex Lane 无法启动</b></div>
      <p style="color:var(--text-2);font-size:12px;line-height:1.6">{loadError}</p>
      <button class="btn" onclick={() => load(true)}>重试</button>
    </div>
  </div>
{:else if snapshot}
  <div class="window">
    <header class="appbar">
      <a class="brand" href="#overview" aria-label="Codex Lane 首页" onclick={(event) => { event.preventDefault(); navigate("overview"); }}>
        <span class="brand-mark"><Icon name="mark" size={25} /></span>
        <span class="brand-name">Codex <span>Lane</span></span>
        <span class="brand-edition">LOCAL</span>
      </a>

      <nav class="nav" aria-label="主导航">
        {#each pages as page}
          <button class:active={activePage === page.id} onclick={() => navigate(page.id)}>{page.label}</button>
        {/each}
      </nav>

      <div class="app-actions">
        {#if snapshot.update?.available}
          <button class="update-badge" title={`发现新版本 v${snapshot.update.latestVersion}`} onclick={openAbout}>
            <Icon name={snapshot.update.status === "ready" ? "power" : "download"} size={14} />
            <span>{snapshot.update.status === "ready" ? "更新就绪" : snapshot.update.status === "downloading" ? "正在下载" : "更新"} <b>v{snapshot.update.latestVersion}</b></span>
          </button>
        {/if}
        <span class={`run-state ${runStateClass}`}><i class="live-dot"></i><span>{runStateLabel}</span></span>
        <button class="icon-button" class:spin={busy === "refresh"} aria-label="刷新状态" title="刷新状态" onclick={refresh}>
          <Icon name="refresh" size={18} />
        </button>
        <button class="icon-button" aria-label="打开设置" title="设置" onclick={() => navigate("settings")}>
          <Icon name="settings" size={18} />
        </button>
      </div>
    </header>

    <main class="main">
      {#if activePage === "overview"}
        <Overview
          {snapshot}
          {busy}
          onLaunch={() => launch(false)}
          onLaneToggle={toggleLane}
          onNavigate={navigate}
          onDiagnostics={openChecks}
          onRunTests={runTests}
        />
      {:else if activePage === "requests"}
        <Requests {snapshot} {busy} onClear={clearRequests} onExport={exportReport} />
      {:else if activePage === "checks"}
        <Checks {snapshot} {busy} onRun={runTests} onExport={exportSummary} />
      {:else if activePage === "settings"}
        <Settings
          {snapshot}
          {busy}
          onSave={saveSettings}
          onProbe={probe}
          onCheckUpdate={checkForUpdates}
          onDownloadUpdate={downloadUpdate}
          onRestartUpdate={restartUpdate}
          onOpenProject={openProjectPage}
          onOpenConfigFolder={openConfigFolder}
          onSwitchUpstream={switchUpstream}
          onSelectFastest={selectFastestUpstream}
        />
      {/if}
    </main>
  </div>

  <div class="toast-wrap" aria-live="polite">
    {#each toasts as toast}
      <div class={`toast ${toast.kind}`}>
        <Icon name={toast.kind === "ok" ? "checkCircle" : toast.kind === "error" ? "alert" : "activity"} size={17} />
        <div><b>{toast.title}</b>{toast.detail}</div>
      </div>
    {/each}
  </div>

  {#if modal}
    <div class="modal-backdrop" role="presentation" onclick={() => (modal = null)}>
      <div
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="lane-modal-title"
        tabindex="-1"
        onclick={(event) => event.stopPropagation()}
        onkeydown={(event) => { if (event.key === "Escape") modal = null; }}
      >
        <div class="modal-head"><h2 id="lane-modal-title">{modal.title}</h2><p>{modal.body}</p></div>
        {#if modal.note}<div class="modal-body">{modal.note}</div>{/if}
        <div class="modal-actions">
          <button class="btn" onclick={() => (modal = null)}>取消</button>
          <button class:accent={!modal.danger} class:dark={modal.danger} class="btn" onclick={async () => { const action = modal?.action; modal = null; await action?.(); }}>
            {modal.confirm}
          </button>
        </div>
      </div>
    </div>
  {/if}
{/if}
