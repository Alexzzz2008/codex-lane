package appservice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/config"
	"github.com/Alexzzz2008/codex-lane/internal/model"
	"github.com/Alexzzz2008/codex-lane/internal/selfupdate"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{"0.2.0", "0.1.0", 1},
		{"v0.1.0", "0.1.0", 0},
		{"0.1.1", "0.1", 1},
		{"0.2.0-beta.1", "0.2.0", -1},
		{"1.0.0", "0.99.99", 1},
	}
	for _, test := range tests {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestVersionFromReleaseURL(t *testing.T) {
	tests := map[string]string{
		"https://github.com/Alexzzz2008/codex-lane/releases/tag/v0.2.0": "v0.2.0",
		"https://github.com/Alexzzz2008/codex-lane/releases/latest":     "",
		"not a url": "",
	}
	for value, want := range tests {
		if got := versionFromReleaseURL(value); got != want {
			t.Fatalf("versionFromReleaseURL(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestEvaluateManifestAvailable(t *testing.T) {
	manifest := selfupdate.Manifest{
		Version:    "0.1.31",
		ReleaseURL: "https://github.com/Alexzzz2008/codex-lane/releases/tag/v0.1.31",
		Assets: map[string]selfupdate.Asset{
			updateAssetKey: {
				URL:    "https://example.com/CodexLane.exe",
				Size:   1024,
				SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
	}
	status, asset := evaluateManifest(manifest, "0.1.30", "2026-11-03T10:00:00+08:00")
	if !status.Available || status.Status != "available" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if asset.Size != 1024 || status.Total != 1024 {
		t.Fatalf("unexpected asset size: %+v", status)
	}
}

func TestEvaluateManifestCurrent(t *testing.T) {
	status, _ := evaluateManifest(selfupdate.Manifest{Version: "0.1.30"}, "0.1.30", "")
	if status.Available || status.Status != "current" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestEvaluateManifestRejectsMissingAsset(t *testing.T) {
	status, _ := evaluateManifest(selfupdate.Manifest{Version: "0.1.31"}, "0.1.30", "")
	if !status.Available || status.Status != "error" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestCheckForUpdatesDownloadsAndVerifies(t *testing.T) {
	body := []byte("fake update payload")
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest.json":
			_ = json.NewEncoder(writer).Encode(selfupdate.Manifest{
				Version:    "99.0.0",
				ReleaseURL: "https://example.com/releases/v99.0.0",
				Assets: map[string]selfupdate.Asset{
					updateAssetKey: {
						URL:    "http://" + request.Host + "/CodexLane.exe",
						Size:   int64(len(body)),
						SHA256: hex.EncodeToString(sum[:]),
					},
				},
			})
		case "/CodexLane.exe":
			_, _ = writer.Write(body)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("CODEXLANE_UPDATE_FEED", server.URL+"/latest.json")

	service := New()
	service.paths.LocalAppData = t.TempDir()
	cfg := config.Default()
	cfg.Upstream.Host = ""
	cfg.Upstream.Port = 0
	service.cfg = cfg

	status := service.CheckForUpdates()
	if !status.Available || status.LatestVersion != "99.0.0" {
		t.Fatalf("unexpected update status: %+v", status)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status = service.currentUpdateStatus()
		if status.Status == "ready" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if status.Status != "ready" {
		t.Fatalf("update did not become ready: %+v", status)
	}
	file := service.stagedUpdate
	if file == "" {
		t.Fatal("no staged update path")
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != string(body) {
		t.Fatalf("staged update = %q, err = %v", got, err)
	}
	restarted := New()
	restarted.paths = service.paths
	restarted.restoreUpdateCache()
	if restarted.currentUpdateStatus().Status != "ready" || restarted.stagedUpdate != file {
		t.Fatal("verified update was not restored after restart")
	}
	if err := os.WriteFile(file, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if service.updateReadyFor("99.0.0") {
		t.Fatal("accepted corrupt cached update")
	}
}

func TestOfflineUpdateCheckPreservesDownloadedVersion(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	t.Setenv("CODEXLANE_UPDATE_FEED", server.URL)
	service := New()
	service.cfg = config.Default()
	service.cfg.Upstream.Host = ""
	service.stagedVersion = "99.0.0"
	service.stagedUpdate = filepath.Join(t.TempDir(), "update.exe")
	if err := os.WriteFile(service.stagedUpdate, []byte("verified update"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("verified update"))
	service.updateManifest = selfupdate.Manifest{Version: "99.0.0", Assets: map[string]selfupdate.Asset{updateAssetKey: {URL: server.URL, Size: int64(len("verified update")), SHA256: hex.EncodeToString(sum[:])}}}
	service.updateStatus = model.UpdateStatus{Status: "ready", Available: true, LatestVersion: "99.0.0"}
	status := service.CheckForUpdates()
	if status.Status != "ready" || !status.Available || status.LatestVersion != "99.0.0" || status.Error == "" {
		t.Fatalf("downloaded update was lost: %+v", status)
	}
}

func TestSameVersionUpdateReplacementIsDownloadedAgain(t *testing.T) {
	var mu sync.RWMutex
	body := []byte("original update")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		payload := append([]byte(nil), body...)
		mu.RUnlock()
		if r.URL.Path == "/update.exe" {
			_, _ = w.Write(payload)
			return
		}
		sum := sha256.Sum256(payload)
		_ = json.NewEncoder(w).Encode(selfupdate.Manifest{Version: "99.0.0", Assets: map[string]selfupdate.Asset{
			updateAssetKey: {URL: "http://" + r.Host + "/update.exe", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])},
		}})
	}))
	defer server.Close()
	t.Setenv("CODEXLANE_UPDATE_FEED", server.URL+"/latest.json")
	service := New()
	service.paths.LocalAppData = t.TempDir()
	service.cfg = config.Default()
	service.cfg.Upstream.Host = ""
	waitDownload := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for service.updateDownloadRunning.Load() && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if service.updateDownloadRunning.Load() || service.currentUpdateStatus().Status != "ready" {
			t.Fatalf("update not ready: %+v", service.currentUpdateStatus())
		}
	}
	service.CheckForUpdates()
	waitDownload()
	mu.Lock()
	body = []byte("replacement update")
	mu.Unlock()
	service.CheckForUpdates()
	waitDownload()
	if !service.updateReadyFor("99.0.0") {
		t.Fatal("same-version replacement was marked ready using the previous asset checksum")
	}
	data, err := os.ReadFile(service.stagedUpdate)
	if err != nil || string(data) != "replacement update" {
		t.Fatalf("staged update = %q, err = %v", data, err)
	}
}

func TestInvalidManifestPreservesVerifiedUpdate(t *testing.T) {
	for _, test := range []struct {
		name     string
		manifest selfupdate.Manifest
	}{
		{"version", selfupdate.Manifest{Version: "invalid"}},
		{"missing asset", selfupdate.Manifest{Version: "99.0.1"}},
		{"invalid size", selfupdate.Manifest{Version: "99.0.1", Assets: map[string]selfupdate.Asset{updateAssetKey: {URL: "https://example.com/update.exe", SHA256: string(make([]byte, 64))}}}},
		{"invalid hash", selfupdate.Manifest{Version: "99.0.1", Assets: map[string]selfupdate.Asset{updateAssetKey: {URL: "https://example.com/update.exe", Size: 10, SHA256: "invalid"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(test.manifest) }))
			defer server.Close()
			t.Setenv("CODEXLANE_UPDATE_FEED", server.URL)
			service := New()
			service.cfg = config.Default()
			service.cfg.Upstream.Host = ""
			service.stagedVersion = "99.0.0"
			service.stagedUpdate = filepath.Join(t.TempDir(), "update.exe")
			body := []byte("verified update")
			sum := sha256.Sum256(body)
			if err := os.WriteFile(service.stagedUpdate, body, 0o600); err != nil {
				t.Fatal(err)
			}
			service.updateManifest = selfupdate.Manifest{Version: "99.0.0", Assets: map[string]selfupdate.Asset{updateAssetKey: {URL: server.URL, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}}}
			service.updateStatus = model.UpdateStatus{Status: "ready", Available: true, LatestVersion: "99.0.0"}
			status := service.CheckForUpdates()
			if status.Status != "ready" || !status.Available || status.LatestVersion != "99.0.0" || status.Error == "" || !service.updateReadyFor("99.0.0") {
				t.Fatalf("verified update was invalidated: %+v", status)
			}
		})
	}
}

func TestUpdateVersionsAreSafeAndOrdered(t *testing.T) {
	for _, value := range []string{"1.2.3-../../escape", "-1.2.3", "1.2.3/escape", "1.2.3-beta..1", "1.2"} {
		if isVersion(value) {
			t.Fatalf("accepted unsafe version %q", value)
		}
	}
	if !isVersion("1.2.3-beta.10+build.7") || compareVersions("1.2.3-beta.10", "1.2.3-beta.2") <= 0 || compareVersions("1.2.3-beta", "1.2.3") >= 0 {
		t.Fatal("incorrect prerelease version handling")
	}
}
