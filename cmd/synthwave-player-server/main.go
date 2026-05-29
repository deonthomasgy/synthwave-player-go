package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Elive/synthwave-player/internal/config"
	"github.com/Elive/synthwave-player/internal/db"
	"github.com/Elive/synthwave-player/internal/network"
	"github.com/Elive/synthwave-player/internal/paths"
	"github.com/Elive/synthwave-player/internal/server"
)

func main() {
	port := flag.Int("port", 0, "listen port")
	host := flag.String("host", "", "listen host (e.g. 0.0.0.0, 127.0.0.1); overrides config when set")
	foreground := flag.Bool("foreground", true, "run in foreground")
	flag.Parse()
	_ = foreground

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Dev: default SYNTHWAVE_BASE to tree path when install dir missing
	if os.Getenv("SYNTHWAVE_BASE") == "" {
		exe, _ := os.Executable()
		dev := filepath.Join(filepath.Dir(exe), "..", "..", "tree", "usr", "share", "synthwave-player")
		if st, err := os.Stat(dev); err == nil && st.IsDir() {
			_ = os.Setenv("SYNTHWAVE_BASE", dev)
		}
	}

	cfg := config.NewStore()
	if err := cfg.Load(logger); err != nil {
		logger.Error("config load", "err", err)
		os.Exit(1)
	}
	config.CheckDependencies(logger)

	st := cfg.Get()
	listenPort := st.ServerPort
	listenHost := st.ServerHost
	if *port > 0 {
		listenPort = *port
	}
	if *host != "" {
		listenHost = config.NormalizeListenHost(*host)
	}
	listenAddr := config.ListenAddr(listenHost, listenPort)
	if !network.PortAvailable(listenHost, listenPort) {
		fmt.Println("already running")
		os.Exit(0)
	}

	database, err := db.Open(logger)
	if err != nil {
		logger.Error("database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	srv := server.New(logger, cfg, database)
	srv.Library.StartBackground()
	srv.Library.StartWatchers()

	// Session secret from admin password
	st = cfg.Get()
	secret := st.AdminPassword
	if secret == "" {
		secret = "synthwave-player-default-secret"
	}
	_ = secret

	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 30 * time.Second,
	}

	writePID()
	defer removePID()

	go func() {
		logger.Info("Synthwave Player starting", "addr", listenAddr, "base", paths.Base())
		fmt.Printf("%s Player starting up...\n", st.WebsiteTitle)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func pidPath() string {
	uid := os.Getuid()
	runDir := fmt.Sprintf("/run/user/%d", uid)
	if st, err := os.Stat(runDir); err == nil && st.IsDir() {
		return filepath.Join(runDir, "synthwave-player-server.pid")
	}
	return filepath.Join("/tmp", fmt.Sprintf(".synthwave-player-server-%d.pid", uid))
}

func writePID() {
	_ = os.WriteFile(pidPath(), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

func removePID() {
	_ = os.Remove(pidPath())
}
