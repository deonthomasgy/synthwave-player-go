package search

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/Elive/synthwave-player/internal/models"
	"github.com/Elive/synthwave-player/internal/utils"
)

func Normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func Levenshtein(a, b string) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ca := range a {
		cur[0] = i + 1
		for j, cb := range b {
			cost := 1
			if ca == cb {
				cost = 0
			}
			cur[j+1] = min3(cur[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func SearchSongs(db *sql.DB, cfg config.Settings, r *http.Request, query string) models.LibraryResult {
	if query == "" {
		return models.LibraryResult{}
	}
	terms := strings.Fields(query)
	var normTerms []string
	for _, t := range terms {
		for _, p := range strings.Fields(Normalize(t)) {
			if p != "" {
				normTerms = append(normTerms, p)
			}
		}
	}
	if len(normTerms) == 0 {
		return models.LibraryResult{}
	}
	initial := InitialSongIDsFromContext(r, db)
	where := "1=1"
	var args []interface{}
	for _, term := range normTerms {
		if len(term) >= 4 {
			prefix := term[:2]
			where += " AND (title LIKE ? OR artist LIKE ? OR album LIKE ? OR title LIKE ? OR artist LIKE ? OR album LIKE ?)"
			args = append(args, "%"+term+"%", "%"+term+"%", "%"+term+"%", prefix+"%", prefix+"%", prefix+"%")
		} else {
			where += " AND (title LIKE ? OR artist LIKE ? OR album LIKE ?)"
			args = append(args, "%"+term+"%", "%"+term+"%", "%"+term+"%")
		}
	}
	if initial != nil {
		if len(initial) == 0 {
			return models.LibraryResult{}
		}
		placeholders := strings.Repeat("?,", len(initial))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND id IN (" + placeholders + ")"
		for _, id := range initial {
			args = append(args, id)
		}
	}
	rows, err := db.Query("SELECT * FROM songs WHERE "+where, args...)
	if err != nil {
		return models.LibraryResult{}
	}
	defer rows.Close()
	var candidates []map[string]interface{}
	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		m := map[string]interface{}{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		candidates = append(candidates, m)
	}
	ids, _ := rankResults(query, normTerms, candidates)
	hasManualSort := r.URL.Query().Get("sort") != ""
	return ProcessSongs(db, cfg, r, ids, nil, !hasManualSort, !hasManualSort)
}

func rankResults(query string, normTerms []string, rows []map[string]interface{}) ([]string, map[string]map[string]string) {
	type scored struct {
		id    string
		score int
	}
	var list []scored
	matches := map[string]map[string]string{}
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" {
			if b, ok := row["id"].([]byte); ok {
				id = string(b)
			}
		}
		title := str(row["title"])
		artist := str(row["artist"])
		album := str(row["album"])
		score := 0
		for _, term := range normTerms {
			for _, field := range []struct {
				name, val string
				weight    int
			}{
				{"title", title, 10},
				{"artist", artist, 5},
				{"album", album, 3},
			} {
				norm := Normalize(field.val)
				if strings.Contains(norm, term) {
					score += field.weight
				} else if len(term) >= 4 && Levenshtein(norm, term) <= 2 {
					score += field.weight / 2
				}
			}
		}
		if score > 0 {
			list = append(list, scored{id, score})
		}
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].score > list[i].score {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	ids := make([]string, len(list))
	for i, s := range list {
		ids[i] = s.id
	}
	return ids, matches
}

func str(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

func folderPath(location string) string {
	if i := strings.LastIndex(location, "/"); i > 0 {
		return location[:i]
	}
	return location
}

func sortSongsByFolder(songs []models.Song, sortDir string) {
	desc := strings.EqualFold(sortDir, "desc")
	sort.SliceStable(songs, func(i, j int) bool {
		fi, fj := folderPath(songs[i].Location), folderPath(songs[j].Location)
		if fi != fj {
			if desc {
				return fi > fj
			}
			return fi < fj
		}
		return utils.NaturalCompare(songs[i].Title, songs[j].Title) < 0
	})
}

func ProcessSongs(db *sql.DB, cfg config.Settings, r *http.Request, ids []string, matches map[string]map[string]string, disableSort, preserveOrder bool) models.LibraryResult {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	limit := atoiDefault(q.Get("limit"), cfg.SongsPerPage)
	sortBy := cfg.DefaultSortBy
	sortDir := cfg.DefaultSortOrder
	if sp := q.Get("sort"); sp != "" {
		re := regexp.MustCompile(`^(\w+)(?:\|(asc|desc))?$`)
		if m := re.FindStringSubmatch(strings.ToLower(sp)); len(m) > 0 {
			valid := map[string]bool{"title": true, "artist": true, "album": true, "genre": true, "duration": true, "rating": true, "track_number": true, "bitrate": true, "folder": true}
			if valid[m[1]] {
				sortBy = m[1]
			}
			if len(m) > 2 && m[2] == "desc" {
				sortDir = "desc"
			} else if len(m) > 2 {
				sortDir = "asc"
			}
		}
	}
	var songs []models.Song
	total := 0
	if preserveOrder && ids != nil {
		filtered := filterByRating(db, ids, q.Get("ratings"))
		total = len(filtered)
		start := (page - 1) * limit
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		pageIDs := filtered[start:end]
		songs = fetchSongsOrdered(db, pageIDs)
	} else {
		where := "1=1"
		var args []interface{}
		if ids != nil {
			if len(ids) == 0 {
				return models.LibraryResult{}
			}
			ph := strings.Repeat("?,", len(ids))
			ph = ph[:len(ph)-1]
			where += " AND id IN (" + ph + ")"
			for _, id := range ids {
				args = append(args, id)
			}
		}
		if rf := q.Get("ratings"); rf != "" {
			parts := strings.Split(rf, "|")
			var allowed []interface{}
			for _, p := range parts {
				if n := atoiDefault(p, -1); n >= 0 && n <= 5 {
					allowed = append(allowed, n)
				}
			}
			if len(allowed) > 0 {
				ph := strings.Repeat("?,", len(allowed))
				ph = ph[:len(ph)-1]
				where += " AND rating IN (" + ph + ")"
				args = append(args, allowed...)
			}
		}
		countQ := "SELECT COUNT(*) FROM songs WHERE " + where
		_ = db.QueryRow(countQ, args...).Scan(&total)
		if !disableSort && sortBy == "folder" {
			rows, err := db.Query("SELECT * FROM songs WHERE "+where, args...)
			if err == nil {
				all := scanSongs(rows)
				sortSongsByFolder(all, sortDir)
				start := (page - 1) * limit
				if start > len(all) {
					start = len(all)
				}
				end := start + limit
				if end > len(all) {
					end = len(all)
				}
				songs = all[start:end]
			}
		} else {
			order := sortBy + " " + strings.ToUpper(sortDir)
			if !disableSort {
				args = append(args, limit, (page-1)*limit)
				rows, err := db.Query("SELECT * FROM songs WHERE "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", args...)
				if err == nil {
					songs = scanSongs(rows)
				}
			}
		}
	}
	if songs == nil {
		songs = []models.Song{}
	}
	return models.LibraryResult{Songs: songs, Total: total}
}

func filterByRating(db *sql.DB, ids []string, ratings string) []string {
	if ratings == "" {
		return ids
	}
	allowed := map[int]bool{}
	for _, p := range strings.Split(ratings, "|") {
		if n := atoiDefault(p, -1); n >= 0 && n <= 5 {
			allowed[n] = true
		}
	}
	if len(allowed) == 0 {
		return ids
	}
	var out []string
	for _, id := range ids {
		var rating int
		if db.QueryRow(`SELECT rating FROM songs WHERE id = ?`, id).Scan(&rating) == nil {
			if allowed[rating] {
				out = append(out, id)
			}
		}
	}
	return out
}

func fetchSongsOrdered(db *sql.DB, ids []string) []models.Song {
	if len(ids) == 0 {
		return []models.Song{}
	}
	ph := strings.Repeat("?,", len(ids))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query("SELECT * FROM songs WHERE id IN ("+ph+")", args...)
	if err != nil {
		return nil
	}
	all := scanSongs(rows)
	byID := map[string]models.Song{}
	for _, s := range all {
		byID[s.ID] = s
	}
	out := make([]models.Song, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

func scanSongs(rows *sql.Rows) []models.Song {
	defer rows.Close()
	cols, _ := rows.Columns()
	var songs []models.Song
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		m := map[string]interface{}{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		songs = append(songs, rowToSong(m))
	}
	return songs
}

func rowToSong(m map[string]interface{}) models.Song {
	s := models.Song{
		ID:       str(m["id"]),
		Title:    str(m["title"]),
		Artist:   str(m["artist"]),
		Album:    str(m["album"]),
		Genre:    str(m["genre"]),
		Location: str(m["location"]),
	}
	if v, ok := m["rating"].(int64); ok {
		s.Rating = int(v)
	}
	if d, ok := m["duration"].(int64); ok {
		di := int(d)
		s.Duration = &di
	}
	if tn := trackNumber(m["track_number"]); tn != "" {
		s.TrackNumber = tn
	}
	if b := str(m["bitrate"]); b != "" {
		s.Bitrate = b
	}
	s.AlbumArtist = str(m["album_artist"])
	if v, ok := m["play_count"].(int64); ok {
		s.PlayCount = int(v)
	}
	if v, ok := m["skip_count"].(int64); ok {
		s.SkipCount = int(v)
	}
	s.IsRadio = strings.HasPrefix(strings.ToLower(s.Location), "http://") || strings.HasPrefix(strings.ToLower(s.Location), "https://")
	if s.IsRadio {
		s.Duration = nil
	}
	return s
}

// trackNumber normalizes a stored track number, dropping any "/total" suffix
// (e.g. "3/12" -> "3") to match the Perl server output. Returns "" when unset.
func trackNumber(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case int64:
		if t <= 0 {
			return ""
		}
		return strconv.FormatInt(t, 10)
	case string:
		t = strings.TrimSpace(t)
		if i := strings.IndexByte(t, '/'); i >= 0 {
			t = t[:i]
		}
		if t == "" || t == "0" {
			return ""
		}
		return t
	default:
		s := str(v)
		if i := strings.IndexByte(s, '/'); i >= 0 {
			s = s[:i]
		}
		if s == "0" {
			return ""
		}
		return s
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}

// CompareNatural wraps utils for sort callbacks.
func CompareNatural(a, b string) int { return utils.NaturalCompare(a, b) }
