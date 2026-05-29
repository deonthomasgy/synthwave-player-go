package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Elive/synthwave-player/internal/config"
	"golang.org/x/mod/semver"
)

type UpdateInfo struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	Changelog       string `json:"changelog,omitempty"`
	Error           string `json:"error,omitempty"`
	URL             string `json:"url,omitempty"`
	PatreonURL      string `json:"patreon_url,omitempty"`
}

func CheckUpdate() UpdateInfo {
	info := UpdateInfo{CurrentVersion: config.AppVersion}
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/Elive/synthwave-player/releases/latest", nil)
	req.Header.Set("User-Agent", "synthwave-player")
	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()
	var rel struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		info.Error = err.Error()
		return info
	}
	latest := rel.TagName
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}
	cur := config.AppVersion
	if !strings.HasPrefix(cur, "v") {
		cur = "v" + cur
	}
	info.LatestVersion = latest
	if semver.Compare(latest, cur) > 0 {
		info.UpdateAvailable = true
		info.Changelog = rel.Body
	}
	return info
}

func FormatUpdateLog(info UpdateInfo) string {
	if info.Error != "" {
		return fmt.Sprintf("update check failed: %s", info.Error)
	}
	if info.UpdateAvailable {
		return fmt.Sprintf("update available: %s -> %s", info.CurrentVersion, info.LatestVersion)
	}
	return "up to date"
}
