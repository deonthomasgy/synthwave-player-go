package library

import (
	"database/sql"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/Elive/synthwave-player/internal/jsonutil"
	"github.com/Elive/synthwave-player/internal/db"
	"github.com/Elive/synthwave-player/internal/models"
	"github.com/Elive/synthwave-player/internal/utils"
)

type Service struct {
	DB       *db.DB
	Config   *config.Store
	Logger   *slog.Logger
	Hub      Broadcaster

	mu            sync.Mutex
	loading       models.LoadingStatus
	isLoading     bool
	processorOnce sync.Once
}

type Broadcaster interface {
	BroadcastStatus()
	BroadcastLibrary()
	BroadcastConfig()
	BroadcastLyrics(songID, lyrics string)
}

func NewService(database *db.DB, cfg *config.Store, logger *slog.Logger, hub Broadcaster) *Service {
	s := &Service{
		DB:     database,
		Config: cfg,
		Logger: logger,
		Hub:    hub,
		loading: models.LoadingStatus{
			Message: "Initializing...",
		},
	}
	return s
}

func (s *Service) LoadingStatus() models.LoadingStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.loading
	if s.loading.Total > 0 && s.loading.Processed >= s.loading.Total && !s.loading.Done {
		st.Done = true
		st.Progress = 100
	}
	return st
}

func (s *Service) SetLoading(st models.LoadingStatus) {
	s.mu.Lock()
	s.loading = st
	s.isLoading = !st.Done
	s.mu.Unlock()
	if s.Hub != nil {
		s.Hub.BroadcastStatus()
	}
}

func (s *Service) IsLibraryReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loading.Done
}

