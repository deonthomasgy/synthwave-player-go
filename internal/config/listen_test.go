package config

import "testing"

func TestNormalizeListenHost(t *testing.T) {
	tests := map[string]string{
		"":          "0.0.0.0",
		"*":         "0.0.0.0",
		"0.0.0.0":   "0.0.0.0",
		"127.0.0.1": "127.0.0.1",
	}
	for in, want := range tests {
		if got := NormalizeListenHost(in); got != want {
			t.Errorf("NormalizeListenHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestListenAddr(t *testing.T) {
	if got := ListenAddr("0.0.0.0", 8160); got != "0.0.0.0:8160" {
		t.Fatalf("got %q", got)
	}
}
