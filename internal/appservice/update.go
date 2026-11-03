package appservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/platform"
	"github.com/Alexzzz2008/codex-lane/internal/selfupdate"
)

const (
	RepositoryURL     = "https://github.com/Alexzzz2008/codex-lane"
	ReleasesURL       = RepositoryURL + "/releases"
	updateFeedURL     = ReleasesURL + "/latest/download/latest.json"
	updateAssetKey    = "windows-amd64"
	updateCheckPeriod = 6 * time.Hour
	updateStartDelay  = 5 * time.Second
)

//wails:ignore
func (s *AppService) SetQuit(quit func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quitFn = quit
}

func (s *AppService) CheckForUpdates() model.UpdateStatus {
	s.checkForUpdates(true)
	return s.currentUpdateStatus()
}

func (s *AppService) DownloadUpdate() (model.UpdateStatus, error) {
	s.mu.RLock()
	manifest := s.updateManifest
	status := s.updateStatus
	s.mu.RUnlock()
	if status.Status == "ready" {
		return status, nil
	}
	if manifest.Version == "" || !status.Available {
		return status, errors.New("当前没有可下载的新版本")
	}
	if !s.updateDownloadRunning.CompareAndSwap(false, true) {
		return status, nil
	}
	go s.downloadUpdate(manifest)
	return s.currentUpdateStatus(), nil
}

func (s *AppService) RestartToUpdate() (model.UpdateStatus, error) {
	if !s.updateInstallRunning.CompareAndSwap(false, true) {
		return s.currentUpdateStatus(), errors.New("更新正在安装")
	}
	defer s.updateInstallRunning.Store(false)

	s.mu.RLock()
	staged := s.stagedUpdate
	version := s.stagedVersion
	quit := s.quitFn
	s.mu.RUnlock()
	if staged == "" || version == "" {
		return s.currentUpdateStatus(), errors.New("更新尚未下载完成")
	}
	if quit == nil {
		return s.currentUpdateStatus(), errors.New("当前无法重启应用")
	}

	status := s.currentUpdateStatus()
	status.Status = "installing"
	status.Error = ""
	s.setUpdateStatus(status)

	executable, err := selfupdate.Executable()
	if err != nil {
		return s.failUpdate("无法定位当前程序", err)
	}
	old, err := selfupdate.Install(staged, executable)
	if err != nil {
		platform.AppLog("更新安装失败：%v", err)
		return s.failUpdate("替换程序失败", err)
	}
	platform.AppLog("更新已安装 v%s，旧版本 %s", version, old)
	s.mu.Lock()
	s.stagedUpdate = ""
	s.stagedVersion = ""
	s.mu.Unlock()

	marker := selfupdate.MarkerPath()
	replacement, err := selfupdate.StartReplacement(executable, marker)
	if err != nil {
		platform.AppLog("启动新版本失败：%v", err)
		_ = selfupdate.Rollback(executable, old)
		return s.failUpdate("新版本启动失败，已回滚", err)
	}
	if err := replacement.WaitStarted(marker, 20*time.Second); err != nil {
		platform.AppLog("新版本未报告启动：%v", err)
		replacement.Kill()
		_ = selfupdate.Rollback(executable, old)
		return s.failUpdate("新版本未能启动，已回滚", err)
	}
	_ = os.Remove(marker)
	platform.AppLog("新版本已启动，准备退出旧实例")

	status = s.currentUpdateStatus()
	status.Status = "restarting"
	status.Error = ""
	s.setUpdateStatus(status)
	s.addEvent("info", "正在重启并更新", "Codex 不会被结束，通道会短暂中断")
	go func() {
		time.Sleep(350 * time.Millisecond)
		platform.AppLog("退出旧实例以完成更新")
		quit()
	}()
	return status, nil
}

func (s *AppService) failUpdate(message string, err error) (model.UpdateStatus, error) {
	status := s.currentUpdateStatus()
	status.Status = "error"
	status.Error = message + "：" + err.Error()
	s.setUpdateStatus(status)
	s.addEvent("error", message, err.Error())
	return status, err
}

func (s *AppService) startUpdateLoop() {
	if s.ctx == nil || s.selfTest {
		return
	}
	go func() {
		timer := time.NewTimer(updateStartDelay)
		defer timer.Stop()
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			s.checkForUpdates(false)
		}
		ticker := time.NewTicker(updateCheckPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.checkForUpdates(false)
			}
		}
	}()
}

