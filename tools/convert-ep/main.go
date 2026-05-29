// convert-ep translates Mojolicious EP templates to Go html/template syntax.
package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
)

var varMap = map[string]string{
	"website_title": "WebsiteTitle", "app_version": "AppVersion",
	"ui_font_size_desktop": "UIFontSizeDesktop", "ui_font_size_mobile": "UIFontSizeMobile",
	"ui_font_family": "UIFontFamily", "lyrics_font_size_desktop": "LyricsFontSizeDesktop",
	"lyrics_font_size_mobile": "LyricsFontSizeMobile", "lyrics_font_family": "LyricsFontFamily",
	"welcome_message": "WelcomeMessage", "default_sort_by": "DefaultSortBy",
	"default_sort_order": "DefaultSortOrder", "protocol": "Protocol", "website_domain": "WebsiteDomain",
	"initial_songs_to_load_mobile": "InitialSongsToLoadMobile", "initial_songs_to_load_desktop": "InitialSongsToLoadDesktop",
	"songs_to_load_on_scroll_mobile": "SongsToLoadOnScrollMobile", "songs_to_load_on_scroll_desktop": "SongsToLoadOnScrollDesktop",
	"show_playback_speed_min_minutes": "ShowPlaybackSpeedMinMinutes",
	"min_search_length": "MinSearchLength", "songs_per_page": "SongsPerPage",
	"previous_button_restart_seconds": "PreviousButtonRestartSeconds",
	"ignore_playback_errors_ms": "IgnorePlaybackErrorsMS",
	"notification_visible_ms": "NotificationVisibleMS",
	"scroll_to_song_align_mobile": "ScrollToSongAlignMobile",
	"scroll_to_song_align_desktop": "ScrollToSongAlignDesktop",
	"periodic_update_check_hours": "PeriodicUpdateCheckHours",
	"source_hash": "SourceHash", "upnp_error_message": "UPnPErrorMessage",
}

func main() {
	if len(os.Args) < 3 {
		os.Exit(1)
	}
	in, _ := os.ReadFile(os.Args[1])
	out := convert(string(in))
	_ = os.WriteFile(os.Args[2], []byte(out), 0o644)
}

func convert(s string) string {
	// Remove perl blocks for google fonts - replace with template
	s = regexp.MustCompile(`(?s)% my @gfonts;.*?% }`).ReplaceAllString(s, `{{template "gfonts" .}}`)
	s = strings.ReplaceAll(s, `<%= include 'partials/modals_synthwave' %>`, `{{template "modals" .}}`)

	// og_data blocks
	s = regexp.MustCompile(`(?s)% if \(my \$title = \$og_data->\{title\}\) \{.*?% } else \{.*?% }`).
		ReplaceAllString(s, `{{template "ogtags" .}}`)

	// Simple if/unless
	s = regexp.MustCompile(`<% if \(!\$is_private_network\) \{ %>`).ReplaceAllString(s, `{{if not .IsPrivateNetwork}}`)
	s = regexp.MustCompile(`<% unless \(\$is_pro\) \{ %>`).ReplaceAllString(s, `{{if not .IsPro}}`)
	s = regexp.MustCompile(`<% \} %>`).ReplaceAllString(s, `{{end}}`)

	// Ternary in <%= %>
	reTern := regexp.MustCompile(`<%= (\$[a-z_]+) \? 'true' : 'false' %>`)
	s = reTern.ReplaceAllStringFunc(s, func(m string) string {
		sub := reTern.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		name := strings.TrimPrefix(sub[1], "$")
		if f, ok := varMap[name]; ok {
			return `{{if .` + f + `}}true{{else}}false{{end}}`
		}
		return m
	})

	// <%= $var %>
	reVar := regexp.MustCompile(`<%= \$([a-z_]+) %>`)
	s = reVar.ReplaceAllStringFunc(s, func(m string) string {
		sub := reVar.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if f, ok := varMap[sub[1]]; ok {
			return `{{.` + f + `}}`
		}
		return `{{.` + strings.Title(strings.ReplaceAll(sub[1], "_", " ")) + `}}`
	})

	// <%== raw json friends %>
	s = strings.Replace(s, `<%== Mojo::Util::decode("UTF-8", Mojo::JSON::encode_json($friends_music || [])) =~ s/'/\\'/gr %>`,
		`{{.FriendsJSON}}`, 1)

	// session
	s = strings.ReplaceAll(s, `<%= session('user') || '' %>`, ``)

	// Complex style attrs with regex - simplify
	s = strings.ReplaceAll(s, `'<%= $ui_font_family =~ s/ /-/gr %>'`, `'{{.UIFontFamilySafe}}'`)

	// Remove remaining perl lines
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "%") && !strings.HasPrefix(trim, "{{") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var _ = bytes.Buffer{}
