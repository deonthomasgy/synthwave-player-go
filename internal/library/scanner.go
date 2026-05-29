package library

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Elive/synthwave-player/internal/models"
	"github.com/Elive/synthwave-player/internal/utils"
)

var audioExts = map[string]bool{
	".mp3": true, ".m4a": true, ".ogg": true, ".flac": true, ".wav": true,
	".opus": true, ".aiff": true, ".aif": true, ".aac": true, ".wma": true, ".mka": true,
}

func (s *Service) runTaskProcessor() {
	for {
		row := s.DB.QueryRow(`SELECT key FROM scan_state WHERE (key LIKE 'task:%' OR key LIKE 'purge_task:%') AND value = 'pending' LIMIT 1`)
		var key string
		if err := row.Scan(&key); err != nil {
			time.Sleep(2 * time.Second)
			s.checkIdle()
			continue
		}
		if strings.HasPrefix(key, "purge_task:") {
			s.purgeDir(strings.TrimPrefix(key, "purge_task:"))
		} else {
			s.scanDir(strings.TrimPrefix(key, "task:"))
		}
	}
}

func (s *Service) scanDir(dir string) {
	s.SetLoading(models.LoadingStatus{Message: "Scanning " + dir + "...", Done: false})
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if audioExts[strings.ToLower(filepath.Ext(path))] {
			files = append(files, path)
		}
		return nil
	})
	total := len(files)
	s.SetLoading(models.LoadingStatus{Total: total, Processed: 0, Message: "Processing files...", Done: false})
	for i, f := range files {
		s.indexFile(f)
		if i%10 == 0 {
			prog := 0
			if total > 0 {
				prog = (i * 100) / total
			}
			s.SetLoading(models.LoadingStatus{Total: total, Processed: i, Progress: prog, Message: "Processing...", Done: false})
		}
	}
	_, _ = s.DB.Exec(`UPDATE configured_directories SET status = 'scanned', last_scanned = ? WHERE path = ?`, time.Now().Unix(), dir)
	_, _ = s.DB.Exec(`DELETE FROM scan_state WHERE key = ?`, "task:"+dir)
	go s.parsePlaylists(true)
	s.SetLoading(models.LoadingStatus{Done: true, Total: total, Processed: total, Progress: 100, Message: "Ready"})
	if s.Hub != nil {
		s.Hub.BroadcastLibrary()
	}
}

func (s *Service) indexFile(path string) {
	real, err := utils.Realpath(path)
	if err != nil {
		real = path
	}
	real = strings.TrimRight(real, "/")
	id := utils.HashPathID(real)
	st, _ := os.Stat(path)
	var mtime int64
	if st != nil {
		mtime = st.ModTime().Unix()
	}
	var dbMtime int64
	_ = s.DB.QueryRow(`SELECT date_modified FROM songs WHERE id = ?`, id).Scan(&dbMtime)
	if dbMtime > 0 && dbMtime == mtime {
		return
	}
	meta := utils.ExtractMetadata(path)
	_, _ = s.DB.Exec(`INSERT INTO songs (id, title, artist, album, genre, duration, rating, location, track_number, bitrate, date_added, date_modified, file_format)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title, artist=excluded.artist, album=excluded.album, genre=excluded.genre,
		duration=excluded.duration, location=excluded.location, track_number=excluded.track_number, bitrate=excluded.bitrate,
		date_modified=excluded.date_modified, file_format=excluded.file_format`,
		id, meta.Title, meta.Artist, meta.Album, meta.Genre, meta.Duration, meta.Rating, real,
		meta.TrackNumber, meta.Bitrate, meta.DateAdded, mtime, meta.FileFormat)
}

func (s *Service) purgeDir(dir string) {
	like := dir + "%"
	_, _ = s.DB.Exec(`DELETE FROM songs WHERE location LIKE ?`, like)
	_, _ = s.DB.Exec(`DELETE FROM configured_directories WHERE path = ?`, dir)
	_, _ = s.DB.Exec(`DELETE FROM scan_state WHERE key = ?`, "purge_task:"+dir)
	if s.Hub != nil {
		s.Hub.BroadcastLibrary()
	}
}

func (s *Service) parsePlaylists(isReload bool) {
	st := s.Config.Get()
	for _, dir := range st.PlaylistsDirectories {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".pls" && ext != ".m3u" && ext != ".m3u8" {
				return nil
			}
			s.parsePlaylistFile(path, ext)
			return nil
		})
	}
	_ = isReload
}

func (s *Service) parsePlaylistFile(path, ext string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	name := strings.TrimSuffix(filepath.Base(path), ext)
	_, _ = s.DB.Exec(`INSERT OR REPLACE INTO playlists (name, type, mtime) VALUES (?, ?, ?)`, name, ext[1:], time.Now().Unix())
	_, _ = s.DB.Exec(`DELETE FROM playlist_songs WHERE playlist_name = ?`, name)
	sc := bufio.NewScanner(f)
	pos := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "http") {
			continue
		}
		loc := line
		if idx := strings.Index(line, "file://"); idx >= 0 {
			loc = line[idx+7:]
		}
		id := utils.HashPathID(loc)
		pos++
		_, _ = s.DB.Exec(`INSERT OR IGNORE INTO playlist_songs (playlist_name, song_id, position) VALUES (?, ?, ?)`, name, id, pos)
	}
}

func (s *Service) VerifyIntegrity() int {
	rows, _ := s.DB.Query(`SELECT path FROM configured_directories`)
	defer rows.Close()
	tasks := 0
	for rows.Next() {
		var p string
		if rows.Scan(&p) != nil {
			continue
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES (?, 'pending')`, "purge_task:"+p)
			tasks++
		}
	}
	if tasks > 0 {
		go s.runTaskProcessor()
	}
	return tasks
}

func (s *Service) checkIdle() {
	s.reconcileReadyState()
}

func (s *Service) DeepRescan() {
	st := s.Config.Get()
	for _, d := range st.MusicDirectories {
		if strings.HasPrefix(strings.ToLower(d), "http") {
			continue
		}
		p, _ := utils.Realpath(d)
		if p == "" {
			p = d
		}
		_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES (?, 'pending')`, "task:"+strings.TrimRight(p, "/"))
	}
	go s.runTaskProcessor()
}
