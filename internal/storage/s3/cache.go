package s3

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalCachePath downloads an object to cacheDir and returns the local path.
func LocalCachePath(ctx context.Context, client *Client, cacheDir, uri string) (string, error) {
	loc, err := ParseURI(uri)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum([]byte(uri))
	id := hex.EncodeToString(sum[:8])
	base := filepath.Base(loc.Key)
	if base == "" || base == "." {
		base = "object"
	}
	dir := filepath.Join(cacheDir, "s3-cache", id)
	dest := filepath.Join(dir, base)
	if st, err := os.Stat(dest); err == nil && !st.IsDir() {
		return dest, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := dest + ".part." + fmt.Sprintf("%d", time.Now().UnixNano())
	if err := client.DownloadToFile(ctx, loc.Bucket, loc.Key, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return dest, nil
}

// ContentTypeForKey guesses audio MIME type from object key extension.
func ContentTypeForKey(key string) string {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".opus":
		return "audio/opus"
	case ".aiff", ".aif":
		return "audio/aiff"
	case ".wma":
		return "audio/x-ms-wma"
	default:
		return "application/octet-stream"
	}
}
