package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type scanResult struct {
	Devices []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"devices"`
}

type cache struct {
	mu      sync.RWMutex
	devices []json.RawMessage
	updated time.Time
}

func main() {
	interval := 5 * time.Minute
	if raw := os.Getenv("SMART_INTERVAL"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 30*time.Second {
			interval = d
		}
	}
	listen := os.Getenv("SMART_LISTEN")
	if listen == "" {
		listen = ":9633"
	}
	var explicit []string
	for _, d := range strings.Split(os.Getenv("SMART_DEVICES"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			explicit = append(explicit, d)
		}
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c := &cache{}
	go func() {
		c.refresh(ctx, logger, explicit)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				c.refresh(ctx, logger, explicit)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		fresh := time.Since(c.updated) < 3*interval
		c.mu.RUnlock()
		if !fresh {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /disks", func(w http.ResponseWriter, r *http.Request) {
		c.mu.RLock()
		defer c.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"devices": c.devices, "updated": c.updated.Unix()})
	})

	srv := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	logger.Info("smart sidecar listening", "addr", listen, "interval", interval.String())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (c *cache) refresh(ctx context.Context, logger *slog.Logger, explicit []string) {
	devices := explicit
	types := map[string]string{}
	if len(devices) == 0 {
		scanned, scannedTypes, err := scan(ctx)
		if err != nil {
			logger.Warn("smartctl scan", "error", err)
			return
		}
		devices, types = scanned, scannedTypes
	}
	sort.Strings(devices)
	var out []json.RawMessage
	for _, dev := range devices {
		raw, err := query(ctx, dev, types[dev])
		if err != nil {
			logger.Warn("smartctl", "device", dev, "error", err)
			continue
		}
		out = append(out, raw)
	}
	c.mu.Lock()
	c.devices = out
	c.updated = time.Now()
	c.mu.Unlock()
	logger.Info("smart refresh", "devices", len(out))
}

func scan(ctx context.Context) ([]string, map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, _ := exec.CommandContext(ctx, "smartctl", "--scan-open", "-j").Output()
	var res scanResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, nil, fmt.Errorf("scan output: %w", err)
	}
	var names []string
	types := map[string]string{}
	seen := map[string]bool{}
	for _, d := range res.Devices {
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		names = append(names, d.Name)
		types[d.Name] = d.Type
	}
	return names, types, nil
}

func query(ctx context.Context, device, devType string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	args := []string{"-a", "-j", "-n", "standby"}
	if devType != "" {
		args = append(args, "-d", devType)
	}
	args = append(args, device)
	raw, _ := exec.CommandContext(ctx, "smartctl", args...).Output()
	if !json.Valid(raw) {
		return nil, errors.New("smartctl produced no json")
	}
	return json.RawMessage(raw), nil
}
