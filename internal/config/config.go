package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/adrg/xdg"
	"github.com/Elive/synthwave-player/internal/storage/s3"
)

const AppVersion = "v3.6"

type FriendMusic struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Settings holds runtime configuration (Perl package globals).
type Settings struct {
	UserFullName                   string        `json:"USER_FULL_NAME"`
	MusicDirectories               []string      `json:"MUSIC_DIRECTORIES"`
	WelcomeMessage                 string        `json:"WELCOME_MESSAGE"`
	UIFontFamily                   string        `json:"UI_FONT_FAMILY"`
	UIFontSizeDesktop              string        `json:"UI_FONT_SIZE_DESKTOP"`
	UIFontSizeMobile               string        `json:"UI_FONT_SIZE_MOBILE"`
	LyricsFontFamily               string        `json:"LYRICS_FONT_FAMILY"`
	LyricsFontSizeDesktop          string        `json:"LYRICS_FONT_SIZE_DESKTOP"`
	LyricsFontSizeMobile           string        `json:"LYRICS_FONT_SIZE_MOBILE"`
	EnableCoverArt                 bool          `json:"ENABLE_COVER_ART"`
	EnableRadioStationsHTTP        bool          `json:"ENABLE_RADIO_STATIONS_HTTP"`
	EnableOnlineLyricsSearch       bool          `json:"ENABLE_ONLINE_LYRICS_SEARCH"`
	EnableDebugLogging             bool          `json:"ENABLE_DEBUG_LOGGING"`
	EnableVerboseLogging           bool          `json:"ENABLE_VERBOSE_LOGGING"`
	ServerWorkers                  int           `json:"SERVER_WORKERS"`
	DefaultSortBy                  string        `json:"DEFAULT_SORT_BY"`
	DefaultSortOrder               string        `json:"DEFAULT_SORT_ORDER"`
	ScrollToSongAlignMobile        float64       `json:"SCROLL_TO_SONG_ALIGN_MOBILE"`
	ScrollToSongAlignDesktop       float64       `json:"SCROLL_TO_SONG_ALIGN_DESKTOP"`
	SongsPerPage                   int           `json:"SONGS_PER_PAGE"`
	MinSearchLength                int           `json:"MIN_SEARCH_LENGTH"`
	PreviousButtonRestartSeconds   int           `json:"PREVIOUS_BUTTON_RESTART_SECONDS"`
	IgnorePlaybackErrorsMS         int           `json:"IGNORE_PLAYBACK_ERRORS_MS"`
	NotificationVisibleMS          int           `json:"NOTIFICATION_VISIBLE_MS"`
	SidebarAutoCollapseDelayMS     int           `json:"SIDEBAR_AUTO_COLLAPSE_DELAY_MS"`
	PeriodicUpdateCheckHours       int           `json:"PERIODIC_UPDATE_CHECK_HOURS"`
	InitialSongsToLoadDesktop      int           `json:"INITIAL_SONGS_TO_LOAD_DESKTOP"`
	InitialSongsToLoadMobile       int           `json:"INITIAL_SONGS_TO_LOAD_MOBILE"`
	SongsToLoadOnScrollDesktop     int           `json:"SONGS_TO_LOAD_ON_SCROLL_DESKTOP"`
	SongsToLoadOnScrollMobile      int           `json:"SONGS_TO_LOAD_ON_SCROLL_MOBILE"`
	ShowPlaybackSpeedMinMinutes    int           `json:"SHOW_PLAYBACK_SPEED_MIN_MINUTES"`
	ServerHost                     string        `json:"SERVER_HOST"`
	ServerPort                     int           `json:"SERVER_PORT"`
	EnableUPnP                     bool          `json:"ENABLE_UPNP"`
	UPnPCheckIntervalHours         int           `json:"UPNP_CHECK_INTERVAL_HOURS"`
	FriendsMusic                   []FriendMusic `json:"FRIENDS_MUSIC"`
	PlaylistsDirectories           []string      `json:"PLAYLISTS_DIRECTORIES"`
	IgnorePlaylistsMatching        []string      `json:"IGNORE_PLAYLISTS_MATCHING"`
	IgnoreGenresMatching           []string      `json:"IGNORE_GENRES_MATCHING"`
	BlacklistPlaylistsMatching     []string      `json:"BLACKLIST_PLAYLISTS_MATCHING"`
	BlacklistGenresMatching        []string      `json:"BLACKLIST_GENRES_MATCHING"`
	BlacklistArtistsMatching       []string      `json:"BLACKLIST_ARTISTS_MATCHING"`
	RateLimitRequests              int           `json:"RATE_LIMIT_REQUESTS"`
	RateLimitWindow                int           `json:"RATE_LIMIT_WINDOW"`
	GlobalRateLimitRequests        int           `json:"GLOBAL_RATE_LIMIT_REQUESTS"`
	GlobalRateLimitWindow          int           `json:"GLOBAL_RATE_LIMIT_WINDOW"`
	MaxSimultaneousIPs             int           `json:"MAX_SIMULTANEOUS_IPS"`
	PrivateNetworks                []string      `json:"PRIVATE_NETWORKS"`
	AdminPassword                  string        `json:"ADMIN_PASSWORD"`
	AllowAdminFromAnyNetwork       bool          `json:"ALLOW_ADMIN_FROM_ANY_NETWORK"`
	AllowLocalNetworkEditing       bool          `json:"ALLOW_LOCAL_NETWORK_EDITING"`
	ShowPlaylistsOutOfDateWarning  bool          `json:"SHOW_PLAYLISTS_OUT_OF_DATE_WARNING"`

	WebsiteTitle              string
	UserConfigFile            string
	LyricsCacheDir            string
	LyricsFailCacheDir        string
	LyricsInstrumentalCacheDir string
	EyeD3Available            bool
	UPnPErrorMessage          string
}

