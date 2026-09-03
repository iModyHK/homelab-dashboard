package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/alerts"
	"github.com/iModyHK/homelab-dashboard/internal/api"
	"github.com/iModyHK/homelab-dashboard/internal/auth"
	"github.com/iModyHK/homelab-dashboard/internal/collector"
	"github.com/iModyHK/homelab-dashboard/internal/config"
	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/host"
	"github.com/iModyHK/homelab-dashboard/internal/portainer"
	"github.com/iModyHK/homelab-dashboard/internal/registry"
	"github.com/iModyHK/homelab-dashboard/internal/store"
	"github.com/iModyHK/homelab-dashboard/web"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		if err := hashPasswordCommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func hashPasswordCommand() error {
	pw := os.Getenv("ADMIN_PASSWORD")
	if pw == "" {
		return errors.New("set ADMIN_PASSWORD in the environment to hash it")
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration:\n%w", err)
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "dashboard.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	authManager, err := auth.NewManager(cfg.AdminPassword, cfg.AdminPasswordHash, cfg.SessionSecret, cfg.DataDir, cfg.SessionTTL, cfg.SecureCookies)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dockerClient := docker.New(cfg.DockerHost)
	pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := dockerClient.Ping(pingCtx); err != nil {
		logger.Warn("docker socket proxy not reachable at startup, will keep retrying", "error", err)
	}
	pingCancel()

	portainerClient := portainer.New(cfg.PortainerURL, func() string {
		fresh, _ := config.ReloadSecret("PORTAINER_API_KEY")
		if fresh != "" {
			return fresh
		}
		return cfg.PortainerAPIKey
	})
	hostReader := host.New(cfg.HostProc, cfg.HostSys, cfg.HostRoot)
	bus := collector.NewBus()
	coll := collector.New(cfg, dockerClient, portainerClient, hostReader, st, bus, logger, version)

	var notifier alerts.Notifier
	if cfg.TelegramBotToken != "" {
		notifier = alerts.NewTelegram(cfg.TelegramBotToken, cfg.TelegramChatID)
		logger.Info("alert delivery enabled", "channel", "telegram")
	} else {
		logger.Info("alert delivery disabled, alerts stay in the dashboard only")
	}
	engine := alerts.NewEngine(cfg, st, bus, notifier, logger)
	if err := engine.Restore(ctx); err != nil {
		logger.Warn("restore alerts", "error", err)
	}
	coll.SetEvaluator(engine)

	checker := registry.NewChecker(cfg.RegistryAuth, logger)
	coll.SetUpdateChecker(func(ctx context.Context) {
		images := make([]string, 0)
		for image := range coll.ImagesInUse() {
			images = append(images, image)
		}
		err := registry.RunCheck(ctx, checker, dockerClient, st, images)
		coll.SetRegistryStatus(err)
		if err != nil {
			logger.Warn("image update check", "error", err)
		} else {
			logger.Info("image update check complete", "images", len(images))
		}
	})

	static, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}
	server := api.New(cfg, coll, st, authManager, static, logger)
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go coll.Run(ctx)
	go engine.Run(ctx)
	go server.RunHub(ctx.Done())

	errCh := make(chan error, 1)
	go func() {
		logger.Info("dashboard listening", "addr", cfg.ListenAddr, "version", version, "timezone", cfg.Timezone.String())
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := st.Checkpoint(shutdownCtx); err != nil {
			logger.Warn("final checkpoint", "error", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
