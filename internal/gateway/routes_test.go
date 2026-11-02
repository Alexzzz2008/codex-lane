package gateway

import "testing"

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern string
		host    string
		want    bool
	}{
		{"*.openai.com", "api.openai.com", true},
		{"*.openai.com", "openai.com", true},
		{"*.openai.com", "evil.openai.com.attacker.io", false},
		{"openai.com", "openai.com", true},
		{"openai.com", "api.openai.com", false},
		{"*", "anything.example", true},
	}
	for _, tc := range cases {
		if got := matchPattern(tc.pattern, tc.host); got != tc.want {
			t.Fatalf("matchPattern(%q, %q) = %v, want %v", tc.pattern, tc.host, got, tc.want)
		}
	}
}
