package search

import (
	"database/sql"
	"net/http"
	"strings"
)

type contextFilter struct {
	typ    string
	values []string
}

// InitialSongIDsFromContext returns song IDs matching playlist/genre/artist/album filters.
// nil means no sidebar filters (all songs). An empty slice means no matches.
func InitialSongIDsFromContext(r *http.Request, db *sql.DB) []string {
	q := r.URL.Query()
	logic := q.Get("logic")
	if logic == "" {
		logic = "and"
	}

	var filters []contextFilter
	if s := q.Get("playlists"); s != "" {
		filters = append(filters, contextFilter{"playlist", splitPipe(s)})
	}
	if s := q.Get("genres"); s != "" {
		filters = append(filters, contextFilter{"genre", splitPipe(s)})
	}
	if s := q.Get("artists"); s != "" {
		filters = append(filters, contextFilter{"artist", splitPipe(s)})
	}
	if s := q.Get("albums"); s != "" {
		filters = append(filters, contextFilter{"album", splitPipe(s)})
	}
	if len(filters) == 0 {
		return nil
	}

	var set map[string]bool
	first := true
	for _, f := range filters {
		if len(f.values) == 0 {
			continue
		}
		matches := idsForFilter(db, f)
		if first {
			set = make(map[string]bool, len(matches))
			for _, id := range matches {
				set[id] = true
			}
			first = false
			continue
		}
		if logic == "or" {
			for _, id := range matches {
				set[id] = true
			}
		} else {
			current := make(map[string]bool, len(matches))
			for _, id := range matches {
				current[id] = true
			}
			next := make(map[string]bool)
			for id := range set {
				if current[id] {
					next[id] = true
				}
			}
			set = next
		}
	}
	if set == nil {
		return []string{}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func splitPipe(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "|") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func idsForFilter(db *sql.DB, f contextFilter) []string {
	switch f.typ {
	case "playlist":
		ph := strings.Repeat("?,", len(f.values))
		ph = ph[:len(ph)-1]
		args := make([]interface{}, len(f.values))
		for i, v := range f.values {
			args[i] = v
		}
		q := `SELECT song_id FROM playlist_songs WHERE playlist_name IN (` + ph + `)`
		return queryIDColumn(db, q, args...)
	case "genre", "artist", "album":
		ph := strings.Repeat("?,", len(f.values))
		ph = ph[:len(ph)-1]
		args := make([]interface{}, len(f.values))
		for i, v := range f.values {
			args[i] = v
		}
		q := `SELECT id FROM songs WHERE ` + f.typ + ` IN (` + ph + `)`
		return queryIDColumn(db, q, args...)
	default:
		return nil
	}
}

func queryIDColumn(db *sql.DB, query string, args ...interface{}) []string {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
