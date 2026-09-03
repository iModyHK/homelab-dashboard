package collector

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

func (c *Collector) logScanLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
	}
	c.scanLogs(ctx)
	c.ticker(ctx, logScanInterval, c.scanLogs)
}

func (c *Collector) scanLogs(ctx context.Context) {
	if len(c.patterns) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, logScanInterval)
	defer cancel()

	type target struct {
		id, name string
		tty      bool
	}
	c.state.mu.RLock()
	targets := make([]target, 0, len(c.state.containers))
	for _, cont := range c.state.containers {
		if cont.Running() {
			targets = append(targets, target{id: cont.ID, name: cont.Name, tty: cont.Tty})
		}
	}
	c.state.mu.RUnlock()

	now := time.Now()
	var batch []store.LogErrorRecord
	for _, t := range targets {
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		since, ok := c.logCursor[t.id]
		c.mu.Unlock()
		if !ok {
			since = now.Add(-logScanInterval)
		}
		callCtx, callCancel := context.WithTimeout(ctx, c.cfg.StatsTimeout)
		lines, err := c.docker.Logs(callCtx, t.id, t.tty, docker.LogOptions{Tail: logScanTail, Since: since})
		callCancel()
		if err != nil {
			continue
		}
		c.mu.Lock()
		c.logCursor[t.id] = now
		c.mu.Unlock()
		for _, line := range lines {
			if line.Time.IsZero() || !line.Time.After(since) {
				continue
			}
			text := stripANSI(line.Text)
			if !c.matchesError(text) {
				continue
			}
			batch = append(batch, store.LogErrorRecord{
				TS: line.Time.Unix(), ContainerID: t.id, ContainerName: t.name, Kind: "log", Stream: line.Stream,
				Line: truncate(text, 4000),
			})
		}
	}
	if len(batch) == 0 {
		return
	}
	inserted, err := c.store.InsertLogErrors(ctx, batch)
	if err != nil {
		c.log.Warn("store log errors", "error", err)
		return
	}
	if inserted > 0 {
		c.bus.Publish("errors", map[string]any{"count": inserted, "latest": batch[len(batch)-1]})
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiRe.ReplaceAllString(s, "")
}

func (c *Collector) matchesError(text string) bool {
	for _, re := range c.patterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func (c *Collector) Logs(ctx context.Context, id string, opts docker.LogOptions) ([]docker.LogLine, error) {
	cont, ok := c.state.container(id)
	tty := false
	realID := id
	if ok {
		tty = cont.Tty
		realID = cont.ID
	}
	return c.docker.Logs(ctx, realID, tty, opts)
}
