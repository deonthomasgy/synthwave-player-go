package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/Elive/synthwave-player/internal/jsonutil"
	"github.com/Elive/synthwave-player/internal/db"
	"github.com/Elive/synthwave-player/internal/library"
	"github.com/Elive/synthwave-player/internal/lyrics"
	"github.com/Elive/synthwave-player/internal/models"
	"github.com/Elive/synthwave-player/internal/network"
	"github.com/Elive/synthwave-player/internal/paths"
	"github.com/Elive/synthwave-player/internal/search"
	"github.com/Elive/synthwave-player/internal/storage/s3"
	"github.com/Elive/synthwave-player/internal/utils"
	"github.com/alexedwards/scs/v2"
	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	Logger   *slog.Logger
	Config   *config.Store
	DB       *db.DB
	Library  *library.Service
	Hub      *Hub
	Sessions *scs.SessionManager
	Limiter  *network.RateLimiter
	Login    *network.LoginState
	Hash     string
}

func New(logger *slog.Logger, cfg *config.Store, database *db.DB) *Server {
	hash := utils.SourceHash(paths.Base(), filepath.Join(paths.Base(), "public"))
	hub := NewHub(hash)
	lib := library.NewService(database, cfg, logger, hub)
	hub.hash = hash
	sess := scs.New()
	sess.Lifetime = 10 * 365 * 24 * time.Hour
	sess.Cookie.Name = "synthwave_player"
	sess.Cookie.HttpOnly = true
	sess.Cookie.SameSite = http.SameSiteLaxMode
	st := cfg.Get()
	return &Server{
		Logger: logger, Config: cfg, DB: database, Library: lib, Hub: hub,
		Sessions: sess, Limiter: network.NewRateLimiter(st),
		Login: network.NewLoginState(), Hash: hash,
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(s.Sessions.LoadAndSave)
	r.Use(s.forwardedProto)
	r.Use(s.reloadConfig)

	r.Get("/", s.handleIndex)
	r.Get("/api/version", s.handleVersion)
	r.Get("/api/ws", s.handleWS)
	r.Get("/api/status", s.handleStatus)
	r.Get("/api/config", s.handleGetConfig)
	r.Post("/api/config", s.handlePostConfig)
	r.Post("/api/admin/set-password", s.handleSetPassword)
	r.Post("/api/admin/login", s.handleLogin)
	r.Post("/api/admin/logout", s.handleLogout)
	r.Post("/api/admin/setup_upnp", s.handleUPnP)
	r.Get("/api/check_update", s.handleCheckUpdate)
	r.Get("/api/library", s.handleLibrary)
	r.Get("/api/playlist/{name}", s.handlePlaylist)
	r.Get("/api/search", s.handleSearch)
	r.Get("/api/song/{id}", s.handleSong)
	r.Get("/api/playlists", s.handlePlaylists)
	r.Get("/api/genres", s.handleGenres)
	r.Get("/api/artists", s.handleArtists)
	r.Get("/api/albums", s.handleAlbums)
	r.Post("/api/song/{id}/stats", s.handleStats)
	r.Post("/api/log", s.handleLog)
	r.Get("/api/cover_exists/{id}", s.handleCoverExists)
	r.Get("/api/lyrics/{id}", s.handleLyrics)
	r.Post("/api/library/rescan", s.handleRescan)
	r.Post("/api/song/{id}/update_tag", s.handleUpdateTag)
	r.Get("/audio/{id}", s.handleAudio)
	r.Get("/cover/{id}", s.handleCover)

	// Static assets at root paths used by UI
	r.Get("/synthwaveplayer.js", staticFile("synthwaveplayer.js"))
	r.Get("/synthwaveplayer.css", staticFile("synthwaveplayer.css"))
	r.Get("/tailwindcss.min.js", staticFile("tailwindcss.min.js"))
	r.Get("/fuse.min.js", staticFile("fuse.min.js"))
	r.Get("/alpinejs.min.js", staticFile("alpinejs.min.js"))
	r.Get("/menu-popup.js", staticFile("menu-popup.js"))
	r.Get("/manifest.webmanifest", staticFile("manifest.webmanifest"))

	// Fallback static assets (cover art placeholders, etc.)
	fs := http.FileServer(http.Dir(paths.PublicDir()))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/", fs).ServeHTTP(w, r)
	})

	return r
}

func staticFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(paths.PublicDir(), name))
	}
}

func (s *Server) forwardedProto(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
			r.URL.Scheme = p
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				r.RemoteAddr = strings.TrimSpace(parts[0]) + ":0"
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) reloadConfig(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Config.MaybeReload(s.Logger)
		st := s.Config.Get()
		s.Limiter = network.NewRateLimiter(st)
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(w http.ResponseWriter, r *http.Request) bool {
	st := s.Config.Get()
	ip := network.RemoteAddr(r)
	addr := network.RemoteAddrWithPort(r)
	ok, msg := s.Limiter.Allow(ip, addr, st.PrivateNetworks)
	if !ok {
		w.Header().Set("X-Error-Notification", url.QueryEscape(msg))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": msg})
		return false
	}
	return true
}

func (s *Server) isAdmin(r *http.Request) bool {
	st := s.Config.Get()
	if s.Sessions.GetBool(r.Context(), "admin") {
		return true
	}
	ip := network.RemoteAddr(r)
	if (ip == "127.0.0.1" || ip == "::1") && st.AdminPassword == "" {
		return true
	}
	return false
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	st := s.Config.Get()
	addr := network.RemoteAddrWithPort(r)
	priv := network.IsPrivateIP(addr, st.PrivateNetworks)
	admin := s.isAdmin(r)
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	scheme := "http"
	if r.URL.Scheme != "" {
		scheme = r.URL.Scheme
	}
	og := OGData{}
	if song := r.URL.Query().Get("song"); validSongID(song) {
		if songObj, fullID, ok := s.Library.FindSongByPartialID(song); ok {
			og = OGData{
				Title: songObj.Title, Description: "Listen to " + songObj.Title + " by " + songObj.Artist,
				Image: fmt.Sprintf("%s://%s/cover/%s", scheme, r.Host, fullID),
				URL:   fmt.Sprintf("%s://%s/?song=%s", scheme, r.Host, fullID),
			}
		}
	}
	if st.EnableRadioStationsHTTP {
		w.Header().Set("Content-Security-Policy", "media-src 'self' http: https: blob:")
	}
	data := PageDataFromConfig(st, og, scheme, host, priv, admin)
	data.SourceHash = s.Hash
	body, err := RenderEP(data)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(body)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")
	s.Hub.Add(c)
	defer s.Hub.Remove(c)
	s.Hub.SendVersion(c)
	s.Hub.SendStatusUpdate(c, s.Library.LoadingStatus())
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			s.Hub.send(c, map[string]string{"event": "ping"})
			s.Hub.SendVersion(c)
		}
	}()
	for {
		_, _, err := c.Read(r.Context())
		if err != nil {
			return
		}
	}
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"source_hash": s.Hash})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Library.LoadingStatus())
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	if !s.Library.IsLibraryReady() {
		writeJSON(w, 200, models.LibraryResult{})
		return
	}
	st := s.Config.Get()
	ids := search.InitialSongIDsFromContext(r, s.DB.DB)
	res := search.ProcessSongs(s.DB.DB, st, r, ids, nil, false, false)
	writeJSON(w, 200, res)
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	name := chi.URLParam(r, "name")
	rows, err := s.DB.Query(`SELECT song_id FROM playlist_songs WHERE playlist_name = ? ORDER BY position`, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	st := s.Config.Get()
	res := search.ProcessSongs(s.DB.DB, st, r, ids, nil, false, false)
	writeJSON(w, 200, res)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	st := s.Config.Get()
	res := search.SearchSongs(s.DB.DB, st, r, r.URL.Query().Get("q"))
	writeJSON(w, 200, res)
}

func (s *Server) handleSong(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	song, _, ok := s.Library.FindSongByPartialID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, 200, song)
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	p, _ := s.Library.GetPlaylists(r.URL.Query().Get("ratings"))
	writeJSON(w, 200, p)
}

func (s *Server) handleGenres(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	g, _ := s.Library.GetDistinct("genre", r.URL.Query().Get("ratings"))
	writeJSON(w, 200, jsonutil.StringSlice(g))
}

