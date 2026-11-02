package appservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestCheckForUpdatesAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/atom+xml")
		_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <updated>2026-11-02T10:00:00Z</updated>
    <link rel="alternate" href="https://github.com/Alexzzz2008/codex-lane/releases/tag/v0.2.0"/>
  </entry>
</feed>`))
	}))
	defer server.Close()

	status := checkForUpdates(context.Background(), "0.1.0", server.URL)
	if !status.Available || status.LatestVersion != "0.2.0" || status.Status != "available" {
		t.Fatalf("unexpected update status: %+v", status)
	}
	if status.ReleaseURL == "" || status.PublishedAt == "" {
		t.Fatalf("missing release metadata: %+v", status)
	}
}

func TestCheckForUpdatesCurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <updated>2026-11-02T10:00:00Z</updated>
    <link rel="alternate" href="https://github.com/Alexzzz2008/codex-lane/releases/tag/v0.1.0"/>
  </entry>
</feed>`))
	}))
	defer server.Close()

	status := checkForUpdates(context.Background(), "0.1.0", server.URL)
	if status.Available || status.Status != "current" {
		t.Fatalf("unexpected update status: %+v", status)
	}
}

func TestCheckForUpdatesUnavailable(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	status := checkForUpdates(context.Background(), "0.1.0", server.URL)
	if status.Available || status.Status != "unavailable" {
		t.Fatalf("unexpected update status: %+v", status)
	}
}
