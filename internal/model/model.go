package model

const (
	EngineAuto    = "auto"
	EngineWindows = "windows"
	EngineWSL     = "wsl"

	ProtocolHTTP   = "http"
	ProtocolHTTPS  = "https"
	ProtocolSOCKS5 = "socks5"

	SourceInterface = "interface"
	SourceKernel    = "kernel"
	SourceSelfTest  = "self-test"

	HostFilterOff       = "off"
	HostFilterAllowlist = "allowlist"

	RouteProxy  = "proxy"
	RouteDirect = "direct"
)

type Upstream struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	NoProxy  string `json:"noProxy"`
}

type Lane struct {
	ListenHost     string      `json:"listenHost"`
	ListenPort     int         `json:"listenPort"`
	InterfacePort  int         `json:"interfacePort"`
	KernelPort     int         `json:"kernelPort"`
	HostFilterMode string      `json:"hostFilterMode"`
	AllowedHosts   []string    `json:"allowedHosts"`
	Routes         []RouteRule `json:"routes"`
}

type RouteRule struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`
}

type UpstreamProfile struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

type Startup struct {
	OpenCodex       bool `json:"openCodex"`
	StopCodexOnExit bool `json:"stopCodexOnExit"`
	MinimizeToTray  bool `json:"minimizeToTray"`
}

type Logging struct {
	RequestMetadata bool `json:"requestMetadata"`
	RetentionDays   int  `json:"retentionDays"`
	MaxMemoryRows   int  `json:"maxMemoryRows"`
}

type Config struct {
	SchemaVersion       int               `json:"schemaVersion"`
	Upstream            Upstream          `json:"upstreamProxy"`
	Upstreams           []UpstreamProfile `json:"upstreams"`
	ActiveUpstream      string            `json:"activeUpstream"`
	AutoFailover        bool              `json:"autoFailover"`
	Lane                Lane              `json:"lane"`
	Engine              string            `json:"engine"`
	WSLDistro           string            `json:"wslDistro"`
	AskBeforeKill       bool              `json:"askBeforeKill"`
	RequireHealthyProxy bool              `json:"requireHealthyProxy"`
	Startup             Startup           `json:"startup"`
	Logging             Logging           `json:"logging"`
}

type Settings struct {
	Protocol        string            `json:"protocol"`
	Host            string            `json:"host"`
	Port            int               `json:"port"`
	NoProxy         string            `json:"noProxy"`
	Engine          string            `json:"engine"`
	WSLDistro       string            `json:"wslDistro"`
	OpenCodex       bool              `json:"openCodex"`
	StopCodexOnExit bool              `json:"stopCodexOnExit"`
	RetentionDays   int               `json:"retentionDays"`
	MaxMemoryRows   int               `json:"maxMemoryRows"`
	Upstreams       []UpstreamProfile `json:"upstreams"`
	ActiveUpstream  string            `json:"activeUpstream"`
	AutoFailover    bool              `json:"autoFailover"`
	HostFilterMode  string            `json:"hostFilterMode"`
	AllowedHosts    []string          `json:"allowedHosts"`
	Routes          []RouteRule       `json:"routes"`
	MinimizeToTray  bool              `json:"minimizeToTray"`
	LaunchAtLogin   bool              `json:"launchAtLogin"`
}

type LaneStatus struct {
	Running       bool   `json:"running"`
	InterfaceURL  string `json:"interfaceUrl"`
	KernelURL     string `json:"kernelUrl"`
	InterfacePort int    `json:"interfacePort"`
	KernelPort    int    `json:"kernelPort"`
	StartedAt     string `json:"startedAt"`
	Error         string `json:"error"`
}

type UpstreamStatus struct {
	URL       string        `json:"url"`
	Reachable bool          `json:"reachable"`
	LatencyMs int64         `json:"latencyMs"`
	Error     string        `json:"error"`
	CheckedAt string        `json:"checkedAt"`
	Domains   []DomainProbe `json:"domains"`
}

type DomainProbe struct {
	Host      string `json:"host"`
	OK        bool   `json:"ok"`
	Optional  bool   `json:"optional"`
	LatencyMs int64  `json:"latencyMs"`
	Status    int    `json:"status"`
	TLS       string `json:"tls"`
	Error     string `json:"error"`
}

type CodexStatus struct {
	Installed       bool      `json:"installed"`
	PackageName     string    `json:"packageName"`
	Version         string    `json:"version"`
	InstallLocation string    `json:"installLocation"`
	ExecutablePath  string    `json:"executablePath"`
	Running         bool      `json:"running"`
	MainPID         int       `json:"mainPid"`
	CommandLine     string    `json:"commandLine"`
	ProxyURL        string    `json:"proxyUrl"`
	ProxyMatched    bool      `json:"proxyMatched"`
	Processes       []Process `json:"processes"`
	Error           string    `json:"error"`
}

type Process struct {
	PID         int    `json:"pid"`
	ParentPID   int    `json:"parentPid"`
	Name        string `json:"name"`
	Executable  string `json:"executable"`
	CommandLine string `json:"commandLine"`
	IsMain      bool   `json:"isMain"`
}

type WSLStatus struct {
	Available       bool     `json:"available"`
	Distros         []string `json:"distros"`
	Distro          string   `json:"distro"`
	NetworkMode     string   `json:"networkMode"`
	InteropOK       bool     `json:"interopOk"`
	KernelPath      string   `json:"kernelPath"`
	KernelFound     bool     `json:"kernelFound"`
	ConfigEngineWSL bool     `json:"configEngineWsl"`
	Error           string   `json:"error"`
}

type Environment struct {
	OSName        string `json:"osName"`
	OSVersion     string `json:"osVersion"`
	Arch          string `json:"arch"`
	UserHome      string `json:"userHome"`
	CodexHome     string `json:"codexHome"`
	ConfigPath    string `json:"configPath"`
	LauncherPath  string `json:"launcherPath"`
	LogDirectory  string `json:"logDirectory"`
	SystemProxyOn bool   `json:"systemProxyOn"`
	SystemProxy   string `json:"systemProxy"`
	Administrator bool   `json:"administrator"`
	GoroutineSafe bool   `json:"goroutineSafe"`
}

type ResolvedEngine struct {
	Mode       string `json:"mode"`
	Resolved   string `json:"resolved"`
	Source     string `json:"source"`
	ConfigPath string `json:"configPath"`
	KeyValue   bool   `json:"keyValue"`
	KeyPresent bool   `json:"keyPresent"`
	Error      string `json:"error"`
}

type RequestRecord struct {
	ID         string `json:"id"`
	Time       string `json:"time"`
	Source     string `json:"source"`
	Method     string `json:"method"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Scheme     string `json:"scheme"`
	Status     int    `json:"status"`
	Result     string `json:"result"`
	LatencyMs  int64  `json:"latencyMs"`
	DurationMs int64  `json:"durationMs"`
	Upstream   string `json:"upstream"`
	Process    string `json:"process"`
	BytesUp    int64  `json:"bytesUp"`
	BytesDown  int64  `json:"bytesDown"`
	Error      string `json:"error"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

type RequestStats struct {
	Total       int64   `json:"total"`
	Success     int64   `json:"success"`
	Failed      int64   `json:"failed"`
	BytesUp     int64   `json:"bytesUp"`
	BytesDown   int64   `json:"bytesDown"`
	MedianMs    int64   `json:"medianMs"`
	P95Ms       int64   `json:"p95Ms"`
	SuccessRate float64 `json:"successRate"`
}

type RequestSeriesPoint struct {
	Time      string `json:"time"`
	BytesDown int64  `json:"bytesDown"`
	LatencyMs int64  `json:"latencyMs"`
}

type HistoryPoint struct {
	Time      string `json:"time"`
	BytesUp   int64  `json:"bytesUp"`
	BytesDown int64  `json:"bytesDown"`
	Requests  int64  `json:"requests"`
	LatencyMs int64  `json:"latencyMs"`
}

type UpstreamProbeResult struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error"`
}

