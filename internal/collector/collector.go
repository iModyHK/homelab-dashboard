package collector

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/config"
	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/host"
	"github.com/iModyHK/homelab-dashboard/internal/portainer"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

const (
	unmanagedStack  = "unmanaged"
	sourcePortainer = "portainer"
	sourceDocker    = "docker"
	sourceHost      = "host"
	sourceSmart     = "smart"
	sourceRegistry  = "registry"
	reinspectEvery  = 5 * time.Minute
	logScanInterval = 60 * time.Second
	logScanTail     = 500
	sparklinePoints = 30
)

type Evaluator interface {
	Evaluate(ctx context.Context, snap Snapshot, restarts map[string]int)
}

type Collector struct {
	cfg       *config.Config
	docker    *docker.Client
	portainer *portainer.Client
	host      *host.Reader
	store     *store.Store
	bus       *Bus
	log       *slog.Logger
	http      *http.Client
	state     *state
	evaluator Evaluator
	version   string
	startedAt time.Time

	mu        sync.Mutex
	cpuPrev   map[string]docker.CPUSnapshot
	counters  map[string]counterSnapshot
	hostPrev  hostSnapshot
	logCursor map[string]time.Time

	patterns   []*regexp.Regexp
	excludes   []string
	updateNow  chan struct{}
	updateFunc func(ctx context.Context)
}

type counterSnapshot struct {
	ts       time.Time
	netRx    uint64
	netTx    uint64
	blkRead  uint64
	blkWrite uint64
}

type rateSet struct {
	netRx, netTx, blkRead, blkWrite uint64
}

type hostSnapshot struct {
	ts  time.Time
	cpu host.CPUTimes
	net host.NetCounters
	ok  bool
}

func New(cfg *config.Config, dockerClient *docker.Client, portainerClient *portainer.Client, hostReader *host.Reader,
	st *store.Store, bus *Bus, logger *slog.Logger, version string) *Collector {
	patterns := make([]*regexp.Regexp, 0, len(cfg.LogErrorPatterns))
	for _, p := range cfg.LogErrorPatterns {
		if re, err := regexp.Compile(p); err == nil {
			patterns = append(patterns, re)
		} else {
			logger.Warn("ignoring invalid log pattern", "pattern", p, "error", err)
		}
	}
	return &Collector{
		cfg:       cfg,
		docker:    dockerClient,
		portainer: portainerClient,
		host:      hostReader,
		store:     st,
		bus:       bus,
		log:       logger,
		http:      &http.Client{Timeout: 20 * time.Second},
		state:     newState(),
		version:   version,
		startedAt: time.Now(),
		cpuPrev:   map[string]docker.CPUSnapshot{},
		counters:  map[string]counterSnapshot{},
		logCursor: map[string]time.Time{},
		patterns:  patterns,
		excludes:  cfg.ExcludePatterns,
		updateNow: make(chan struct{}, 1),
	}
}

func (c *Collector) SetEvaluator(e Evaluator) {
	c.evaluator = e
}

func (c *Collector) SetUpdateChecker(fn func(ctx context.Context)) {
	c.updateFunc = fn
}

func (c *Collector) Bus() *Bus {
	return c.bus
}

func (c *Collector) Snapshot() Snapshot {
	snap := c.state.snapshot()
	snap.DBBytes = c.store.SizeBytes()
	snap.StartedAt = c.startedAt.Unix()
	snap.Version = c.version
	return snap
}

func (c *Collector) Container(id string) (*Container, bool) {
	cont, ok := c.state.container(id)
	if !ok {
		return nil, false
	}
	cp := *cont
	c.state.mu.RLock()
	cp.UpdateReady = c.state.updates[cp.Image]
	c.state.mu.RUnlock()
	return &cp, true
}

func (c *Collector) Run(ctx context.Context) {
	var wg sync.WaitGroup
	run := func(name string, fn func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(ctx)
			c.log.Debug("loop stopped", "loop", name)
		}()
	}
	c.bootstrap(ctx)
	run("portainer", c.portainerLoop)
	run("inventory", c.inventoryLoop)
	run("stats", c.statsLoop)
	run("host", c.hostLoop)
	run("events", c.eventsLoop)
	run("logs", c.logScanLoop)
	run("rollup", c.rollupLoop)
	run("updates", c.updateLoop)
	wg.Wait()
}

func (c *Collector) bootstrap(ctx context.Context) {
	c.refreshPortainer(ctx)
	c.refreshInventory(ctx)
	c.collectHost(ctx)
	c.loadUpdates(ctx)
}

func (c *Collector) excluded(name, stack string) bool {
	if stack != "" && stack == c.cfg.OwnStack {
		return true
	}
	for _, pattern := range c.excludes {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
		if strings.Contains(pattern, "*") {
			continue
		}
		if pattern == name {
			return true
		}
	}
	return false
}

func (c *Collector) ticker(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

func (c *Collector) evaluate(ctx context.Context) {
	if c.evaluator == nil {
		return
	}
	snap := c.Snapshot()
	restarts, err := c.store.RestartCounts(ctx, time.Now().Add(-c.cfg.Alerts.RestartLoopWindow))
	if err != nil {
		c.log.Warn("restart counts", "error", err)
		restarts = map[string]int{}
	}
	c.evaluator.Evaluate(ctx, snap, restarts)
}

func (c *Collector) TriggerUpdateCheck() {
	select {
	case c.updateNow <- struct{}{}:
	default:
	}
}
