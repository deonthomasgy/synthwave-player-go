package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Known mount paths checked when config has no music directories.
var knownMusicMounts = []string{
	"/mnt/share/640GB/Music_Deon",
}

// DiscoverMusicDirectories returns existing directories to scan when config lists none.
func DiscoverMusicDirectories() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimRight(filepath.Clean(p), string(os.PathSeparator))
		if p == "" || seen[p] {
			return
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return
		}
		seen[p] = true
		out = append(out, p)
	}

	if env := os.Getenv("SYNTHWAVE_MUSIC_DIRECTORIES"); env != "" {
		for _, p := range strings.Split(env, ":") {
			add(p)
		}
	}
	if len(out) > 0 {
		return out
	}

	add(xdgMusicDir())
	home, _ := os.UserHomeDir()
	add(filepath.Join(home, "Music"))
	for _, p := range knownMusicMounts {
		add(p)
	}
	if st, err := os.Stat("/usr/share/elive-demo-files-skel/Music"); err == nil && st.IsDir() {
		add("/usr/share/elive-demo-files-skel/Music")
	}
	return out
}

// DiscoverPlaylistDirectories returns playlist roots when config lists none.
func DiscoverPlaylistDirectories(musicDirs []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimRight(filepath.Clean(p), string(os.PathSeparator))
		if p == "" || seen[p] {
			return
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return
		}
		seen[p] = true
		out = append(out, p)
	}

	home, _ := os.UserHomeDir()
	add(filepath.Join(home, "Music", "playlists"))
	for _, d := range musicDirs {
		add(d)
	}
	if st, err := os.Stat("/usr/share/elive-demo-files-skel/Music"); err == nil && st.IsDir() {
		add("/usr/share/elive-demo-files-skel/Music")
	}
	return out
}
