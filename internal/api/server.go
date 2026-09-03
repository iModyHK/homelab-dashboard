package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/iModyHK/homelab-dashboard/internal/auth"
	"github.com/iModyHK/homelab-dashboard/internal/collector"
	"github.com/iModyHK/homelab-dashboard/internal/config"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

type Notifier interface {
	SendTest(ctx context.Context) error
}

type Server struct {
	cfg       *config.Config
	collector *collector.Collector
	store     *store.Store
	auth      *auth.Manager
	notifier  Notifier
	log       *slog.Logger
	static    fs.FS
	hub       *hub
}

func New(cfg *config.Config, coll *collector.Collector, st *store.Store, am *auth.Manager, notifier Notifier, static fs.FS, logger *slog.Logger) *Server {
	h := newHub(coll.Bus(), logger)
	h.setOrigins(cfg.AllowedOrigins)
	return &Server{
		cfg:       cfg,
		collector: coll,
		store:     st,
		auth:      am,
		notifier:  notifier,
		log:       logger,
		static:    static,
		hub:       h,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(middleware.Compress(5, "application/json", "text/html", "text/css", "application/javascript", "image/svg+xml"))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.NoCache)
		r.Post("/auth/login", s.handleLogin)
		r.Get("/auth/session", s.handleSession)

		r.Group(func(r chi.Router) {
			r.Use(s.auth.Require)
			r.Use(s.auth.CSRF)
			r.Post("/auth/logout", s.handleLogout)

			r.Get("/overview", s.handleOverview)
			r.Get("/status", s.handleStatus)
			r.Get("/stacks", s.handleStacks)
			r.Get("/containers", s.handleContainers)
			r.Get("/containers/{id}", s.handleContainer)
			r.Get("/containers/{id}/series", s.handleContainerSeries)
			r.Get("/containers/{id}/logs", s.handleContainerLogs)
			r.Get("/containers/{id}/events", s.handleContainerEvents)
			r.Get("/logs/search", s.handleLogSearch)
			r.Get("/host/series", s.handleHostSeries)
			r.Get("/disks", s.handleDisks)
			r.Get("/updates", s.handleUpdates)
			r.Post("/updates/check", s.handleUpdateCheck)
			r.Get("/alerts", s.handleAlerts)
			r.Post("/alerts/{id}/ack", s.handleAlertAck)
			r.Post("/alerts/test", s.handleAlertTest)
			r.Get("/events", s.handleEvents)
			r.Get("/errors", s.handleErrors)
			r.Get("/live", s.hub.serve)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})
	})

	r.NotFound(s.serveStatic)
	return r
}

func (s *Server) RunHub(stop <-chan struct{}) {
	s.hub.run(stop)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	name := strings.TrimPrefix(clean, "/")
	if name == "" {
		name = "index.html"
	}
	if f, err := s.static.Open(name); err == nil {
		st, statErr := f.Stat()
		f.Close()
		if statErr == nil && !st.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFileFS(w, r, s.static, name)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.ServeFileFS(w, r, s.static, "index.html")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func parseRange(raw string) (time.Duration, bool) {
	switch raw {
	case "", "1h":
		return time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	default:
		return 0, false
	}
}
