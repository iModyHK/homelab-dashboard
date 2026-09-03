package collector

import (
	"context"
	"time"
)

func (c *Collector) updateLoop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(90 * time.Second):
	case <-c.updateNow:
	}
	c.runUpdateCheck(ctx)
	t := time.NewTicker(c.cfg.UpdateInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.runUpdateCheck(ctx)
		case <-c.updateNow:
			c.runUpdateCheck(ctx)
		}
	}
}

func (c *Collector) runUpdateCheck(ctx context.Context) {
	if c.updateFunc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	c.updateFunc(ctx)
	c.loadUpdates(ctx)
	c.publishContainers()
}

func (c *Collector) loadUpdates(ctx context.Context) {
	records, err := c.store.ImageUpdates(ctx)
	if err != nil {
		c.log.Warn("load image updates", "error", err)
		return
	}
	updates := map[string]bool{}
	for _, r := range records {
		if r.UpdateAvailable {
			updates[r.Image] = true
		}
	}
	c.state.mu.Lock()
	c.state.updates = updates
	c.state.mu.Unlock()
}

func (c *Collector) ImagesInUse() map[string]string {
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()
	out := map[string]string{}
	for _, cont := range c.state.containers {
		out[cont.Image] = cont.ImageID
	}
	return out
}

func (c *Collector) SetRegistryStatus(err error) {
	c.state.setSource(sourceRegistry, err)
}
