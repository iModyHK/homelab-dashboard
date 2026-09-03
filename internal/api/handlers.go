package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/iModyHK/homelab-dashboard/internal/auth"
	"github.com/iModyHK/homelab-dashboard/internal/collector"
	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

const maxSeriesPoints = 400

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := auth.ClientIP(r)
	if !s.auth.Allow(r.RemoteAddr) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, wait a minute")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.auth.CheckPassword(body.Password) {
		s.log.Warn("login failed", "ip", ip)
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	if err := s.auth.Issue(w); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	s.log.Info("login", "ip", ip)
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.Clear(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": s.auth.Authenticated(r)})
}

type containerSummary struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Image         string                  `json:"image"`
	Stack         string                  `json:"stack"`
	StackSource   string                  `json:"stackSource"`
	Service       string                  `json:"service"`
	State         string                  `json:"state"`
	Status        string                  `json:"status"`
	Health        string                  `json:"health"`
	RestartCount  int                     `json:"restartCount"`
	RestartPolicy string                  `json:"restartPolicy"`
	Created       int64                   `json:"created"`
	StartedAt     int64                   `json:"startedAt"`
	FinishedAt    int64                   `json:"finishedAt"`
	ExitCode      int                     `json:"exitCode"`
	MemoryLimit   int64                   `json:"memoryLimit"`
	CPULimit      float64                 `json:"cpuLimit"`
	UpdateReady   bool                    `json:"updateAvailable"`
	Live          *collector.Live         `json:"live"`
	Sparkline     []float64               `json:"sparkline"`
	Ports         []collector.PortMapping `json:"ports"`
}

func summarize(c *collector.Container) containerSummary {
	spark := c.Sparkline
	if len(spark) > 30 {
		spark = spark[len(spark)-30:]
	}
	return containerSummary{
		ID: c.ID, Name: c.Name, Image: c.Image, Stack: c.Stack, StackSource: c.StackSource, Service: c.Service,
		State: c.State, Status: c.Status, Health: c.Health, RestartCount: c.RestartCount, RestartPolicy: c.RestartPolicy,
		Created: c.Created, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt, ExitCode: c.ExitCode,
		MemoryLimit: c.MemoryLimit, CPULimit: c.CPULimit, UpdateReady: c.UpdateReady, Live: c.Live, Sparkline: spark,
		Ports: c.Ports,
	}
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	snap := s.collector.Snapshot()
	containers := make([]containerSummary, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		containers = append(containers, summarize(c))
	}
	alerts, err := s.store.FiringAlerts(r.Context())
	if err != nil {
		s.log.Warn("firing alerts", "error", err)
	}
	events, err := s.store.Events(r.Context(), time.Now().Add(-24*time.Hour), "", 30)
	if err != nil {
		s.log.Warn("events", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"host":       snap.Host,
		"portainer":  snap.Portainer,
		"stacks":     snap.Stacks,
		"containers": containers,
		"disks":      orEmpty(snap.Disks),
		"arrays":     orEmpty(snap.Arrays),
		"alerts":     orEmpty(alerts),
		"events":     orEmpty(events),
		"sources":    snap.Sources,
		"dbBytes":    snap.DBBytes,
		"startedAt":  snap.StartedAt,
		"version":    snap.Version,
		"timezone":   s.cfg.Timezone.String(),
		"intervals": map[string]int{
			"stats": int(s.cfg.StatsInterval.Seconds()),
			"host":  int(s.cfg.HostInterval.Seconds()),
		},
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	snap := s.collector.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"sources":   snap.Sources,
		"dbBytes":   snap.DBBytes,
		"startedAt": snap.StartedAt,
		"version":   snap.Version,
		"clients":   s.hub.clients(),
	})
}