func (s *AppService) checkForUpdates(manual bool) {
	if !s.updateCheckRunning.CompareAndSwap(false, true) {
		return
	}
	defer s.updateCheckRunning.Store(false)
	if s.updateDownloadRunning.Load() || s.updateInstallRunning.Load() {
		return
	}

	previous := s.currentUpdateStatus()
	checking := previous
	checking.Status = "checking"
	checking.Error = ""
	checking.CheckedAt = time.Now().Format(time.RFC3339)
	s.setUpdateStatus(checking)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := s.currentConfig()
	client := updateClient(cfg, 20*time.Second)
	manifest, err := selfupdate.FetchManifest(ctx, client, updateFeed())
	if err != nil {
		status := model.UpdateStatus{
			CurrentVersion: Version,
			Status:         "error",
			CheckedAt:      checking.CheckedAt,
			Error:          "无法检查更新：" + err.Error(),
		}
		var httpErr *selfupdate.HTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.StatusCode {
			case http.StatusNotFound:
				status.Status = "unavailable"
				status.Error = "仓库未公开或暂时没有 Release"
			case http.StatusForbidden:
				status.Error = "GitHub 暂时限制了更新检查"
			}
		}
		s.setUpdateStatus(status)
		if manual && status.Status == "error" {
			s.addEvent("error", "更新检查失败", status.Error)
		}
		return
	}
	if !isVersion(manifest.Version) {
		s.setUpdateStatus(model.UpdateStatus{
			CurrentVersion: Version,
			Status:         "error",
			CheckedAt:      checking.CheckedAt,
			Error:          "无法识别最新版本号",
		})
		return
	}

	status, asset := evaluateManifest(manifest, Version, checking.CheckedAt)
	if compareVersions(manifest.Version, Version) <= 0 {
		s.setUpdateStatus(status)
		s.clearStaleUpdate(manifest.Version)
		return
	}
	if status.Status == "available" && s.updateReadyFor(manifest.Version) {
		status.Status = "ready"
		status.Downloaded = asset.Size
	}
	s.mu.Lock()
	s.updateManifest = manifest
	s.mu.Unlock()
	s.setUpdateStatus(status)
	if manual && status.Status == "available" {
		s.addEvent("info", "发现新版本", "v"+manifest.Version+" 正在后台下载")
	}
	if status.Status == "available" {
		if s.updateDownloadRunning.CompareAndSwap(false, true) {
			go s.downloadUpdate(manifest)
		}
	}
}

func evaluateManifest(manifest selfupdate.Manifest, currentVersion, checkedAt string) (model.UpdateStatus, selfupdate.Asset) {
	status := model.UpdateStatus{
		CurrentVersion: currentVersion,
		LatestVersion:  manifest.Version,
		ReleaseURL:     manifest.ReleaseURL,
		PublishedAt:    manifest.PublishedAt,
		Notes:          manifest.Notes,
		CheckedAt:      checkedAt,
	}
	if compareVersions(manifest.Version, currentVersion) <= 0 {
		status.Status = "current"
		return status, selfupdate.Asset{}
	}
	status.Available = true
	asset, err := manifest.Asset(updateAssetKey)
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		return status, selfupdate.Asset{}
	}
	status.Status = "available"
	status.Total = asset.Size
	return status, asset
}

func (s *AppService) downloadUpdate(manifest selfupdate.Manifest) {
	defer s.updateDownloadRunning.Store(false)
	asset, err := manifest.Asset(updateAssetKey)
	if err != nil {
		s.failDownload(manifest.Version, err)
		return
	}
	paths := s.currentPaths()
	if paths.LocalAppData == "" {
		s.failDownload(manifest.Version, errors.New("无法定位更新目录"))
		return
	}
	destination := filepath.Join(paths.LocalAppData, "CodexLane", "updates",
		fmt.Sprintf("CodexLane-%s-windows-amd64.exe", manifest.Version))

	status := s.currentUpdateStatus()
	status.Status = "downloading"
	status.Available = true
	status.LatestVersion = manifest.Version
	status.Downloaded = 0
	status.Total = asset.Size
	status.Error = ""
	s.setUpdateStatus(status)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := updateClient(s.currentConfig(), 10*time.Minute)
	err = selfupdate.Download(ctx, client, asset, destination, func(downloaded, total int64) {
		current := s.currentUpdateStatus()
		current.Status = "downloading"
		current.Downloaded = downloaded
		if total > 0 {
			current.Total = total
		}
		s.setUpdateStatus(current)
	})
	if err != nil {
		s.failDownload(manifest.Version, err)
		return
	}

	s.mu.Lock()
	previousStaged := s.stagedUpdate
	s.stagedUpdate = destination
	s.stagedVersion = manifest.Version
	s.updateManifest = manifest
	s.mu.Unlock()
	if previousStaged != "" && previousStaged != destination {
		_ = os.Remove(previousStaged)
	}
	status = s.currentUpdateStatus()
	status.Status = "ready"
	status.Available = true
	status.Downloaded = asset.Size
	status.Total = asset.Size
	status.Error = ""
	s.setUpdateStatus(status)
	s.addEvent("ok", "更新已下载", "v"+manifest.Version+" 已通过 SHA-256 校验，可在「关于」中重启更新")
}

