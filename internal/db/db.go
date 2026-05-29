package db

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/Elive/synthwave-player/internal/config"
	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(logger *slog.Logger) (*DB, error) {
	path := config.DBPath()
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	w := &DB{DB: sqlDB}
	if err := w.InitSchema(); err != nil {
		logger.Warn("database init", "err", err)
	}
	return w, nil
}

func (db *DB) InitSchema() error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS songs (
			id TEXT PRIMARY KEY,
			title TEXT, artist TEXT, album TEXT, genre TEXT, duration INTEGER, rating INTEGER, location TEXT UNIQUE, track_number INTEGER, bitrate TEXT,
			album_artist TEXT, bpm TEXT, channels TEXT, comment TEXT, composer TEXT, date_added INTEGER, date_modified INTEGER, description TEXT, disc_number INTEGER, episode_number TEXT, keywords TEXT, sample_rate INTEGER, season_number TEXT, show_name TEXT, source TEXT, year TEXT, replaygain TEXT, file_format TEXT,
			last_played INTEGER, last_skipped INTEGER, play_count INTEGER DEFAULT 0, skip_count INTEGER DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_songs_artist ON songs(artist)`,
		`CREATE INDEX IF NOT EXISTS idx_songs_album ON songs(album)`,
		`CREATE INDEX IF NOT EXISTS idx_songs_genre ON songs(genre)`,
		`CREATE INDEX IF NOT EXISTS idx_songs_location ON songs(location)`,
		`CREATE TABLE IF NOT EXISTS playlists (name TEXT PRIMARY KEY, type TEXT, mtime INTEGER DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS playlist_songs (
			playlist_name TEXT, song_id TEXT, position INTEGER,
			PRIMARY KEY (playlist_name, position),
			FOREIGN KEY (playlist_name) REFERENCES playlists(name) ON DELETE CASCADE,
			FOREIGN KEY (song_id) REFERENCES songs(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_playlist_songs_song_id ON playlist_songs(song_id)`,
		`CREATE TABLE IF NOT EXISTS ignored_paths (path TEXT PRIMARY KEY, timestamp INTEGER)`,
		`CREATE TABLE IF NOT EXISTS scan_state (key TEXT PRIMARY KEY, value TEXT, timestamp INTEGER DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS configured_directories (path TEXT PRIMARY KEY, status TEXT, last_scanned INTEGER DEFAULT 0)`,
		`INSERT OR IGNORE INTO scan_state (key, value) VALUES ('scan_status', 'idle')`,
		`INSERT OR IGNORE INTO scan_state (key, value) VALUES ('last_scan_completed_at', '0')`,
		`INSERT OR IGNORE INTO scan_state (key, value) VALUES ('config_hash', '')`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	alters := []string{
		`ALTER TABLE scan_state ADD COLUMN timestamp INTEGER DEFAULT 0`,
		`ALTER TABLE songs ADD COLUMN album_artist TEXT`,
		`ALTER TABLE songs ADD COLUMN play_count INTEGER DEFAULT 0`,
		`ALTER TABLE songs ADD COLUMN skip_count INTEGER DEFAULT 0`,
	}
	for _, s := range alters {
		_, _ = tx.Exec(s)
	}
	return tx.Commit()
}

func (db *DB) ShrinkMemory() {
	_, _ = db.Exec(`PRAGMA shrink_memory`)
}