type TestResult struct {
	ID        string `json:"id"`
	Group     string `json:"group"`
	Name      string `json:"name"`
	Detail    string `json:"detail"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latencyMs"`
	Hint      string `json:"hint"`
}

type TestReport struct {
	StartedAt  string       `json:"startedAt"`
	FinishedAt string       `json:"finishedAt"`
	DurationMs int64        `json:"durationMs"`
	Score      int          `json:"score"`
	Healthy    bool         `json:"healthy"`
	Results    []TestResult `json:"results"`
}

type UpdateStatus struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	Status         string `json:"status"`
	ReleaseURL     string `json:"releaseUrl"`
	PublishedAt    string `json:"publishedAt"`
	CheckedAt      string `json:"checkedAt"`
	Notes          string `json:"notes"`
	Downloaded     int64  `json:"downloaded"`
	Total          int64  `json:"total"`
	Error          string `json:"error"`
}

type DiagnosticItem struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type DiagnosticReport struct {
	GeneratedAt string           `json:"generatedAt"`
	DurationMs  int64            `json:"durationMs"`
	Score       int              `json:"score"`
	Healthy     bool             `json:"healthy"`
	Items       []DiagnosticItem `json:"items"`
}

type Event struct {
	Time   string `json:"time"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type Snapshot struct {
	Version       string               `json:"version"`
	RepositoryURL string               `json:"repositoryUrl"`
	ReleasesURL   string               `json:"releasesUrl"`
	Settings      Settings             `json:"settings"`
	Lane          LaneStatus           `json:"lane"`
	Upstream      UpstreamStatus       `json:"upstream"`
	Codex         CodexStatus          `json:"codex"`
	Dependencies  []Dependency         `json:"dependencies"`
	Environment   Environment          `json:"environment"`
	Engine        ResolvedEngine       `json:"engine"`
	WSL           WSLStatus            `json:"wsl"`
	Stats         RequestStats         `json:"stats"`
	Requests      []RequestRecord      `json:"requests"`
	Series        []RequestSeriesPoint `json:"series"`
	TestReport    *TestReport          `json:"testReport"`
	Diagnostics   *DiagnosticReport    `json:"diagnostics"`
	Update        UpdateStatus         `json:"update"`
	Events        []Event              `json:"events"`
}

type Dependency struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Ready    bool   `json:"ready"`
	Detail   string `json:"detail"`
	Hint     string `json:"hint"`
	Action   string `json:"action"`
}

type SettingsInput struct {
	Protocol        string            `json:"protocol"`
	Host            string            `json:"host"`
	Port            int               `json:"port"`
	NoProxy         string            `json:"noProxy"`
	Engine          string            `json:"engine"`
	WSLDistro       string            `json:"wslDistro"`
	OpenCodex       bool              `json:"openCodex"`
	StopCodexOnExit bool              `json:"stopCodexOnExit"`
	RetentionDays   int               `json:"retentionDays"`
	MaxMemoryRows   int               `json:"maxMemoryRows"`
	Upstreams       []UpstreamProfile `json:"upstreams"`
	ActiveUpstream  string            `json:"activeUpstream"`
	AutoFailover    bool              `json:"autoFailover"`
	HostFilterMode  string            `json:"hostFilterMode"`
	AllowedHosts    []string          `json:"allowedHosts"`
	Routes          []RouteRule       `json:"routes"`
	MinimizeToTray  bool              `json:"minimizeToTray"`
	LaunchAtLogin   bool              `json:"launchAtLogin"`
}

type LaunchOptions struct {
	RestartExisting bool `json:"restartExisting"`
}

type LaunchResult struct {
	Started        bool   `json:"started"`
	AlreadyRunning bool   `json:"alreadyRunning"`
	NeedsRestart   bool   `json:"needsRestart"`
	NeedsInstall   bool   `json:"needsInstall"`
	PID            int    `json:"pid"`
	CommandLine    string `json:"commandLine"`
	Message        string `json:"message"`
}
