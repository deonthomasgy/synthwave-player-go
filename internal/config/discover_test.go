package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverMusicDirectoriesEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SYNTHWAVE_MUSIC_DIRECTORIES", dir)
	got := DiscoverMusicDirectories()
	if len(got) != 1 || got[0] != dir {
		t.Fatalf("got %v, want [%s]", got, dir)
	}
}

func TestDiscoverMusicDirectoriesSkipsMissing(t *testing.T) {
	t.Setenv("SYNTHWAVE_MUSIC_DIRECTORIES", "/nonexistent/path/for/synthwave-test")
	got := DiscoverMusicDirectories()
	// Falls through to xdg/home/mounts; should not include the missing env path.
	for _, p := range got {
		if p == "/nonexistent/path/for/synthwave-test" {
			t.Fatalf("included missing env path")
		}
	}
}

func TestDiscoverPlaylistDirectories(t *testing.T) {
	music := t.TempDir()
	playlists := filepath.Join(music, "playlists")
	if err := os.MkdirAll(playlists, 0o755); err != nil {
		t.Fatal(err)
	}
	got := DiscoverPlaylistDirectories([]string{music})
	if len(got) == 0 {
		t.Fatal("expected at least one playlist dir")
	}
	found := false
	for _, p := range got {
		if p == playlists || p == music {
			found = true
		}
	}
	if !found {
		t.Fatalf("got %v", got)
	}
}