func (s *Server) handleStacks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.collector.Snapshot().Stacks)
}

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	stack := r.URL.Query().Get("stack")
	state := r.URL.Query().Get("state")
	snap := s.collector.Snapshot()
	out := make([]containerSummary, 0, len(snap.Containers))
	for _, c := range snap.Containers {
		if stack != "" && c.Stack != stack {
			continue
		}
		if state != "" && c.State != state {
			continue
		}
		out = append(out, summarize(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, ok := s.collector.Container(id)
	if !ok {
		rec, err := s.store.Container(r.Context(), id)
		if err != nil || rec.ID == "" {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"container": rec, "gone": true})
		return
	}
	restarts, err := s.store.Events(r.Context(), time.Now().Add(-7*24*time.Hour), c.ID, 50)
	if err != nil {
		s.log.Warn("container events", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"container": c, "events": orEmpty(restarts)})
}

func (s *Server) handleContainerSeries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	span, ok := parseRange(r.URL.Query().Get("range"))
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be one of 1h, 6h, 24h, 7d, 30d")
		return
	}
	if c, found := s.collector.Container(id); found {
		id = c.ID
	}
	now := time.Now()
	points, err := s.store.ContainerSeries(r.Context(), id, now.Add(-span), now, maxSeriesPoints)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "series query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": span.String(), "points": orEmpty(points)})
}

var levelPatterns = map[string]*regexp.Regexp{
	"error": regexp.MustCompile(`(?i)\b(error|err|fatal|panic|critical|crit|exception|traceback|emerg)\b|\[E\]`),
	"warn":  regexp.MustCompile(`(?i)\b(warn|warning|wrn)\b|\[W\]`),
	"info":  regexp.MustCompile(`(?i)\b(info|inf)\b|\[I\]`),
	"debug": regexp.MustCompile(`(?i)\b(debug|dbg|trace|trc)\b|\[D\]`),
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiRe.ReplaceAllString(s, "")
}

func classify(line string) string {
	for _, lvl := range []string{"error", "warn", "debug", "info"} {
		if levelPatterns[lvl].MatchString(line) {
			return lvl
		}
	}
	return "info"
}

type logLine struct {
	TS     int64  `json:"ts"`
	Stream string `json:"stream"`
	Level  string `json:"level"`
	Text   string `json:"text"`
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query()
	tail, _ := strconv.Atoi(q.Get("tail"))
	if tail <= 0 {
		tail = 500
	}
	if tail > 5000 {
		tail = 5000
	}
	opts := docker.LogOptions{Tail: tail}
	if since := q.Get("since"); since != "" {
		if secs, err := strconv.ParseInt(since, 10, 64); err == nil && secs > 0 {
			opts.Since = time.Unix(secs, 0)
		}
	}
	level := q.Get("level")
	var search *regexp.Regexp
	if pattern := q.Get("q"); pattern != "" {
		if len(pattern) > 200 {
			writeError(w, http.StatusBadRequest, "search pattern too long")
			return
		}
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid regular expression")
			return
		}
		search = re
	}
	lines, err := s.collector.Logs(r.Context(), id, opts)
	if err != nil {
		if docker.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		writeError(w, http.StatusBadGateway, "log fetch failed")
		return
	}
	out := make([]logLine, 0, len(lines))
	for _, l := range lines {
		text := stripANSI(l.Text)
		lvl := classify(text)
		if !levelAllowed(level, lvl) {
			continue
		}
		if search != nil && !search.MatchString(text) {
			continue
		}
		out = append(out, logLine{TS: l.Time.Unix(), Stream: l.Stream, Level: lvl, Text: text})
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": out, "count": len(out)})
}

func levelAllowed(filter, level string) bool {
	rank := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	switch filter {
	case "", "all":
		return true
	case "debug", "info", "warn", "error":
		return rank[level] >= rank[filter]
	default:
		return true
	}
}

func (s *Server) handleContainerEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if c, ok := s.collector.Container(id); ok {
		id = c.ID
	}
	events, err := s.store.Events(r.Context(), time.Now().Add(-30*24*time.Hour), id, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "events query failed")
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(events))
}

