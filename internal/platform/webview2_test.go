package platform

import "testing"

func TestValidWebView2Version(t *testing.T) {
	tests := map[string]bool{
		"":          false,
		"0.0.0.0":   false,
		"  ":        false,
		"120.0.0.0": true,
		" 121.0.1 ": true,
	}
	for value, want := range tests {
		if got := validWebView2Version(value); got != want {
			t.Fatalf("validWebView2Version(%q) = %v, want %v", value, got, want)
		}
	}
}
