package appservice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Alexzzz2008/codex-lane/internal/config"
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
}
