package config

// VarInfo describes a server-config.json key for the admin settings UI.
type VarInfo struct {
	Type    string  `json:"type"`
	Comment string  `json:"comment"`
	Group   string  `json:"group"`
	Order   float64 `json:"order"`
}

// SettingsSchema returns metadata for keys exposed in /api/config.
func SettingsSchema() map[string]VarInfo {
	return map[string]VarInfo{
		"SERVER_HOST": {
			Type:    "scalar",
			Comment: "The network address for the web server to listen on (e.g. 0.0.0.0 for all interfaces, 127.0.0.1 for local only). Restart required.",
			Group:   "Networking",
			Order:   35.4,
		},
		"SERVER_PORT": {
			Type:    "scalar",
			Comment: "The port number for the web server to listen on. You must restart the server for changes to take effect.",
			Group:   "Networking",
			Order:   35.5,
		},
		"ALLOW_ADMIN_FROM_ANY_NETWORK": {
			Type:    "boolean",
			Comment: "Allow entering Admin Mode from any network (public internet). By default, Admin Mode is only accessible from private/local networks for security.",
			Group:   "Networking",
			Order:   38,
		},
	}
}
