package library

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Elive/synthwave-player/internal/storage/s3"
	"github.com/fsnotify/fsnotify"
)

func (s *Service) StartWatchers() {
	st := s.Config.Get()
	go watchConfig(st.UserConfigFile, s.Logger, func() {
		_ = s.Config.Load(s.Logger)
		if s.Hub != nil {
			s.Hub.BroadcastConfig()
		}
	})
	for _, dir := range st.MusicDirectories {
		if s3.IsURI(dir) {
			continue
		}
		d := dir
		go watchMusicDir(d, s.Logger, func() {
			time.AfterFunc(3*time.Second, func() {
				p, _ := filepath.Abs(d)
				_, _ = s.DB.Exec(`INSERT OR REPLACE INTO scan_state (key, value) VALUES (?, 'pending')`, "task:"+p)
				go s.runTaskProcessor()
			})
		})
	}
}

func watchConfig(path string, logger *slog.Logger, onChange func()) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	_ = w.Add(dir)
	var debounce *time.Timer
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if filepath.Base(ev.Name) == filepath.Base(path) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(2*time.Second, onChange)
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

func watchMusicDir(dir string, logger *slog.Logger, onChange func()) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	_ = w.Add(dir)
	var debounce *time.Timer
	for {
		select {
		case _, ok := <-w.Events:
			if !ok {
				return
			}
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(3*time.Second, onChange)
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}
