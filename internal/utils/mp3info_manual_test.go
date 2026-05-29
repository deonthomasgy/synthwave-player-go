package utils

import (
	"os"
	"testing"
)

// Opt-in manual check: SWP_MP3=/path/to.mp3 go test ./internal/utils/ -run TestMP3BitrateManual -v
func TestMP3BitrateManual(t *testing.T) {
	p := os.Getenv("SWP_MP3")
	if p == "" {
		t.Skip("set SWP_MP3 to a file path")
	}
	b, ok := MP3Bitrate(p)
	t.Logf("bitrate=%q ok=%v", b, ok)
	if !ok {
		t.Fatalf("failed to parse %s", p)
	}
}
