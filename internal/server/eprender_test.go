package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderEPNoRawTags(t *testing.T) {
	root := filepath.Join("..", "..", "tree", "usr", "share", "synthwave-player")
	if _, err := os.Stat(root); err != nil {
		t.Skip("tree install path not found")
	}
	t.Setenv("SYNTHWAVE_BASE", root)

	data := PageData{
		WebsiteTitle:               "Test Player",
		AppVersion:                 "test",
		UIFontSizeDesktop:          "16px",
		UIFontSizeMobile:           "14px",
		UIFontFamily:               "sans-serif",
		LyricsFontSizeDesktop:      "18px",
		LyricsFontSizeMobile:       "16px",
		LyricsFontFamily:           "sans-serif",
		WelcomeMessage:             "Hello",
		DefaultSortBy:              "title",
		DefaultSortOrder:           "asc",
		Protocol:                   "http",
		WebsiteDomain:              "localhost",
		InitialSongsToLoadMobile:   50,
		InitialSongsToLoadDesktop:  100,
		SongsToLoadOnScrollMobile:  25,
		SongsToLoadOnScrollDesktop: 50,
		ShowPlaybackSpeedMinMinutes: 10,
		MinSearchLength:            2,
		SongsPerPage:               50,
		PreviousButtonRestartSeconds: 3,
		IgnorePlaybackErrorsMS:     500,
		NotificationVisibleMS:      3000,
		ScrollToSongAlignMobile:    0.5,
		ScrollToSongAlignDesktop:   0.5,
		PeriodicUpdateCheckHours:   24,
		SidebarAutoCollapseDelayMS: 2000,
		EnableOnlineLyricsSearch:   true,
		EnableCoverArt:             true,
		AdminPasswordIsSet:         true,
		IsPrivateNetwork:           true,
		IsAdmin:                    false,
		FriendsJSON:                "[]",
		GoogleFontsURL:             "",
	}

	out, err := RenderEP(data)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if strings.Contains(html, "<%") {
		t.Fatalf("output still contains EP tags:\n%s", snippet(html, "<%"))
	}
	if !strings.Contains(html, `<!DOCTYPE html>`) {
		t.Fatal("missing doctype")
	}
	if !strings.Contains(html, `x-data="musicPlayer()"`) {
		t.Fatal("missing alpine root")
	}
	if strings.Contains(html, `$friends_music`) {
		t.Fatal("unreplaced friends_music expression")
	}
}

func snippet(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	end := i + 120
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}