func (s *Server) handleLogSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pattern := q.Get("q")
	if pattern == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	if len(pattern) > 200 {
		writeError(w, http.StatusBadRequest, "search pattern too long")
		return
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid regular expression")
		return
	}
	since := time.Now().Add(-time.Hour)
	if raw := q.Get("since"); raw != "" {
		if secs, err := strconv.ParseInt(raw, 10, 64); err == nil && secs > 0 {
			since = time.Unix(secs, 0)
		}
	}
	filter := map[string]bool{}
	for _, part := range strings.Split(q.Get("containers"), ",") {
		if part = strings.TrimSpace(part); part != "" {
			filter[part] = true
		}
	}
	snap := s.collector.Snapshot()
	type hit struct {
		ContainerID   string `json:"containerId"`
		ContainerName string `json:"containerName"`
		TS            int64  `json:"ts"`
		Stream        string `json:"stream"`
		Level         string `json:"level"`
		Text          string `json:"text"`
	}
	var hits []hit
	const perContainer = 200
	for _, c := range snap.Containers {
		if !c.Running() {
			continue
		}
		if len(filter) > 0 && !filter[c.ID] && !filter[c.Name] && !filter[c.ID[:min(12, len(c.ID))]] {
			continue
		}
		if r.Context().Err() != nil {
			break
		}
		lines, err := s.collector.Logs(r.Context(), c.ID, docker.LogOptions{Tail: 2000, Since: since})
		if err != nil {
			continue
		}
		count := 0
		for _, l := range lines {
			text := stripANSI(l.Text)
			if !re.MatchString(text) {
				continue
			}
			hits = append(hits, hit{ContainerID: c.ID, ContainerName: c.Name, TS: l.Time.Unix(), Stream: l.Stream, Level: classify(text), Text: text})
			count++
			if count >= perContainer {
				break
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].TS > hits[j].TS })
	if len(hits) > 1000 {
		hits = hits[:1000]
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": orEmpty(hits), "count": len(hits)})
}

func (s *Server) handleHostSeries(w http.ResponseWriter, r *http.Request) {
	span, ok := parseRange(r.URL.Query().Get("range"))
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be one of 1h, 6h, 24h, 7d, 30d")
		return
	}
	now := time.Now()
	points, err := s.store.HostSeries(r.Context(), now.Add(-span), now, maxSeriesPoints)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "series query failed")
		return
	}
	mounts, err := s.store.MountSeries(r.Context(), now.Add(-span), now, maxSeriesPoints)
	if err != nil {
		s.log.Warn("mount series", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": span.String(), "points": orEmpty(points), "mounts": orEmpty(mounts)})
}

func (s *Server) handleDisks(w http.ResponseWriter, r *http.Request) {
	snap := s.collector.Snapshot()
	span, _ := parseRange(r.URL.Query().Get("range"))
	if span == 0 {
		span = 24 * time.Hour
	}
	temps, err := s.store.DiskTemps(r.Context(), time.Now().Add(-span), 200)
	if err != nil {
		s.log.Warn("disk temps", "error", err)
	}
	if temps == nil {
		temps = map[string][]store.TempPoint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disks":  orEmpty(snap.Disks),
		"arrays": orEmpty(snap.Arrays),
		"mounts": orEmpty(snap.Host.Mounts),
		"temps":  temps,
		"smart":  snap.Sources["smart"],
	})
}

func (s *Server) handleUpdates(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.ImageUpdates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "updates query failed")
		return
	}
	snap := s.collector.Snapshot()
	usedBy := map[string][]string{}
	for _, c := range snap.Containers {
		usedBy[c.Image] = append(usedBy[c.Image], c.Name)
	}
	type row struct {
		store.ImageUpdateRecord
		Containers []string `json:"containers"`
	}
	out := make([]row, 0, len(records))
	for _, rec := range records {
		out = append(out, row{ImageUpdateRecord: rec, Containers: orEmpty(usedBy[rec.Image])})
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": out, "registry": snap.Sources["registry"]})
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	s.collector.TriggerUpdateCheck()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	alerts, err := s.store.Alerts(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "alerts query failed")
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(alerts))
}

func (s *Server) handleAlertAck(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid alert id")
		return
	}
	ok, err := s.store.AckAlert(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ack failed")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "alert not found or already acknowledged")
		return
	}
	s.collector.Bus().Publish("alert_ack", map[string]int64{"id": id, "ackedAt": time.Now().Unix()})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := time.Now().Add(-24 * time.Hour)
	if raw := q.Get("since"); raw != "" {
		if secs, err := strconv.ParseInt(raw, 10, 64); err == nil && secs > 0 {
			since = time.Unix(secs, 0)
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	events, err := s.store.Events(r.Context(), since, q.Get("container"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "events query failed")
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(events))
}

func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since := time.Now().Add(-24 * time.Hour)
	if raw := q.Get("since"); raw != "" {
		if secs, err := strconv.ParseInt(raw, 10, 64); err == nil && secs > 0 {
			since = time.Unix(secs, 0)
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	records, err := s.store.LogErrors(r.Context(), since, q.Get("container"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error feed query failed")
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(records))
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
