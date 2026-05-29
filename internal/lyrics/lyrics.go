package lyrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/PuerkitoBio/goquery"
)

func GetLocal(cfg config.Settings, songPath string) string {
	if cfg.EyeD3Available && songPath != "" {
		if l := fromEyeD3(songPath); l != "" && len(l) >= 100 {
			return l
		}
	}
	return ""
}

func fromEyeD3(path string) string {
	// Simplified: read cached file by song id elsewhere
	return ""
}

func FromCache(cacheDir, id string) (string, bool) {
	p := filepath.Join(cacheDir, id+".txt")
	data, err := os.ReadFile(p)
	if err != nil || len(data) < 100 {
		return "", false
	}
	return string(data), true
}

func SaveCache(cacheDir, id, text string) {
	_ = os.MkdirAll(cacheDir, 0o755)
	_ = os.WriteFile(filepath.Join(cacheDir, id+".txt"), []byte(text), 0o644)
}

func FetchOnlineAsync(cfg config.Settings, artist, title string, callback func(string)) {
	go func() {
		lyrics := fetchGenius(artist, title)
		if lyrics == "" {
			lyrics = fetchLyricsOVH(artist, title)
		}
		callback(lyrics)
	}()
}

func fetchLyricsOVH(artist, title string) string {
	u := "https://api.lyrics.ovh/v1/" + url.PathEscape(artist) + "/" + url.PathEscape(title)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var out struct {
		Lyrics string `json:"lyrics"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) == nil && len(out.Lyrics) >= 100 {
		return clean(out.Lyrics)
	}
	return ""
}

func fetchGenius(artist, title string) string {
	slug := geniusSlug(artist + " " + title)
	u := "https://genius.com/" + slug + "-lyrics"
	client := &http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return ""
	}
	var b strings.Builder
	doc.Find(`div[data-lyrics-container="true"]`).Each(func(_ int, sel *goquery.Selection) {
		sel.Find("br").Each(func(_ int, br *goquery.Selection) {
			br.ReplaceWithHtml("\n")
		})
		b.WriteString(sel.Text())
		b.WriteByte('\n')
	})
	text := clean(b.String())
	if len(text) >= 100 {
		return text
	}
	return ""
}

func geniusSlug(s string) string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer("'", "", "&", "and")
	s = repl.Replace(s)
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	s = reg.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func clean(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func ReadID3Lyrics(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	// USLT frames via id3v2 would go here; optional enhancement
	_, _ = io.ReadAll(f)
	return ""
}
