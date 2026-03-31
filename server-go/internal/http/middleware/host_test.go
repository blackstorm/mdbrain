package middleware

import "testing"

func TestParseHostDomain(t *testing.T) {
	tests := []struct {
		host   string
		domain string
		err    string
	}{
		{"example.com", "example.com", ""},
		{"example.com:8080", "example.com", ""},
		{"localhost:3000", "localhost", ""},
		{"[::1]:8080", "::1", ""},
		{"", "", "missing"},
		{"bad host", "", "invalid"},
		{"bad/host", "", "invalid"},
	}

	for _, tc := range tests {
		got := ParseHostDomain(tc.host)
		if got.Domain != tc.domain || got.Error != tc.err {
			t.Fatalf("host %q -> %#v, expected domain=%q err=%q", tc.host, got, tc.domain, tc.err)
		}
	}
}
