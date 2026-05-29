package utils

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func HashPathID(path string) string {
	h := sha1.Sum([]byte(path))
	return hex.EncodeToString(h[:])[:16]
}

func SourceHash(paths ...string) string {
	h := sha256.New()
	var files []string
	for _, root := range paths {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if stringsHasAnySuffix(p, ".go", ".js", ".css", ".html", ".ep", ".pm") {
				files = append(files, p)
			}
			return nil
		})
	}
	sort.Strings(files)
	for _, p := range files {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		_, _ = io.Copy(h, f)
		f.Close()
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if len(sum) > 16 {
		return sum[:16]
	}
	return sum
}

func stringsHasAnySuffix(p string, suffixes ...string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(p, s) {
			return true
		}
	}
	return false
}
