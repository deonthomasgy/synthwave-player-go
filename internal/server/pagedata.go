package server

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/Elive/synthwave-player/internal/config"
)

type OGData struct {
	Title       string
	Description string
	Image       string
	URL         string
}

type PageData struct {
	WebsiteTitle              string
	AppVersion                string
	UIFontSizeDesktop         string
	UIFontSizeMobile          string
	UIFontFamily              string
	UIFontFamilySafe          string
	LyricsFontSizeDesktop     string
	LyricsFontSizeMobile      string
	LyricsFontFamily          string
	LyricsFontFamilySafe      string
	WelcomeMessage            string
	DefaultSortBy             string
	DefaultSortOrder          string
	Protocol                  string
	WebsiteDomain             string
	InitialSongsToLoadMobile  int
	InitialSongsToLoadDesktop int
	SongsToLoadOnScrollMobile int
	SongsToLoadOnScrollDesktop int
	ShowPlaybackSpeedMinMinutes int
	MinSearchLength           int
	SongsPerPage              int
	PreviousButtonRestartSeconds int
	IgnorePlaybackErrorsMS    int
	NotificationVisibleMS     int
	ScrollToSongAlignMobile   float64
	ScrollToSongAlignDesktop  float64
	PeriodicUpdateCheckHours  int
	SidebarAutoCollapseDelayMS int
	SourceHash                string
	UPnPErrorMessage          string
	EnableOnlineLyricsSearch  bool
	EnableCoverArt            bool
	AdminPasswordIsSet        bool
	AllowAdminFromAnyNetwork  bool
	AllowLocalNetworkEditing  bool
	ShowPlaylistsOutOfDateWarning bool
	IsPrivateNetwork          bool
	IsAdmin                   bool
	IsPro                     bool
	IsElive                   bool
	ShowSetInitialPasswordModal bool
	ShowUpnpErrorModal        bool
	OG                        OGData
	GoogleFontsURL            string
	FriendsJSON               string
}

func PageDataFromConfig(st config.Settings, og OGData, scheme, host string, isPrivate, isAdmin bool) PageData {
	friendsJSON, _ := json.Marshal(st.FriendsMusic)
	gfonts := googleFontsURL(st.UIFontFamily, st.LyricsFontFamily)
	uiSafe := fontSafe(st.UIFontFamily)
	lySafe := fontSafe(st.LyricsFontFamily)
	showInit := st.AdminPassword == "" && (isPrivate || st.AllowAdminFromAnyNetwork)
	showUPnP := st.UPnPErrorMessage != "" && isPrivate
	return PageData{
		WebsiteTitle: st.WebsiteTitle, AppVersion: config.AppVersion,
		UIFontSizeDesktop: st.UIFontSizeDesktop, UIFontSizeMobile: st.UIFontSizeMobile,
		UIFontFamily: st.UIFontFamily, UIFontFamilySafe: uiSafe,
		LyricsFontSizeDesktop: st.LyricsFontSizeDesktop, LyricsFontSizeMobile: st.LyricsFontSizeMobile,
		LyricsFontFamily: st.LyricsFontFamily, LyricsFontFamilySafe: lySafe,
		WelcomeMessage: st.WelcomeMessage, DefaultSortBy: st.DefaultSortBy, DefaultSortOrder: st.DefaultSortOrder,
		Protocol: scheme, WebsiteDomain: host,
		InitialSongsToLoadMobile: st.InitialSongsToLoadMobile, InitialSongsToLoadDesktop: st.InitialSongsToLoadDesktop,
		SongsToLoadOnScrollMobile: st.SongsToLoadOnScrollMobile, SongsToLoadOnScrollDesktop: st.SongsToLoadOnScrollDesktop,
		ShowPlaybackSpeedMinMinutes: st.ShowPlaybackSpeedMinMinutes, MinSearchLength: st.MinSearchLength,
		SongsPerPage: st.SongsPerPage, PreviousButtonRestartSeconds: st.PreviousButtonRestartSeconds,
		IgnorePlaybackErrorsMS: st.IgnorePlaybackErrorsMS, NotificationVisibleMS: st.NotificationVisibleMS,
		ScrollToSongAlignMobile: st.ScrollToSongAlignMobile, ScrollToSongAlignDesktop: st.ScrollToSongAlignDesktop,
		PeriodicUpdateCheckHours: st.PeriodicUpdateCheckHours,
		SidebarAutoCollapseDelayMS: st.SidebarAutoCollapseDelayMS,
		SourceHash: "",
		UPnPErrorMessage: st.UPnPErrorMessage,
		EnableOnlineLyricsSearch: st.EnableOnlineLyricsSearch, EnableCoverArt: st.EnableCoverArt,
		AdminPasswordIsSet:        st.AdminPassword != "",
		AllowAdminFromAnyNetwork:  st.AllowAdminFromAnyNetwork,
		AllowLocalNetworkEditing: st.AllowLocalNetworkEditing && st.AdminPassword != "",
		ShowPlaylistsOutOfDateWarning: st.ShowPlaylistsOutOfDateWarning,
		IsPrivateNetwork: isPrivate, IsAdmin: isAdmin, IsPro: false,
		IsElive: fileExists("/etc/elive-version"),
		ShowSetInitialPasswordModal: showInit, ShowUpnpErrorModal: showUPnP,
		OG: og, GoogleFontsURL: gfonts, FriendsJSON: string(friendsJSON),
	}
}

func googleFontsURL(fonts ...string) string {
	var families []string
	skip := map[string]bool{"serif": true, "sans-serif": true, "monospace": true, "cursive": true, "fantasy": true, "system-ui": true, "sans": true}
	for _, f := range fonts {
		if f == "" || skip[strings.ToLower(f)] {
			continue
		}
		families = append(families, "family="+strings.ReplaceAll(f, " ", "+"))
	}
	if len(families) == 0 {
		return ""
	}
	return "https://fonts.googleapis.com/css2?" + strings.Join(families, "&") + "&display=swap"
}

func fontSafe(f string) string {
	return strings.ReplaceAll(f, " ", "-")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
