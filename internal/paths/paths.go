package paths

import (
	"os"
	"path/filepath"
)

// Base returns the install root (public/, templates/).
// Set SYNTHWAVE_BASE for development; default matches packaged path.
func Base() string {
	if b := os.Getenv("SYNTHWAVE_BASE"); b != "" {
		return b
	}
	return "/usr/share/synthwave-player"
}

func PublicDir() string  { return filepath.Join(Base(), "public") }
func TemplatesDir() string { return filepath.Join(Base(), "templates") }
