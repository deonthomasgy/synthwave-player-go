package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Elive/synthwave-player/internal/paths"
)

// RenderEP serves the Mojolicious template with Go-filled variables.
func RenderEP(data PageData) ([]byte, error) {
	indexPath := filepath.Join(paths.TemplatesDir(), "index_synthwave.html.ep")
	modalsPath := filepath.Join(paths.TemplatesDir(), "partials", "modals_synthwave.html.ep")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	modals, err := os.ReadFile(modalsPath)
	if err != nil {
		return nil, err
	}
	s := string(index)
	s = strings.Replace(s, `<%= include 'partials/modals_synthwave' %>`, string(modals), 1)

	repl := map[string]string{
		"website_title": data.WebsiteTitle, "app_version": data.AppVersion,
		"ui_font_size_desktop": data.UIFontSizeDesktop, "ui_font_size_mobile": data.UIFontSizeMobile,
		"ui_font_family": data.UIFontFamily, "lyrics_font_size_desktop": data.LyricsFontSizeDesktop,
		"lyrics_font_size_mobile": data.LyricsFontSizeMobile, "lyrics_font_family": data.LyricsFontFamily,
		"welcome_message": data.WelcomeMessage, "default_sort_by": data.DefaultSortBy,
		"default_sort_order": data.DefaultSortOrder, "protocol": data.Protocol,
		"website_domain": data.WebsiteDomain, "source_hash": data.SourceHash,
		"upnp_error_message": data.UPnPErrorMessage,
	}
	for k, v := range repl {
		s = strings.ReplaceAll(s, "<%= $"+k+" %>", v)
		s = strings.ReplaceAll(s, "<%= $"+k+" =~ s/ /-/gr %>", strings.ReplaceAll(v, " ", "-"))
	}

	proRepo := "Elive"
	if data.IsPro {
		proRepo = "Elive-Premium"
	}
	s = strings.ReplaceAll(s, "<%= $is_pro ? 'Elive-Premium' : 'Elive' %>", proRepo)

	boolRepl := map[string]bool{
		"enable_online_lyrics_search": data.EnableOnlineLyricsSearch,
		"enable_cover_art":            data.EnableCoverArt,
		"admin_password_is_set":       data.AdminPasswordIsSet,
		"allow_admin_from_any_network": data.AllowAdminFromAnyNetwork,
		"allow_local_network_editing": data.AllowLocalNetworkEditing,
		"show_playlists_out_of_date_warning": data.ShowPlaylistsOutOfDateWarning,
		"is_private_network":          data.IsPrivateNetwork,
		"is_admin":                    data.IsAdmin,
		"is_pro":                      data.IsPro,
		"is_elive":                    data.IsElive,
	}
	for k, v := range boolRepl {
		tv := "false"
		if v {
			tv = "true"
		}
		s = strings.ReplaceAll(s, "<%= $"+k+" ? 'true' : 'false' %>", tv)
	}

	numRepl := map[string]interface{}{
		"initial_songs_to_load_mobile": data.InitialSongsToLoadMobile,
		"initial_songs_to_load_desktop": data.InitialSongsToLoadDesktop,
		"songs_to_load_on_scroll_mobile": data.SongsToLoadOnScrollMobile,
		"songs_to_load_on_scroll_desktop": data.SongsToLoadOnScrollDesktop,
		"show_playback_speed_min_minutes": data.ShowPlaybackSpeedMinMinutes,
		"min_search_length": data.MinSearchLength, "songs_per_page": data.SongsPerPage,
		"previous_button_restart_seconds": data.PreviousButtonRestartSeconds,
		"ignore_playback_errors_ms": data.IgnorePlaybackErrorsMS,
		"notification_visible_ms": data.NotificationVisibleMS,
		"scroll_to_song_align_mobile": data.ScrollToSongAlignMobile,
		"scroll_to_song_align_desktop": data.ScrollToSongAlignDesktop,
		"periodic_update_check_hours": data.PeriodicUpdateCheckHours,
		"sidebar_auto_collapse_delay_ms": data.SidebarAutoCollapseDelayMS,
	}
	for k, v := range numRepl {
		s = strings.ReplaceAll(s, "<%= $"+k+" %>", fmt.Sprint(v))
	}

	showInit := data.ShowSetInitialPasswordModal
	showUPnP := data.ShowUpnpErrorModal
	s = strings.ReplaceAll(s, "<%= !$admin_password_is_set && ($is_private_network || $allow_admin_from_any_network) ? 'true' : 'false' %>", tf(showInit))
	s = strings.ReplaceAll(s, "<%= ($upnp_error_message && $is_private_network) ? 'true' : 'false' %>", tf(showUPnP))
	s = strings.ReplaceAll(s, "<%= ($friends_music && @{$friends_music}) ? 'true' : 'false' %>", tf(hasFriendsMusic(data.FriendsJSON)))

	friendsEsc := strings.ReplaceAll(data.FriendsJSON, `'`, `\'`)
	s = strings.Replace(s, `<%== Mojo::Util::decode("UTF-8", Mojo::JSON::encode_json($friends_music || [])) =~ s/'/\\'/gr %>`,
		friendsEsc, 1)
	s = strings.ReplaceAll(s, `<%= session('user') || '' %>`, "")

	// Google fonts block
	if data.GoogleFontsURL != "" {
		gblock := fmt.Sprintf(`<link href="%s" rel="stylesheet">`, data.GoogleFontsURL)
		s = regexp.MustCompile(`(?s)% my @gfonts;.*?% }`).ReplaceAllString(s, gblock)
	} else {
		s = regexp.MustCompile(`(?s)% my @gfonts;.*?% }`).ReplaceAllString(s, "")
	}

	// OG tags
	if data.OG.Title != "" {
		og := fmt.Sprintf(`<meta property="og:title" content="%s" />
    <meta property="og:type" content="music.song" />
    <meta property="og:url" content="%s" />
    <meta property="og:image" content="%s" />
    <meta property="og:description" content="%s" />`,
			esc(data.OG.Title), esc(data.OG.URL), esc(data.OG.Image), esc(data.OG.Description))
		s = regexp.MustCompile(`(?s)% if \(my \$title = \$og_data->\{title\}\) \{.*?% } else \{.*?% }`).ReplaceAllString(s, og)
	} else {
		def := fmt.Sprintf(`<meta property="og:title" content="%s" />
    <meta property="og:type" content="website" />
    <meta property="og:url" content="%s://%s" />
    <meta property="og:image" content="https://raw.githubusercontent.com/Elive/synthwave-player/635ed6447d45256f54e9d6099d1052e0c2a41deb/media/synthwave-player-multiple-interfaces.png" />
    <meta property="og:description" content="My personal Music Player full of unique features" />`,
			esc(data.WebsiteTitle), data.Protocol, esc(data.WebsiteDomain))
		s = regexp.MustCompile(`(?s)% if \(my \$title = \$og_data->\{title\}\) \{.*?% } else \{.*?% }`).ReplaceAllString(s, def)
	}

	s = processEPConditionals(s, data)
	s = stripRemainingEPTags(s)

	var buf bytes.Buffer
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "%") {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

func processEPConditionals(s string, data PageData) string {
	simple := []struct {
		open string
		keep bool
	}{
		{`<% if (!$is_private_network) { %>`, !data.IsPrivateNetwork},
		{`<% if ($is_private_network || $allow_admin_from_any_network) { %>`, data.IsPrivateNetwork || data.AllowAdminFromAnyNetwork},
		{`<% unless ($is_pro) { %>`, !data.IsPro},
		{`<% if ($enable_cover_art) { %>`, data.EnableCoverArt},
		{`<% if ($enable_online_lyrics_search) { %>`, data.EnableOnlineLyricsSearch},
		{`<% if ($welcome_message) { %>`, data.WelcomeMessage != ""},
		{`<% if ($upnp_error_message && $is_private_network) { %>`, data.ShowUpnpErrorModal},
		{`<% if ($is_private_network) { %>`, data.IsPrivateNetwork},
	}
	for _, c := range simple {
		s = processConditional(s, c.open, `<% } %>`, c.keep)
	}
	for strings.Contains(s, `<% if ($is_pro) { %>`) {
		s = processIfElse(s, `<% if ($is_pro) { %>`, `<% } else { %>`, `<% } %>`, data.IsPro)
	}
	return s
}

func processConditional(s, open, close string, keep bool) string {
	re := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(open) + `(.*?)` + regexp.QuoteMeta(close))
	if !keep {
		return re.ReplaceAllString(s, "")
	}
	return re.ReplaceAllString(s, "$1")
}

func processIfElse(s, openIf, openElse, close string, useIfBranch bool) string {
	re := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(openIf) + `(.*?)` + regexp.QuoteMeta(openElse) + `(.*?)` + regexp.QuoteMeta(close))
	if useIfBranch {
		return re.ReplaceAllString(s, "$1")
	}
	return re.ReplaceAllString(s, "$2")
}

func stripRemainingEPTags(s string) string {
	s = regexp.MustCompile(`<%[^%]*%>`).ReplaceAllString(s, "")
	return s
}

func hasFriendsMusic(friendsJSON string) bool {
	friendsJSON = strings.TrimSpace(friendsJSON)
	if friendsJSON == "" || friendsJSON == "[]" || friendsJSON == "null" {
		return false
	}
	var arr []json.RawMessage
	return json.Unmarshal([]byte(friendsJSON), &arr) == nil && len(arr) > 0
}

func tf(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func esc(s string) string {
	b, _ := json.Marshal(s)
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return s
}