type Store struct {
	mu       sync.RWMutex
	Settings Settings
	lastMtime int64
}

func NewStore() *Store {
	return &Store{Settings: defaults()}
}

func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Settings
}

func (s *Store) Load(logger *slog.Logger) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	configDir := filepath.Join(xdg.ConfigHome, "synthwave-player")
	configFile := filepath.Join(configDir, "server-config.json")
	s.Settings.UserConfigFile = configFile

	var mtime int64
	if st, err := os.Stat(configFile); err == nil {
		mtime = st.ModTime().Unix()
	}
	if s.lastMtime != 0 && s.lastMtime == mtime && mtime != 0 {
		return nil
	}
	if s.lastMtime != 0 && logger != nil {
		logger.Info("configuration file changed, reloading", "mtime", mtime)
	}
	s.lastMtime = mtime

	def := defaults()
	s.Settings = def
	s.Settings.UserConfigFile = configFile

	_ = os.MkdirAll(configDir, 0o755)
	isNew := true
	user := map[string]json.RawMessage{}
	if data, err := os.ReadFile(configFile); err == nil {
		isNew = false
		_ = json.Unmarshal(data, &user)
	}

	merged := structToMap(def)
	needsSave := isNew
	for k, v := range user {
		if k == "ADMIN_PASSWORD" {
			var pass string
			if json.Unmarshal(v, &pass) == nil && pass != "" && len(pass) != 64 {
				h := sha256.Sum256([]byte(pass))
				pass = hex.EncodeToString(h[:])
				b, _ := json.Marshal(pass)
				user[k] = b
				needsSave = true
			}
		}
		merged[k] = v
	}
	for k := range merged {
		if _, ok := user[k]; !ok {
			needsSave = true
		}
	}

	raw, _ := json.Marshal(merged)
	var loaded Settings
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return err
	}
	loaded.UserConfigFile = configFile
	loaded.ServerHost = NormalizeListenHost(loaded.ServerHost)
	applyDerived(&loaded, logger, isNew, configDir)

	if len(loaded.MusicDirectories) == 0 {
		if dirs := DiscoverMusicDirectories(); len(dirs) > 0 {
			loaded.MusicDirectories = dirs
			needsSave = true
			if logger != nil {
				logger.Info("auto-configured music directories", "dirs", dirs)
			}
		}
	}
	if len(loaded.PlaylistsDirectories) == 0 {
		if dirs := DiscoverPlaylistDirectories(loaded.MusicDirectories); len(dirs) > 0 {
			loaded.PlaylistsDirectories = dirs
			needsSave = true
			if logger != nil {
				logger.Info("auto-configured playlist directories", "dirs", dirs)
			}
		}
	}
	if s3cfg := s3.ConfigFromEnv(); s3cfg.Enabled() {
		s3URI := s3cfg.MusicURI()
		hasS3 := false
		for _, d := range loaded.MusicDirectories {
			if s3.IsURI(d) {
				hasS3 = true
				break
			}
		}
		if !hasS3 && os.Getenv("SYNTHWAVE_USE_S3_MUSIC") != "0" {
			loaded.MusicDirectories = append(loaded.MusicDirectories, s3URI)
			needsSave = true
			if logger != nil {
				logger.Info("auto-configured s3 music source", "uri", s3URI)
			}
		}
	}
	if needsSave {
		if b, err := json.Marshal(loaded.MusicDirectories); err == nil {
			merged["MUSIC_DIRECTORIES"] = b
		}
		if b, err := json.Marshal(loaded.PlaylistsDirectories); err == nil {
			merged["PLAYLISTS_DIRECTORIES"] = b
		}
		if err := saveJSON(configFile, merged); err != nil && logger != nil {
			logger.Warn("could not write config", "err", err)
		}
	}

	s.Settings = loaded
	s.Settings.LyricsCacheDir = filepath.Join(CacheDir(), "lyrics")
	s.Settings.LyricsFailCacheDir = filepath.Join(CacheDir(), "lyrics-fail")
	s.Settings.LyricsInstrumentalCacheDir = filepath.Join(CacheDir(), "lyrics-instrumental")
	for _, d := range []string{s.Settings.LyricsCacheDir, s.Settings.LyricsFailCacheDir, s.Settings.LyricsInstrumentalCacheDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	s.Settings.EyeD3Available = lookPath("eyeD3")
	return nil
}