func (s *Server) handleArtists(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	a, _ := s.Library.GetDistinct("artist", r.URL.Query().Get("ratings"))
	writeJSON(w, 200, jsonutil.StringSlice(a))
}

func (s *Server) handleAlbums(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	a, _ := s.Library.GetDistinct("album", r.URL.Query().Get("ratings"))
	writeJSON(w, 200, jsonutil.StringSlice(a))
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Type string `json:"type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Type != "play" && body.Type != "skip" {
		writeJSON(w, 400, map[string]string{"error": "Invalid type"})
		return
	}
	s.Library.UpdateSongStats(chi.URLParam(r, "id"), body.Type)
	writeJSON(w, 200, map[string]int{"success": 1})
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) localSongPath(ctx context.Context, st config.Settings, full string) (string, error) {
	path, err := utils.SafeSongPath(s.DB.DB, full, st.MusicDirectories)
	if err != nil || path == "" {
		return "", err
	}
	if s3.IsURI(path) {
		client := s3.Default()
		if client == nil {
			return "", os.ErrNotExist
		}
		return s3.LocalCachePath(ctx, client, config.CacheDir(), path)
	}
	return path, nil
}

func (s *Server) handleCoverExists(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	_, full, ok := s.Library.FindSongByPartialID(id)
	if !ok {
		writeJSON(w, 200, map[string]int{"exists": 0})
		return
	}
	st := s.Config.Get()
	path, _ := s.localSongPath(r.Context(), st, full)
	if path == "" {
		writeJSON(w, 200, map[string]int{"exists": 0})
		return
	}
	_, _, ok2 := utils.CoverArt(path)
	if ok2 {
		writeJSON(w, 200, map[string]int{"exists": 1})
	} else {
		writeJSON(w, 200, map[string]int{"exists": 0})
	}
}

func (s *Server) handleLyrics(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	song, full, ok := s.Library.FindSongByPartialID(id)
	if !ok {
		writeJSON(w, 200, map[string]interface{}{"lyrics": nil})
		return
	}
	st := s.Config.Get()
	if l, ok := lyrics.FromCache(st.LyricsCacheDir, full); ok {
		writeJSON(w, 200, map[string]string{"lyrics": l})
		return
	}
	path, _ := s.localSongPath(r.Context(), st, full)
	l := lyrics.GetLocal(st, path)
	writeJSON(w, 200, map[string]interface{}{"lyrics": l})
	if l == "" && st.EnableOnlineLyricsSearch {
		lyrics.FetchOnlineAsync(st, song.Artist, song.Title, func(text string) {
			if text != "" {
				lyrics.SaveCache(st.LyricsCacheDir, full, text)
				s.Hub.BroadcastLyrics(full, text)
			}
		})
	}
	_ = id
}

func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	song, full, ok := s.Library.FindSongByPartialID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(strings.ToLower(song.Location), "http") {
		http.NotFound(w, r)
		return
	}
	st := s.Config.Get()
	path, err := utils.SafeSongPath(s.DB.DB, full, st.MusicDirectories)
	if err != nil || path == "" {
		http.NotFound(w, r)
		return
	}
	if s3.IsURI(path) {
		if r.URL.Query().Get("download") != "" {
			local, err := s.localSongPath(r.Context(), st, full)
			if err != nil || local == "" {
				http.NotFound(w, r)
				return
			}
			s.serveDownload(w, r, local)
			return
		}
		s.serveS3Audio(w, r, path)
		return
	}
	if r.URL.Query().Get("download") != "" {
		s.serveDownload(w, r, path)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Server) serveS3Audio(w http.ResponseWriter, r *http.Request, uri string) {
	loc, err := s3.ParseURI(uri)
	if err != nil || !s3.ValidKey(loc.Key) {
		http.NotFound(w, r)
		return
	}
	st := s.Config.Get()
	if !s3.Allowed(loc, st.MusicDirectories) {
		http.NotFound(w, r)
		return
	}
	client := s3.Default()
	if client == nil {
		http.NotFound(w, r)
		return
	}
	stream, err := client.GetObject(r.Context(), loc.Bucket, loc.Key, r.Header.Get("Range"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer stream.Body.Close()
	ctype := stream.ContentType
	if ctype == "" {
		ctype = s3.ContentTypeForKey(loc.Key)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Accept-Ranges", "bytes")
	if stream.Partial {
		w.Header().Set("Content-Range", stream.ContentRange)
		w.Header().Set("Content-Length", strconv.FormatInt(stream.ContentLength, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else if stream.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(stream.ContentLength, 10))
	}
	_, _ = io.Copy(w, stream.Body)
}

func (s *Server) serveDownload(w http.ResponseWriter, r *http.Request, src string) {
	if s3.IsURI(src) {
		client := s3.Default()
		if client == nil {
			http.NotFound(w, r)
			return
		}
		local, err := s3.LocalCachePath(r.Context(), client, config.CacheDir(), src)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		src = local
	}
	tmpDir := filepath.Join(config.CacheDir(), "downloads")
	_ = os.MkdirAll(tmpDir, 0o755)
	tmp, err := os.CreateTemp(tmpDir, "dl-*.mp3")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpPath := tmp.Name()
	tmp.Close()
	in, _ := os.Open(src)
	out, _ := os.Create(tmpPath)
	_, _ = io.Copy(out, in)
	in.Close()
	out.Close()
	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, tmpPath)
}

func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	st := s.Config.Get()
	if !st.EnableCoverArt {
		http.NotFound(w, r)
		return
	}
	_, full, ok := s.Library.FindSongByPartialID(chi.URLParam(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	path, _ := s.localSongPath(r.Context(), st, full)
	data, mime, ok := utils.CoverArt(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Write(data)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	st := s.Config.Get()
	addr := network.RemoteAddrWithPort(r)
	priv := network.IsPrivateIP(addr, st.PrivateNetworks)
	if !s.isAdmin(r) && !(priv && st.AllowLocalNetworkEditing && st.AdminPassword != "") {
		http.Error(w, "Forbidden", 403)
		return
	}
	writeJSON(w, 200, map[string]interface{}{
		"settings": config.SettingsSchema(),
		"values":   config.ClientValues(st),
		"path":     st.UserConfigFile,
		"default_music_dir": config.DefaultMusicDir(),
	})
}

func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	st := s.Config.Get()
	addr := network.RemoteAddrWithPort(r)
	priv := network.IsPrivateIP(addr, st.PrivateNetworks)
	if !s.isAdmin(r) && !(priv && st.AllowLocalNetworkEditing && st.AdminPassword != "") {
		http.Error(w, "Forbidden", 403)
		return
	}
	var vals map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&vals); err != nil {
		writeJSON(w, 500, map[string]interface{}{"success": 0, "message": err.Error()})
		return
	}
	oldDirs := st.MusicDirectories
	if err := s.Config.Update(vals); err != nil {
		writeJSON(w, 500, map[string]interface{}{"success": 0, "message": err.Error()})
		return
	}
	st = s.Config.Get()
	_ = s.Library.SyncLibraryToConfig(oldDirs, st.MusicDirectories, nil, friendURLs(st.FriendsMusic))
	hash := s.Config.ConfigHash()
	_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES ('config_hash', ?)`, hash)
	writeJSON(w, 200, map[string]interface{}{"success": 1, "message": "Configuration saved. The page will now reload."})
}

