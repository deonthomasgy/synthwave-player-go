package utils

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dhowden/tag"
)

type FileMeta struct {
	Title, Artist, Album, Genre string
	Duration                    int
	Rating                      int
	TrackNumber                 int
	Bitrate                     string
	Location                    string
	AlbumArtist, BPM, Channels, Comment, Composer string
	DateAdded, DateModified     int64
	Description                 string
	DiscNumber                  int
	Year                        string
	FileFormat                  string
}

func ExtractMetadata(path string) FileMeta {
	st, _ := os.Stat(path)
	meta := FileMeta{
		Duration: 999999998,
		Location: path,
	}
	if st != nil {
		meta.DateAdded = st.ModTime().Unix()
		meta.DateModified = st.ModTime().Unix()
	}
	meta.FileFormat = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")

	f, err := os.Open(path)
	if err != nil {
		fillFromFilename(&meta, path)
		return meta
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		fillFromFilename(&meta, path)
		return meta
	}
	if t := m.Title(); t != "" {
		meta.Title = t
	}
	if a := m.Artist(); a != "" {
		meta.Artist = a
	}
	if al := m.Album(); al != "" {
		meta.Album = al
	}
	if g := m.Genre(); g != "" {
		meta.Genre = g
	}
	if y := m.Year(); y != 0 {
		meta.Year = strconv.Itoa(y)
	}
	if n, _ := m.Track(); n > 0 {
		meta.TrackNumber = n
	}
	// Duration from tag is format-dependent; leave default or estimate later
	_ = m
	if meta.FileFormat == "mp3" {
		if b, ok := MP3Bitrate(path); ok {
			meta.Bitrate = b
		}
	}
	if meta.Title == "" {
		fillFromFilename(&meta, path)
	}
	return meta
}

func fillFromFilename(meta *FileMeta, path string) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if meta.Title == "" {
		meta.Title = base
	}
}

func CoverArt(path string) ([]byte, string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", false
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, "", false
	}
	p := m.Picture()
	if p == nil || len(p.Data) == 0 {
		return nil, "", false
	}
	mime := p.MIMEType
	if mime == "" {
		mime = "image/jpeg"
	}
	return p.Data, mime, true
}
