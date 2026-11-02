<script lang="ts">
  let { name, size = 18, stroke = 1.7 }: { name: string; size?: number; stroke?: number } = $props();

  const icons: Record<string, string[]> = {
    mark: [
      "M6 18V8.5L12 5l6 3.5V18",
      "M8.8 18v-6.2L12 9.8l3.2 2V18",
      "M12 14.9a1.35 1.35 0 1 0 0-2.7 1.35 1.35 0 0 0 0 2.7Z",
    ],
    refresh: ["M20 11a8 8 0 1 0-2.34 5.66M20 4v7h-7"],
    settings: [
      "M12 15.25a3.25 3.25 0 1 0 0-6.5 3.25 3.25 0 0 0 0 6.5Z",
      "M19.4 13.5a7.8 7.8 0 0 0 0-3l1.45-1.12-1.9-3.28-1.72.69a7.8 7.8 0 0 0-2.6-1.5L14.36 3h-3.8l-.27 2.29a7.8 7.8 0 0 0-2.6 1.5l-1.72-.69-1.9 3.28L5.52 10.5a7.8 7.8 0 0 0 0 3l-1.45 1.12 1.9 3.28 1.72-.69a7.8 7.8 0 0 0 2.6 1.5l.27 2.29h3.8l.27-2.29a7.8 7.8 0 0 0 2.6-1.5l1.72.69 1.9-3.28-1.45-1.12Z",
    ],
    monitor: [
      "M4 5h16v12H4z",
      "M8 21h8M12 17v4",
      "m9.5 9 2.5 1.8L14.5 9",
    ],
    shield: [
      "m4.5 6 7.5-3 7.5 3v6.3c0 4.4-3.1 7.9-7.5 9.2-4.4-1.3-7.5-4.8-7.5-9.2V6Z",
      "M8 11.8h8M8 15h5",
    ],
    globe: [
      "M12 20a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z",
      "M4.5 9.5h15M4.5 14.5h15M12 4c2.2 2.1 3.3 4.7 3.3 8s-1.1 5.9-3.3 8c-2.2-2.1-3.3-4.7-3.3-8S9.8 6.1 12 4Z",
    ],
    route: ["M4 18V8m5 10V4m5 14v-7m5 7V6"],
    download: ["M12 4v12m0 0 4-4m-4 4-4-4M5 20h14"],
    upload: ["M12 16V4m0 0 4 4m-4-4L8 8M5 20h14"],
    clock: ["M12 20a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z", "M12 7v5l3 2"],
    check: ["m6 12 4 4 8-8"],
    checkCircle: ["M12 20a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z", "m8 12 2.6 2.6L16 9"],
    minus: ["M5 12h14"],
    play: ["m8 5 11 7-11 7V5Z"],
    pause: ["M9 5v14M15 5v14"],
    search: ["M10.5 16.5a6 6 0 1 0 0-12 6 6 0 0 0 0 12Z", "m15 15 4 4"],
    filter: ["M4 6h16M7 12h10M10 18h4"],
    folder: ["M3 7h7l2 2h9v9H3V7Z"],
    file: ["M6 3h8l4 4v14H6V3Z", "M14 3v5h4"],
    code: ["m8 8-4 4 4 4M16 8l4 4-4 4M14 5l-4 14"],
    terminal: ["M4 6h16v12H4z", "m7 10 2.5 2L7 14M13 14h4"],
    activity: ["M4 12h4l2-5 4 10 2-5h4"],
    user: ["M12 11a3.2 3.2 0 1 0 0-6.4A3.2 3.2 0 0 0 12 11Z", "M5 20c.7-4 3-6 7-6s6.3 2 7 6"],
    database: ["M5 6c0-1.7 3.1-3 7-3s7 1.3 7 3-3.1 3-7 3-7-1.3-7-3Z", "M5 6v6c0 1.7 3.1 3 7 3s7-1.3 7-3V6M5 12v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6"],
    copy: ["M8 8h10v12H8z", "M6 16H4V4h10v2"],
    external: ["M14 5h5v5M19 5l-8 8", "M18 14v5H5V6h5"],
    alert: ["M12 4 3.8 19h16.4L12 4Z", "M12 9v4M12 16h.01"],
    x: ["M6 6l12 12M18 6 6 18"],
    plus: ["M12 5v14M5 12h14"],
    chevron: ["m7 10 5 5 5-5"],
    layers: ["m12 4 8 4-8 4-8-4 8-4Z", "m4 12 8 4 8-4M4 16l8 4 8-4"],
  };
</script>

<svg viewBox="0 0 24 24" width={size} height={size} fill="none" aria-hidden="true">
  {#each icons[name] ?? icons.alert as path, index}
    <path
      d={path}
      fill={name === "mark" && index === 2 ? "#ffffff" : "none"}
      stroke={name === "mark" && index === 2 ? "none" : "currentColor"}
      stroke-width={name === "mark" ? 1.4 : stroke}
      stroke-linecap="round"
      stroke-linejoin="round"
      transform={name === "mark" ? "translate(12 11.5) scale(1.35) translate(-12 -11.5)" : undefined}
    />
  {/each}
</svg>

