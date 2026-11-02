package codex

import "testing"

func TestProxyServer(t *testing.T) {
	cases := map[string]string{
		`"C:\app\ChatGPT.exe" --proxy-server=http://127.0.0.1:5950 --disable-quic`: "http://127.0.0.1:5950",
		`"C:\app\ChatGPT.exe" --proxy-server=http://127.0.0.1:7897`:                "http://127.0.0.1:7897",
		`"C:\app\ChatGPT.exe" --type=gpu-process`:                                  "",
		``: "",
	}
	for commandLine, want := range cases {
		if got := ProxyServer(commandLine); got != want {
			t.Fatalf("ProxyServer(%q) = %q, want %q", commandLine, got, want)
		}
	}
}