func applyDerived(st *Settings, logger *slog.Logger, isNew bool, configDir string) {
	name := st.UserFullName
	if name == "" {
		name = userFullNameFromPasswd()
	}
	st.WebsiteTitle = name + " Music"
	maxInit := st.InitialSongsToLoadDesktop
	if st.InitialSongsToLoadMobile > maxInit {
		maxInit = st.InitialSongsToLoadMobile
	}
	if maxInit > 100 {
		st.SongsPerPage = maxInit
	} else {
		st.SongsPerPage = 100
	}
	if st.PeriodicUpdateCheckHours < 2 {
		st.PeriodicUpdateCheckHours = 2
	}
	if isNew && logger != nil {
		logger.Info("user configuration created", "path", st.UserConfigFile)
	}
}

func defaults() Settings {
	musicDirs := []string{}
	if d := xdgMusicDir(); d != "" {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			musicDirs = append(musicDirs, d)
		}
	}
	if st, err := os.Stat("/usr/share/elive-demo-files-skel/Music"); err == nil && st.IsDir() {
		musicDirs = append(musicDirs, "/usr/share/elive-demo-files-skel/Music")
	}
	playlistDirs := []string{}
	home, _ := os.UserHomeDir()
	pd := filepath.Join(home, "Music", "playlists")
	if st, err := os.Stat(pd); err == nil && st.IsDir() {
		playlistDirs = append(playlistDirs, pd)
	}
	if st, err := os.Stat("/usr/share/elive-demo-files-skel/Music"); err == nil && st.IsDir() {
		playlistDirs = append(playlistDirs, "/usr/share/elive-demo-files-skel/Music")
	}
	return Settings{
		MusicDirectories:             musicDirs,
		WelcomeMessage:                 "",
		UIFontFamily:                   "Roboto",
		UIFontSizeDesktop:              "1.00em",
		UIFontSizeMobile:               "0.95em",
		LyricsFontFamily:               "Overlock",
		LyricsFontSizeDesktop:          "1.15em",
		LyricsFontSizeMobile:           "1.00em",
		EnableCoverArt:                 true,
		EnableOnlineLyricsSearch:       true,
		ServerWorkers:                  1,
		DefaultSortBy:                  "folder",
		DefaultSortOrder:               "asc",
		ScrollToSongAlignMobile:        0.5,
		ScrollToSongAlignDesktop:       0.25,
		MinSearchLength:                2,
		PreviousButtonRestartSeconds:   20,
		IgnorePlaybackErrorsMS:         300,
		NotificationVisibleMS:          6000,
		SidebarAutoCollapseDelayMS:   2000,
		PeriodicUpdateCheckHours:       72,
		InitialSongsToLoadDesktop:      60,
		InitialSongsToLoadMobile:       30,
		SongsToLoadOnScrollDesktop:     100,
		SongsToLoadOnScrollMobile:      50,
		ShowPlaybackSpeedMinMinutes:    20,
		ServerHost:                     "0.0.0.0",
		ServerPort:                     8160,
		UPnPCheckIntervalHours:         2,
		FriendsMusic: []FriendMusic{
			{Name: "🧙 Thanatermesis", Value: "https://music.home.thanatermesis.org"},
			{Name: "🫥 John Smith", Value: "https://placehold.co/500x500/1a0e2a/ff00ff?text=John+Smith+doesnt+exist"},
			{Name: "🏍 Nightride FM", Value: "https://nightride.fm/eq?station=nightride"},
		},
		PlaylistsDirectories:       playlistDirs,
		IgnorePlaylistsMatching:    []string{"ORD", "^tmp/i", "temporal/i", "stars/i", "iphone/i", "AI", "Recently/i"},
		IgnoreGenresMatching:       []string{"^AI$/i", "^rare$/i", "^other$/i"},
		RateLimitRequests:          240,
		RateLimitWindow:            60,
		GlobalRateLimitRequests:    1000,
		GlobalRateLimitWindow:      60,
		MaxSimultaneousIPs:         50,
		PrivateNetworks: []string{
			`^127\.0\.0\.1(:[0-9]+)?$`,
			`^::1(:[0-9]+)?$`,
			`^10\.`,
			`^172\.(1[6-9]|2[0-9]|3[0-1])\.`,
			`^192\.168\.`,
			`^169\.254\.`,
			`^100\.(6[4-9]|[7-9][0-9]|1[0-1][0-9]|12[0-7])\.`,
			`^f[cd][0-9a-f]{2}:/i`,
			`^fe[89ab][0-9a-f]:/i`,
		},
		SongsPerPage: 100,
	}
}