func (s *AppService) failDownload(version string, err error) {
	status := s.currentUpdateStatus()
	status.Status = "error"
	status.LatestVersion = version
	status.Available = true
	status.Error = "更新下载失败：" + err.Error()
	s.setUpdateStatus(status)
	s.addEvent("error", "更新下载失败", err.Error())
}

func (s *AppService) updateReadyFor(version string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.stagedVersion != version || s.stagedUpdate == "" {
		return false
	}
	_, err := os.Stat(s.stagedUpdate)
	return err == nil
}

func (s *AppService) clearStaleUpdate(version string) {
	s.mu.Lock()
	staged := s.stagedUpdate
	stagedVersion := s.stagedVersion
	if stagedVersion != "" && compareVersions(stagedVersion, version) <= 0 {
		s.stagedUpdate = ""
		s.stagedVersion = ""
	}
	s.mu.Unlock()
	if staged != "" && stagedVersion != "" && compareVersions(stagedVersion, version) <= 0 {
		_ = os.Remove(staged)
	}
}

func (s *AppService) installUpdateOnExit() {
	s.mu.RLock()
	staged := s.stagedUpdate
	s.mu.RUnlock()
	if staged == "" {
		return
	}
	executable, err := selfupdate.Executable()
	if err != nil {
		log.Printf("update on exit: %v", err)
		return
	}
	if _, err := selfupdate.Install(staged, executable); err != nil {
		log.Printf("update on exit: %v", err)
		return
	}
	s.mu.Lock()
	s.stagedUpdate = ""
	s.stagedVersion = ""
	s.mu.Unlock()
}

func (s *AppService) currentUpdateStatus() model.UpdateStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updateStatus
}

func (s *AppService) setUpdateStatus(status model.UpdateStatus) {
	s.mu.Lock()
	s.updateStatus = status
	s.mu.Unlock()
	s.emit("lane:update", status)
}

func updateFeed() string {
	if override := strings.TrimSpace(os.Getenv("CODEXLANE_UPDATE_FEED")); override != "" {
		return override
	}
	return updateFeedURL
}

func updateClient(cfg model.Config, timeout time.Duration) *http.Client {
	transport := &http.Transport{}
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = base.Clone()
	}
	protocol := strings.ToLower(strings.TrimSpace(cfg.Upstream.Protocol))
	if protocol == "" {
		protocol = model.ProtocolHTTP
	}
	address := net.JoinHostPort(strings.TrimSpace(cfg.Upstream.Host), strconv.Itoa(cfg.Upstream.Port))
	if cfg.Upstream.Host != "" && cfg.Upstream.Port > 0 {
		transport.Proxy = http.ProxyURL(&url.URL{Scheme: protocol, Host: address})
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func versionFromReleaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] != "tag" {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func isVersion(value string) bool {
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(value), "v"), "-", 2)
	core := strings.Split(parts[0], ".")
	if len(core) == 0 {
		return false
	}
	for _, part := range core {
		if part == "" {
			return false
		}
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

// CompareVersions reports whether left is newer than right.
func CompareVersions(left, right string) int {
	return compareVersions(left, right)
}

func compareVersions(left, right string) int {
	leftCore, leftPre := versionParts(left)
	rightCore, rightPre := versionParts(right)
	length := max(len(leftCore), len(rightCore))
	for index := 0; index < length; index++ {
		var a, b int
		if index < len(leftCore) {
			a = leftCore[index]
		}
		if index < len(rightCore) {
			b = rightCore[index]
		}
		if a > b {
			return 1
		}
		if a < b {
			return -1
		}
	}
	switch {
	case leftPre == "" && rightPre != "":
		return 1
	case leftPre != "" && rightPre == "":
		return -1
	case leftPre > rightPre:
		return 1
	case leftPre < rightPre:
		return -1
	default:
		return 0
	}
}

func versionParts(value string) ([]int, string) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	value = strings.SplitN(value, "+", 2)[0]
	core := value
	pre := ""
	if index := strings.IndexByte(value, '-'); index >= 0 {
		core = value[:index]
		pre = value[index+1:]
	}
	parts := strings.Split(core, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			number = 0
		}
		numbers = append(numbers, number)
	}
	return numbers, pre
}
