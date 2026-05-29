package search

import (
	"database/sql"
	"net/http"
	"net/url"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInitialSongIDsFromContextAlbumFilter(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO songs (id, title, artist, album, genre, rating, location) VALUES
		('a1', 'Song A', 'Artist 1', 'Album One', 'Rock', 3, '/a1.mp3'),
		('a2', 'Song B', 'Artist 1', 'Album Two', 'Rock', 3, '/a2.mp3'),
		('a3', 'Song C', 'Artist 2', 'Album One', 'Pop', 3, '/a3.mp3')`)

	r := &http.Request{URL: &url.URL{RawQuery: "albums=Album%20One"}}
	ids := InitialSongIDsFromContext(r, db)
	if len(ids) != 2 {
		t.Fatalf("got %d ids %v, want 2 from Album One", len(ids), ids)
	}
	set := map[string]bool{ids[0]: true, ids[1]: true}
	if !set["a1"] || !set["a3"] {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestInitialSongIDsFromContextAndLogic(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO songs (id, title, artist, album, genre, rating, location) VALUES
		('a1', 'Song A', 'Artist 1', 'Album One', 'Rock', 3, '/a1.mp3'),
		('a2', 'Song B', 'Artist 1', 'Album Two', 'Jazz', 3, '/a2.mp3'),
		('a3', 'Song C', 'Artist 2', 'Album One', 'Rock', 3, '/a3.mp3')`)

	r := &http.Request{URL: &url.URL{RawQuery: "genres=Rock&albums=Album%20One"}}
	ids := InitialSongIDsFromContext(r, db)
	if len(ids) != 2 {
		t.Fatalf("AND logic: got %v, want a1 and a3", ids)
	}

	r = &http.Request{URL: &url.URL{RawQuery: "genres=Rock&albums=Album%20Two&logic=or"}}
	ids = InitialSongIDsFromContext(r, db)
	if len(ids) != 3 {
		t.Fatalf("OR logic: got %d ids %v, want 3", len(ids), ids)
	}
}

func TestInitialSongIDsFromContextNoFilters(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	r := &http.Request{URL: &url.URL{}}
	if ids := InitialSongIDsFromContext(r, db); ids != nil {
		t.Fatalf("got %v, want nil", ids)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE songs (
		id TEXT PRIMARY KEY, title TEXT, artist TEXT, album TEXT, genre TEXT,
		rating INTEGER, location TEXT UNIQUE)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE playlist_songs (
		playlist_name TEXT, song_id TEXT, position INTEGER)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
