package codex

import (
	"strings"
	"testing"

	"github.com/Alexzzz2008/codex-lane/internal/model"
)

func TestProxyEnvironmentReplacesInheritedProxy(t *testing.T) {
	base := []string{"Path=C:\\Windows", "http_proxy=http://old:1234", "Https_Proxy=http://old:1234", "NO_PROXY=*", "WSLENV=KEEP/u"}
	for _, engine := range []string{model.EngineWindows, model.EngineWSL} {
		t.Run(engine, func(t *testing.T) {
			result := proxyEnvironment(base, engine, "http://127.0.0.1:54000", "localhost,127.0.0.1,::1")
			seen := map[string]string{}
			for _, item := range result {
				name, value, _ := strings.Cut(item, "=")
				key := strings.ToUpper(name)
				if _, duplicate := seen[key]; duplicate {
					t.Fatalf("duplicate environment variable %s", key)
				}
				seen[key] = value
			}
			for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
				if seen[name] != "http://127.0.0.1:54000" {
					t.Fatalf("%s did not use Lane", name)
				}
			}
			if seen["NO_PROXY"] != "localhost,127.0.0.1,::1" || seen["PATH"] != "C:\\Windows" {
				t.Fatal("loopback bypass or inherited environment was lost")
			}
			if engine == model.EngineWindows && seen["WSLENV"] != "KEEP/u" {
				t.Fatal("Windows mode changed WSLENV")
			}
		})
	}
	if base[1] != "http_proxy=http://old:1234" {
		t.Fatal("parent environment was mutated")
	}
}

func TestWSLProxyVariablesUseDirectionalFlags(t *testing.T) {
	result := proxyEnvironment([]string{"WSLENV=KEEP/p:HTTP_PROXY/p:http_proxy/lw:OTHER/u"}, model.EngineWSL, "http://127.0.0.1:54000", "localhost")
	for _, item := range result {
		value, ok := strings.CutPrefix(item, "WSLENV=")
		if !ok {
			continue
		}
		entries := strings.Split(value, ":")
		if len(entries) != 6 || entries[0] != "KEEP/p" || entries[1] != "OTHER/u" {
			t.Fatalf("inherited flags were not preserved: %s", value)
		}
		for _, name := range []string{"http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
			found := false
			for _, entry := range entries {
				if entry == name+"/u" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing Win32-to-WSL forwarding for %s", name)
			}
		}
		return
	}
	t.Fatal("WSLENV was not set")
}
