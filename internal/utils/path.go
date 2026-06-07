package utils

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/Elive/synthwave-player/internal/storage/s3"
)

func Realpath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	res, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return abs, nil
		}
		return "", err
	}
	return res, nil
}

func SafeSongPath(db *sql.DB, id string, musicDirs []string) (string, error) {
	var location string
	err := db.QueryRow(`SELECT location FROM songs WHERE id = ?`, id).Scan(&location)
	if err != nil || location == "" {
		return "", err
	}
	if strings.Contains(location, "\x00") || strings.ContainsAny(location, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return "", os.ErrInvalid
	}
	if strings.Contains(location, "..") {
		return "", os.ErrInvalid
	}
	if s3.IsURI(location) {
		loc, err := s3.ParseURI(location)
		if err != nil {
			return "", os.ErrInvalid
		}
		if !s3.Allowed(loc, musicDirs) {
			return "", os.ErrPermission
		}
		return location, nil
	}
	real, err := Realpath(location)
	if err != nil || real == "" {
		return "", os.ErrNotExist
	}
	allowed := false
	for _, root := range musicDirs {
		rootReal, _ := Realpath(root)
		if rootReal == "" {
			rootReal = filepath.Clean(root)
		}
		if rootReal != "" && (real == rootReal || strings.HasPrefix(real, rootReal+string(filepath.Separator))) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", os.ErrPermission
	}
	if st, err := os.Stat(real); err != nil || st.IsDir() {
		return "", os.ErrNotExist
	}
	return real, nil
}

func SongIDFromPath(path string) string {
	real, err := Realpath(path)
	if err != nil {
		real = filepath.Clean(path)
	}
	real = strings.TrimRight(real, "/")
	return HashPathID(real)
}
