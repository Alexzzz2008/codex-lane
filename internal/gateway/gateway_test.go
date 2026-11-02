package gateway

import "testing"

func TestParseTarget(t *testing.T) {
	tests := []struct {
		authority string
		def       int
		host      string
		port      int
		wantErr   bool
	}{
		{"api.openai.com:443", 443, "api.openai.com", 443, false},
		{"example.com", 80, "example.com", 80, false},
		{"[::1]:7897", 443, "::1", 7897, false},
		{"", 443, "", 0, true},
		{"example.com:99999", 443, "", 0, true},
	}
	for _, tt := range tests {
		host, _, port, err := ParseTarget(tt.authority, tt.def)
		if (err != nil) != tt.wantErr {
			t.Fatalf("%s: err=%v", tt.authority, err)
		}
		if host != tt.host || port != tt.port {
			t.Fatalf("%s: got %s:%d", tt.authority, host, port)
		}
	}
}