func friendURLs(f []config.FriendMusic) []string {
	var u []string
	for _, x := range f {
		u = append(u, x.Value)
	}
	return u
}

func (s *Server) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	st := s.Config.Get()
	addr := network.RemoteAddrWithPort(r)
	if !st.AllowAdminFromAnyNetwork && !network.IsPrivateIP(addr, st.PrivateNetworks) {
		http.Error(w, "Forbidden", 403)
		return
	}
	if st.AdminPassword != "" {
		writeJSON(w, 200, map[string]interface{}{"success": 0, "message": "Password is already set."})
		return
	}
    var body struct {
        Password string `json:"password"`
    }
    _ = json.NewDecoder(r.Body).Decode(&body)
    if len(body.Password) < 4 {
        writeJSON(w, 200, map[string]interface{}{"success": 0, "message": "Password must be at least 4 characters."})
        return
    }
    hashed, ok := resolvePasswordInputFromConfig(r, body.Password, s.Config)
    if !ok {
        writeJSON(w, 403, map[string]interface{}{"success": 0, "message": "Password must be sent as a hash over the public internet. Use HTTPS, localhost, or access from your local network."})
        return
    }
    _ = s.Config.Update(map[string]interface{}{"ADMIN_PASSWORD": hashed})
	writeJSON(w, 200, map[string]interface{}{"success": 1, "message": "Admin password set successfully."})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	st := s.Config.Get()
	addr := network.RemoteAddrWithPort(r)
	if !st.AllowAdminFromAnyNetwork && !network.IsPrivateIP(addr, st.PrivateNetworks) {
		http.Error(w, "Forbidden", 403)
		return
	}
	time.Sleep(2 * time.Second)
        var body struct {
        Password string `json:"password"`
    }
    _ = json.NewDecoder(r.Body).Decode(&body)
    hashed, hashOK := resolvePasswordInputFromConfig(r, body.Password, s.Config)
    if !hashOK {
        writeJSON(w, 403, map[string]interface{}{
            "success": 0, "message": "Password must be sent as a hash over the public internet. Use HTTPS, localhost, or access from your local network.",
        })
        return
    }
    ip := network.RemoteAddr(r)
    if until := s.Login.LockoutUntil(ip); !until.IsZero() {
        writeJSON(w, 200, map[string]interface{}{
            "success": 0, "message": "Too many failed attempts.", "lockout_until": until.Unix(),
        })
        return
    }
    ok, msg, until := s.Login.ProcessLogin(ip, hashed, st.AdminPassword)
	if ok {
		s.Sessions.Put(r.Context(), "admin", true)
	}
	writeJSON(w, 200, map[string]interface{}{
		"success": ok, "message": msg, "is_admin": ok, "lockout_until": until.Unix(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.Sessions.Remove(r.Context(), "admin")
	writeJSON(w, 200, map[string]interface{}{"success": 1, "message": "Logged out."})
}

func (s *Server) handleUPnP(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "Forbidden", 403)
		return
	}
	st := s.Config.Get()
	errMsg := network.SetupUPnP(st.ServerPort, s.Logger)
	if errMsg != "" {
		writeJSON(w, 200, map[string]interface{}{"success": 0, "message": errMsg})
		return
	}
	writeJSON(w, 200, map[string]interface{}{"success": 1, "message": "UPnP port forwarding configured successfully."})
}