func (s *Service) StartBackground() {
	go func() {
		time.Sleep(time.Duration(1+time.Now().UnixNano()%2) * time.Second)
		// A leftover "scanning" status from a previous process (e.g. interrupted
		// scan) is stale: the goroutine that set it died with that process. Reset
		// it so the library can be synced and marked ready; real pending work is
		// still tracked by configured_directories.status and task:* rows.
		s.resetStaleScanState()
		st := s.Config.Get()
		if err := s.SyncLibraryToConfig(nil, st.MusicDirectories, nil, friendURLs(st.FriendsMusic)); err == nil {
			hash := s.Config.ConfigHash()
			_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES ('config_hash', ?)`, hash)
		}
		s.VerifyIntegrity()
		s.processorOnce.Do(func() { go s.runTaskProcessor() })
		s.checkIdle()
		go s.backfillBitrate()
	}()
}

// backfillBitrate populates bitrate for existing local MP3 rows that were indexed
// before bitrate extraction existed. Runs once at startup over rows missing it.
func (s *Service) backfillBitrate() {
	rows, err := s.DB.Query(`SELECT id, location FROM songs WHERE (bitrate IS NULL OR bitrate = '') AND lower(location) LIKE '%.mp3'`)
	if err != nil {
		return
	}
	type item struct{ id, loc string }
	var items []item
	for rows.Next() {
		var it item
		if rows.Scan(&it.id, &it.loc) == nil {
			items = append(items, it)
		}
	}
	rows.Close()
	if len(items) == 0 {
		return
	}
	updated := 0
	for _, it := range items {
		b, ok := utils.MP3Bitrate(it.loc)
		if !ok || b == "" {
			continue
		}
		if _, err := s.DB.Exec(`UPDATE songs SET bitrate = ? WHERE id = ?`, b, it.id); err == nil {
			updated++
		}
	}
	if updated > 0 {
		if s.Logger != nil {
			s.Logger.Info("backfilled bitrate", "songs", updated)
		}
		if s.Hub != nil {
			s.Hub.BroadcastLibrary()
		}
	}
}

func (s *Service) resetStaleScanState() {
	var status string
	_ = s.DB.QueryRow(`SELECT value FROM scan_state WHERE key = 'scan_status'`).Scan(&status)
	if status == "scanning" {
		_, _ = s.DB.Exec(`UPDATE scan_state SET value = 'idle', timestamp = 0 WHERE key = 'scan_status'`)
		if s.Logger != nil {
			s.Logger.Info("reset stale 'scanning' state from previous run")
		}
	}
}

func friendURLs(friends []config.FriendMusic) []string {
	var urls []string
	for _, f := range friends {
		urls = append(urls, f.Value)
	}
	return urls
}

func (s *Service) SyncLibraryToConfig(oldDirs, newDirs, oldFriends, newFriends []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	var status string
	var ts int64
	_ = tx.QueryRow(`SELECT value, timestamp FROM scan_state WHERE key = 'scan_status'`).Scan(&status, &ts)
	if status == "scanning" && ts > time.Now().Unix()-900 {
		tx.Rollback()
		return nil
	}
	_, _ = tx.Exec(`UPDATE scan_state SET value = 'scanning', timestamp = ? WHERE key = 'scan_status'`, time.Now().Unix())
	tx.Commit()

	newMap := map[string]bool{}
	for _, d := range newDirs {
		if strings.HasPrefix(strings.ToLower(d), "http://") || strings.HasPrefix(strings.ToLower(d), "https://") {
			continue
		}
		p, _ := utils.Realpath(d)
		if p == "" {
			p = filepath.Clean(d)
		}
		p = strings.TrimRight(p, "/")
		newMap[p] = true
	}
	for _, u := range newFriends {
		if strings.HasPrefix(strings.ToLower(u), "http") {
			continue
		}
		p, _ := utils.Realpath(u)
		if p != "" {
			newMap[strings.TrimRight(p, "/")] = true
		}
	}

	rows, _ := s.DB.Query(`SELECT path FROM configured_directories`)
	var dbDirs []string
	for rows.Next() {
		var p string
		if rows.Scan(&p) == nil {
			dbDirs = append(dbDirs, p)
		}
	}
	rows.Close()

	var toPurge, toScan []string
	for _, old := range dbDirs {
		if strings.HasPrefix(strings.ToLower(old), "http") {
			_, _ = s.DB.Exec(`DELETE FROM configured_directories WHERE path = ?`, old)
			continue
		}
		norm := strings.TrimRight(old, "/")
		if !newMap[norm] {
			toPurge = append(toPurge, norm)
		}
	}
	for p := range newMap {
		var st string
		err := s.DB.QueryRow(`SELECT status FROM configured_directories WHERE path = ?`, p).Scan(&st)
		if err == sql.ErrNoRows {
			_, _ = s.DB.Exec(`INSERT INTO configured_directories (path, status) VALUES (?, 'pending')`, p)
			toScan = append(toScan, p)
		} else if st != "scanned" {
			toScan = append(toScan, p)
		}
	}
	if len(toPurge) == 0 && len(toScan) == 0 {
		_, _ = s.DB.Exec(`UPDATE scan_state SET value = 'idle', timestamp = 0 WHERE key = 'scan_status'`)
		go s.parsePlaylists(true)
		return nil
	}
	for _, p := range toPurge {
		_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES (?, 'pending')`, "purge_task:"+p)
	}
	for _, p := range toScan {
		_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES (?, 'pending')`, "task:"+p)
	}
	go s.runTaskProcessor()
	return nil
}

func (s *Service) FindSongByPartialID(partial string) (*models.Song, string, bool) {
	if partial == "" {
		return nil, "", false
	}
	rows, err := s.DB.Query(`SELECT * FROM songs WHERE id LIKE ?`, partial+"%")
	if err != nil {
		return nil, "", false
	}
	defer rows.Close()
	var found *models.Song
	var fullID string
	for rows.Next() {
		song, id, ok := scanRow(rows)
		if !ok {
			continue
		}
		if fullID != "" && fullID != id {
			return nil, "", false
		}
		fullID = id
		found = song
	}
	return found, fullID, found != nil
}

func scanRow(rows *sql.Rows) (*models.Song, string, bool) {
	cols, _ := rows.Columns()
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if rows.Scan(ptrs...) != nil {
		return nil, "", false
	}
	m := map[string]interface{}{}
	for i, c := range cols {
		m[c] = vals[i]
	}
	song := rowToSong(m)
	return &song, song.ID, true
}

func rowToSong(m map[string]interface{}) models.Song {
	str := func(k string) string {
		switch v := m[k].(type) {
		case string:
			return v
		case []byte:
			return string(v)
		default:
			return ""
		}
	}
	s := models.Song{
		ID: str("id"), Title: str("title"), Artist: str("artist"),
		Album: str("album"), Genre: str("genre"), Location: str("location"),
	}
	if r, ok := m["rating"].(int64); ok {
		s.Rating = int(r)
	}
	if d, ok := m["duration"].(int64); ok {
		di := int(d)
		s.Duration = &di
	}
	if tn, ok := m["track_number"].(int64); ok && tn > 0 {
		s.TrackNumber = strconv.FormatInt(tn, 10)
	} else if t := str("track_number"); t != "" {
		if i := strings.IndexByte(t, '/'); i >= 0 {
			t = t[:i]
		}
		if t != "" && t != "0" {
			s.TrackNumber = t
		}
	}
	if b := str("bitrate"); b != "" {
		s.Bitrate = b
	}
	s.AlbumArtist = str("album_artist")
	if v, ok := m["play_count"].(int64); ok {
		s.PlayCount = int(v)
	}
	if v, ok := m["skip_count"].(int64); ok {
		s.SkipCount = int(v)
	}
	s.IsRadio = strings.HasPrefix(strings.ToLower(s.Location), "http")
	if s.IsRadio {
		s.Duration = nil
	}
	return s
}

func (s *Service) GetPlaylists(ratings string) (map[string]interface{}, error) {
	names, err := s.DB.Query(`SELECT name, type, mtime FROM playlists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer names.Close()
	info := map[string]interface{}{}
	var list []string
	for names.Next() {
		var name, typ string
		var mtime int64
		if names.Scan(&name, &typ, &mtime) == nil {
			list = append(list, name)
			info[name] = map[string]interface{}{"type": typ, "mtime": mtime}
		}
	}
	return map[string]interface{}{"names": jsonutil.StringSlice(list), "info": info}, nil
}

func (s *Service) GetDistinct(col, ratings string) ([]string, error) {
	allowed := map[string]bool{"genre": true, "artist": true, "album": true}
	if !allowed[col] {
		return []string{}, nil
	}
	q := `SELECT DISTINCT ` + col + ` FROM songs WHERE ` + col + ` IS NOT NULL AND ` + col + ` != '' ORDER BY ` + col
	rows, err := s.DB.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil && v != "" {
			out = append(out, v)
		}
	}
	return jsonutil.StringSlice(out), nil
}

func (s *Service) UpdateSongStats(id, typ string) {
	col := "play_count"
	if typ == "skip" {
		col = "skip_count"
	}
	_, _ = s.DB.Exec(`UPDATE songs SET `+col+` = COALESCE(`+col+`,0)+1 WHERE id = ?`, id)
}
