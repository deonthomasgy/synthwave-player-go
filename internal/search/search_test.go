package search

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/Elive/synthwave-player/internal/config"
)

func TestProcessSongsFolderSort(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO songs (id, title, artist, album, genre, rating, location) VALUES
		('z', 'Zebra', 'Artist', 'Album', 'Rock', 3, 's3://music/beta/track.mp3'),
		('a', 'Alpha', 'Artist', 'Album', 'Rock', 3, 's3://music/alpha/one.mp3'),
		('b', 'Beta', 'Artist', 'Album', 'Rock', 3, 's3://music/alpha/two.mp3'),
		('c', 'Charlie', 'Artist', 'Album', 'Rock', 3, 's3://music/beta/a.mp3')`)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Settings{SongsPerPage: 100, DefaultSortBy: "title", DefaultSortOrder: "asc"}
	r := &http.Request{URL: &url.URL{RawQuery: "sort=folder|asc"}}
	res := ProcessSongs(db, cfg, r, nil, nil, false, false)

	if len(res.Songs) != 4 {
		t.Fatalf("got %d songs, want 4", len(res.Songs))
	}

	got := []string{res.Songs[0].ID, res.Songs[1].ID, res.Songs[2].ID, res.Songs[3].ID}
	want := []string{"a", "b", "c", "z"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order[%d]=%s (%s), want %s", i, got[i], res.Songs[i].Title, want[i])
		}
	}
}

func TestProcessSongsFolderSortDefaultConfig(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO songs (id, title, artist, album, genre, rating, location) VALUES
		('1', 'Second', 'Artist', 'Album', 'Rock', 3, 's3://music/z/one.mp3'),
		('2', 'First', 'Artist', 'Album', 'Rock', 3, 's3://music/a/one.mp3')`)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Settings{SongsPerPage: 100, DefaultSortBy: "folder", DefaultSortOrder: "asc"}
	r := &http.Request{URL: &url.URL{}}
	res := ProcessSongs(db, cfg, r, nil, nil, false, false)

	if len(res.Songs) != 2 {
		t.Fatalf("got %d songs, want 2", len(res.Songs))
	}
	if res.Songs[0].ID != "2" || res.Songs[1].ID != "1" {
		t.Fatalf("got order %s, %s; want 2 (a folder), 1 (z folder)", res.Songs[0].ID, res.Songs[1].ID)
	}
}