func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	info := utils.CheckUpdate()
	info.URL = "https://github.com/Elive/synthwave-player/releases/latest"
	info.PatreonURL = "https://www.patreon.com/c/elive"
	writeJSON(w, 200, info)
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "Forbidden", 403)
		return
	}
	s.Library.DeepRescan()
	writeJSON(w, 200, map[string]interface{}{"success": 1, "message": "Deep re-scan started."})
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		writeJSON(w, 403, map[string]interface{}{"success": 0, "message": "Forbidden"})
		return
	}
	var body struct {
		TagName  string `json:"tag_name"`
		TagValue string `json:"tag_value"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	id := chi.URLParam(r, "id")
	song, full, ok := s.Library.FindSongByPartialID(id)
	if !ok {
		writeJSON(w, 200, map[string]interface{}{"success": 0, "message": "Song not found"})
		return
	}
	if s3.IsURI(song.Location) {
		writeJSON(w, 200, map[string]interface{}{"success": 0, "message": "Tag editing is not supported for S3-backed songs"})
		return
	}
	st := s.Config.Get()
	path, err := s.localSongPath(r.Context(), st, full)
	if err != nil || path == "" {
		writeJSON(w, 500, map[string]interface{}{"success": 0, "message": "Could not access file"})
		return
	}
	// Use eyeD3 when available for tag writes
	if st.EyeD3Available {
		args := []string{"--quiet", "--no-color"}
		switch body.TagName {
		case "title":
			args = append(args, "-t", body.TagValue)
		case "artist":
			args = append(args, "-a", body.TagValue)
		case "album":
			args = append(args, "-A", body.TagValue)
		case "genre":
			args = append(args, "-G", body.TagValue)
		case "rating":
			args = append(args, "--set-text-frame", "rating:"+body.TagValue)
		}
		args = append(args, path)
		_ = exec.Command("eyeD3", args...).Run()
	}
	writeJSON(w, 200, map[string]interface{}{"success": 1})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func validSongID(id string) bool {
	if len(id) == 0 || len(id) > 16 {
		return false
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

var _ = strconv.Itoa
