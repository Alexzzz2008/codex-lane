package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(Manifest{
			Version:    "0.1.31",
			ReleaseURL: "https://example.com/release",
			Assets: map[string]Asset{
				"windows-amd64": {
					URL:    "https://example.com/CodexLane.exe",
					Size:   12,
					SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				},
			},
		})
	}))
	defer server.Close()

	manifest, err := FetchManifest(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.1.31" {
		t.Fatalf("version = %q", manifest.Version)
	}
	asset, err := manifest.Asset("windows-amd64")
	if err != nil || asset.Size != 12 {
		t.Fatalf("asset = %+v, err = %v", asset, err)
	}
}

func TestFetchManifestNotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	_, err := FetchManifest(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestDownloadChecksSHA256(t *testing.T) {
	body := []byte("new codex lane executable")
	sum := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(body)
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "CodexLane.exe")
	asset := Asset{
		URL:    server.URL,
		Size:   int64(len(body)),
		SHA256: hex.EncodeToString(sum[:]),
	}
	if err := Download(context.Background(), server.Client(), asset, destination, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("downloaded %q", got)
	}
}

func TestDownloadRejectsWrongSHA256(t *testing.T) {
	body := []byte("wrong build")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(body)
	}))
	defer server.Close()

	asset := Asset{
		URL:    server.URL,
		Size:   int64(len(body)),
		SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	err := Download(context.Background(), server.Client(), asset, filepath.Join(t.TempDir(), "CodexLane.exe"), nil)
	if err == nil {
		t.Fatal("expected a checksum error")
	}
}

func TestInstallAndRollback(t *testing.T) {
	previous := canStart
	canStart = func(string) error { return nil }
	t.Cleanup(func() { canStart = previous })

	dir := t.TempDir()
	executable := filepath.Join(dir, "CodexLane.exe")
	staged := filepath.Join(dir, "staged.exe")
	if err := os.WriteFile(executable, []byte("old version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new version"), 0o755); err != nil {
		t.Fatal(err)
	}

	old, err := Install(staged, executable)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(executable)
	if string(got) != "new version" {
		t.Fatalf("installed %q", got)
	}
	if err := Rollback(executable, old); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(executable)
	if string(got) != "old version" {
		t.Fatalf("rolled back to %q", got)
	}
}
