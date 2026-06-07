package s3

import (
	"fmt"
	"strings"
)

// Location is an s3://bucket/key reference stored in the songs table.
type Location struct {
	Bucket string
	Key    string
}

func (l Location) String() string {
	if l.Key == "" {
		return "s3://" + l.Bucket + "/"
	}
	return "s3://" + l.Bucket + "/" + l.Key
}

// IsURI reports whether path is an s3:// location.
func IsURI(path string) bool {
	return strings.HasPrefix(strings.ToLower(path), "s3://")
}

// ParseURI parses s3://bucket/optional/prefix or s3://bucket/key.
func ParseURI(uri string) (Location, error) {
	if !IsURI(uri) {
		return Location{}, fmt.Errorf("not an s3 uri: %q", uri)
	}
	rest := strings.TrimPrefix(uri, "s3://")
	rest = strings.TrimPrefix(rest, "S3://")
	if rest == "" {
		return Location{}, fmt.Errorf("empty s3 uri")
	}
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return Location{Bucket: rest}, nil
	}
	bucket := rest[:slash]
	key := strings.TrimPrefix(rest[slash+1:], "/")
	return Location{Bucket: bucket, Key: key}, nil
}

// NormalizePrefix trims trailing slashes from an s3 directory entry.
func NormalizePrefix(uri string) string {
	loc, err := ParseURI(uri)
	if err != nil {
		return strings.TrimRight(uri, "/")
	}
	return loc.String()
}

// ValidKey rejects unsafe or malformed object keys.
func ValidKey(key string) bool {
	if key == "" {
		return false
	}
	if strings.Contains(key, "\x00") || strings.Contains(key, "..") {
		return false
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	return true
}

// Allowed checks that loc is under one of the configured s3 music directory prefixes.
func Allowed(loc Location, musicDirs []string) bool {
	if !ValidKey(loc.Key) {
		return false
	}
	for _, dir := range musicDirs {
		if !IsURI(dir) {
			continue
		}
		root, err := ParseURI(strings.TrimRight(dir, "/"))
		if err != nil {
			continue
		}
		if root.Bucket != loc.Bucket {
			continue
		}
		if root.Key == "" {
			return true
		}
		prefix := strings.TrimSuffix(root.Key, "/") + "/"
		if loc.Key == root.Key || strings.HasPrefix(loc.Key, prefix) {
			return true
		}
	}
	return false
}