func CacheDir() string {
	cache, err := xdg.CacheFile("synthwave-player/.keep")
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".cache", "synthwave-player")
	}
	return filepath.Dir(cache)
}

func DBPath() string { return filepath.Join(CacheDir(), "library.db") }

func (s *Store) ConfigHash() string {
	st := s.Get()
	h := sha256.New()
	for _, d := range st.MusicDirectories {
		h.Write([]byte(d + "\n"))
	}
	for _, f := range st.FriendsMusic {
		h.Write([]byte(f.Value + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) Update(values map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.Settings.UserConfigFile)
	var current map[string]json.RawMessage
	if err != nil {
		current = structToMap(defaults())
	} else {
		_ = json.Unmarshal(raw, &current)
	}
	for k, v := range values {
		if k == "SERVER_HOST" {
			if s, ok := v.(string); ok {
				v = NormalizeListenHost(s)
			}
		}
		if k == "ADMIN_PASSWORD" {
			if pass, ok := v.(string); ok && pass != "" && len(pass) != 64 {
				sum := sha256.Sum256([]byte(pass))
				v = hex.EncodeToString(sum[:])
			}
		}
		b, _ := json.Marshal(v)
		current[k] = b
	}
	if err := saveJSON(s.Settings.UserConfigFile, current); err != nil {
		return err
	}
	s.lastMtime = 0
	return nil
}

func ClientValues(st Settings) map[string]interface{} {
	b, _ := json.Marshal(st)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}

func CheckDependencies(logger *slog.Logger) {
	if _, err := exec.LookPath("file"); err != nil {
		logger.Error("required command 'file' not found in PATH")
		os.Exit(1)
	}
	if _, err := exec.LookPath("eyeD3"); err != nil {
		logger.Warn("optional command 'eyeD3' not found; lyrics will use fallback methods")
	}
}

func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func xdgMusicDir() string {
	configHome := xdg.ConfigHome
	f := filepath.Join(configHome, "user-dirs.dirs")
	data, err := os.ReadFile(f)
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Music")
	}
	home, _ := os.UserHomeDir()
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, `XDG_MUSIC_DIR="`) {
			p := strings.Trim(line, `"`)
			p = strings.TrimPrefix(p, "XDG_MUSIC_DIR=")
			p = strings.Trim(p, `"`)
			p = strings.ReplaceAll(p, "$HOME", home)
			return p
		}
	}
	return filepath.Join(home, "Music")
}

func userFullNameFromPasswd() string {
	u := os.Getenv("USER")
	if u == "" {
		return "My"
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return u
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) >= 5 && parts[0] == u {
			name := strings.Split(parts[4], ",")[0]
			return strings.TrimSpace(name)
		}
	}
	return u
}

func structToMap(st Settings) map[string]json.RawMessage {
	b, _ := json.Marshal(st)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	return m
}

func saveJSON(path string, m map[string]json.RawMessage) error {
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	out := make(map[string]interface{})
	for k, v := range m {
		var any interface{}
		_ = json.Unmarshal(v, &any)
		out[k] = any
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) MaybeReload(logger *slog.Logger) {
	_ = s.Load(logger)
}

// DefaultMusicDir returns XDG music path for API.
func DefaultMusicDir() string { return xdgMusicDir() }
