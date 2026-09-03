package collector

import (
	"context"
	"sync"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

func (c *Collector) statsLoop(ctx context.Context) {
	c.collectStats(ctx)
	c.ticker(ctx, c.cfg.StatsInterval, c.collectStats)
}

type statsResult struct {
	id    string
	stats docker.Stats
	err   error
}

func (c *Collector) collectStats(ctx context.Context) {
	cycleCtx, cancel := context.WithTimeout(ctx, c.cfg.StatsInterval)
	defer cancel()

	c.state.mu.RLock()
	ids := make([]string, 0, len(c.state.containers))
	for id, cont := range c.state.containers {
		if cont.Running() {
			ids = append(ids, id)
		}
	}
	c.state.mu.RUnlock()
	if len(ids) == 0 {
		c.evaluate(ctx)
		return
	}

	jobs := make(chan string)
	results := make(chan statsResult, len(ids))
	var wg sync.WaitGroup
	workers := min(c.cfg.StatsWorkers, len(ids))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				callCtx, callCancel := context.WithTimeout(cycleCtx, c.cfg.StatsTimeout)
				st, err := c.docker.Stats(callCtx, id)
				callCancel()
				results <- statsResult{id: id, stats: st, err: err}
			}
		}()
	}
	for _, id := range ids {
		select {
		case jobs <- id:
		case <-cycleCtx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	now := time.Now()
	var samples []store.Sample
	live := map[string]*Live{}
	var failures int

	c.mu.Lock()
	for r := range results {
		if r.err != nil {
			failures++
			if docker.IsNotFound(r.err) {
				delete(c.cpuPrev, r.id)
				delete(c.counters, r.id)
			} else {
				c.log.Debug("stats", "id", r.id[:min(12, len(r.id))], "error", r.err)
			}
			continue
		}
		prev, hadPrev := c.cpuPrev[r.id]
		var prevPtr *docker.CPUSnapshot
		if hadPrev {
			prevPtr = &prev
		}
		usage, snap, cpuOK := docker.ExtractUsage(prevPtr, r.stats)
		c.cpuPrev[r.id] = snap

		rates, rateOK := c.rates(r.id, now, usage)
		if !cpuOK || !rateOK {
			continue
		}
		live[r.id] = liveFromUsage(now, usage, rates)
		samples = append(samples, store.Sample{
			ContainerID: r.id, TS: now, CPUPct: usage.CPUPercent, MemBytes: usage.MemBytes, MemLimit: usage.MemLimit,
			NetRx: rates.netRx, NetTx: rates.netTx, BlkRead: rates.blkRead, BlkWrite: rates.blkWrite,
		})
	}
	c.mu.Unlock()

	if failures == len(ids) {
		c.state.setSource(sourceDocker, context.DeadlineExceeded)
	}

	c.state.mu.Lock()
	for id, l := range live {
		if cont, ok := c.state.containers[id]; ok {
			cont.Live = l
			cont.Sparkline = appendSparkline(cont.Sparkline, l.CPUPercent)
		}
	}
	c.state.mu.Unlock()

	if err := c.store.InsertSamples(ctx, samples); err != nil {
		c.log.Warn("insert samples", "error", err)
	}
	if len(live) > 0 {
		c.bus.Publish("stats", map[string]any{"ts": now.Unix(), "containers": live, "stacks": c.Snapshot().Stacks})
	}
	c.evaluate(ctx)
}

func (c *Collector) rates(id string, now time.Time, u docker.Usage) (rateSet, bool) {
	cur := counterSnapshot{ts: now, netRx: u.NetRx, netTx: u.NetTx, blkRead: u.BlkRead, blkWrite: u.BlkWrite}
	prev, ok := c.counters[id]
	c.counters[id] = cur
	if !ok {
		return rateSet{}, false
	}
	secs := now.Sub(prev.ts).Seconds()
	if secs <= 0 {
		return rateSet{}, false
	}
	delta := func(a, b uint64) uint64 {
		if b < a {
			return 0
		}
		return uint64(float64(b-a) / secs)
	}
	return rateSet{
		netRx:    delta(prev.netRx, cur.netRx),
		netTx:    delta(prev.netTx, cur.netTx),
		blkRead:  delta(prev.blkRead, cur.blkRead),
		blkWrite: delta(prev.blkWrite, cur.blkWrite),
	}, true
}

func appendSparkline(existing []float64, v float64) []float64 {
	existing = append(existing, v)
	if len(existing) > sparklinePoints*4 {
		existing = existing[len(existing)-sparklinePoints*4:]
	}
	return existing
}

func (c *Collector) ResetCounters(id string) {
	c.mu.Lock()
	delete(c.cpuPrev, id)
	delete(c.counters, id)
	c.mu.Unlock()
}
