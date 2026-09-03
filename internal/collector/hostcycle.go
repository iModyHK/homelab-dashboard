package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/disks"
	"github.com/iModyHK/homelab-dashboard/internal/host"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

func (c *Collector) hostLoop(ctx context.Context) {
	c.ticker(ctx, c.cfg.HostInterval, c.collectHost)
}

func (c *Collector) collectHost(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.HostInterval)
	defer cancel()
	now := time.Now()

	hs := HostState{TS: now.Unix()}
	var errs []error

	if info, err := c.docker.Info(ctx); err == nil {
		hs.Hostname = info.Name
		hs.OS = info.OperatingSystem
		hs.Kernel = info.KernelVersion
		hs.Docker = info.ServerVersion
		hs.CPUs = info.NCPU
	} else {
		c.state.mu.RLock()
		prev := c.state.host
		c.state.mu.RUnlock()
		hs.Hostname, hs.OS, hs.Kernel, hs.Docker, hs.CPUs = prev.Hostname, prev.OS, prev.Kernel, prev.Docker, prev.CPUs
	}

	cpu, cpuErr := c.host.CPU()
	net, netErr := c.host.Network()
	c.mu.Lock()
	prev := c.hostPrev
	c.hostPrev = hostSnapshot{ts: now, cpu: cpu, net: net, ok: cpuErr == nil && netErr == nil}
	c.mu.Unlock()
	if cpuErr != nil {
		errs = append(errs, fmt.Errorf("cpu: %w", cpuErr))
	} else if prev.ok {
		if pct, ok := host.CPUPercent(prev.cpu, cpu); ok {
			hs.CPUPercent = pct
		}
	}
	if netErr != nil {
		errs = append(errs, fmt.Errorf("net: %w", netErr))
	} else if prev.ok {
		secs := now.Sub(prev.ts).Seconds()
		if secs > 0 {
			if net.RxBytes >= prev.net.RxBytes {
				hs.NetRxRate = uint64(float64(net.RxBytes-prev.net.RxBytes) / secs)
			}
			if net.TxBytes >= prev.net.TxBytes {
				hs.NetTxRate = uint64(float64(net.TxBytes-prev.net.TxBytes) / secs)
			}
		}
	}
	if mem, err := c.host.Memory(); err == nil {
		hs.MemUsed, hs.MemTotal, hs.SwapUsed, hs.SwapTotal = mem.Used(), mem.Total, mem.SwapUsed(), mem.SwapTotal
	} else {
		errs = append(errs, fmt.Errorf("memory: %w", err))
	}
	if load, err := c.host.Load(); err == nil {
		hs.Load1, hs.Load5, hs.Load15 = load.One, load.Five, load.Fifteen
	} else {
		errs = append(errs, fmt.Errorf("load: %w", err))
	}
	if up, err := c.host.Uptime(); err == nil {
		hs.Uptime = int64(up.Seconds())
	}
	if temp, ok := c.host.CPUTemperature(); ok {
		hs.CPUTemp = temp
	}
	probes := make([]host.MountProbe, 0, len(c.cfg.TrackedMounts))
	for _, m := range c.cfg.TrackedMounts {
		probes = append(probes, host.MountProbe{Path: m.Path, Label: m.Label})
	}
	mounts, mountErr := c.host.Mounts(probes)
	if mountErr != nil {
		errs = append(errs, fmt.Errorf("mounts: %w", mountErr))
	}
	hs.Mounts = mounts

	c.state.setSource(sourceHost, errors.Join(errs...))
	if len(errs) > 0 {
		c.log.Warn("host readers", "error", errors.Join(errs...))
	}

	c.state.mu.Lock()
	c.state.host = hs
	c.state.mu.Unlock()

	sample := store.HostSample{
		TS: now, CPUPct: hs.CPUPercent, Load1: hs.Load1, Load5: hs.Load5, Load15: hs.Load15,
		MemUsed: hs.MemUsed, MemTotal: hs.MemTotal, SwapUsed: hs.SwapUsed, SwapTotal: hs.SwapTotal,
		NetRx: hs.NetRxRate, NetTx: hs.NetTxRate, CPUTemp: hs.CPUTemp,
	}
	for _, m := range mounts {
		sample.Mounts = append(sample.Mounts, store.MountSample{Mount: m.Mount, Used: m.Used, Total: m.Total})
	}
	if prev.ok {
		if err := c.store.InsertHostSample(ctx, sample); err != nil {
			c.log.Warn("insert host sample", "error", err)
		}
	}
	c.bus.Publish("host", hs)

	c.collectArrays()
	c.collectSmart(ctx, now)
	c.bus.Publish("disks", map[string]any{"disks": c.Snapshot().Disks, "arrays": c.Snapshot().Arrays})
	c.evaluate(ctx)
}

func (c *Collector) collectArrays() {
	raw, err := os.ReadFile(filepath.Join(c.cfg.HostProc, "mdstat"))
	if err != nil {
		c.log.Debug("mdstat", "error", err)
		return
	}
	arrays := disks.ParseMdstat(string(raw))
	c.state.mu.Lock()
	c.state.arrays = arrays
	c.state.mu.Unlock()
}

type smartPayload struct {
	Devices []json.RawMessage `json:"devices"`
	Updated int64             `json:"updated"`
}

func (c *Collector) collectSmart(ctx context.Context, now time.Time) {
	if c.cfg.SmartURL == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.SmartURL+"/disks", nil)
	if err != nil {
		return
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.state.setSource(sourceSmart, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.state.setSource(sourceSmart, fmt.Errorf("smart sidecar returned %d", resp.StatusCode))
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		c.state.setSource(sourceSmart, err)
		return
	}
	var payload smartPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.state.setSource(sourceSmart, fmt.Errorf("smart payload: %w", err))
		return
	}
	c.state.setSource(sourceSmart, nil)

	var parsed []disks.Disk
	var records []store.DiskRecord
	for _, raw := range payload.Devices {
		d, err := disks.ParseSmartctl(raw)
		if err != nil && !errors.Is(err, disks.ErrStandby) {
			c.log.Debug("smartctl parse", "error", err)
			continue
		}
		parsed = append(parsed, d)
		status := "unknown"
		if d.SmartKnown {
			status = "failed"
			if d.SmartPassed {
				status = "passed"
			}
		}
		records = append(records, store.DiskRecord{
			Device: d.Device, Model: d.Model, Serial: d.Serial, Firmware: d.Firmware, Capacity: int64(d.CapacityBytes),
			Rotation: d.RotationRPM, Transport: d.Transport, Temp: d.Temperature, PowerOnHours: d.PowerOnHours,
			PowerCycles: d.PowerCycles, Reallocated: d.Reallocated, Pending: d.Pending, Uncorrectable: d.Uncorrectable,
			CRCErrors: d.CRCErrors, SmartStatus: status, Standby: d.Standby, PercentUsed: d.PercentUsed,
		})
	}
	if err := c.store.UpsertDisks(ctx, now, records); err != nil {
		c.log.Warn("upsert disks", "error", err)
	}
	merged := c.mergeStandby(ctx, parsed)
	sort.Slice(merged, func(i, j int) bool { return merged[i].Device < merged[j].Device })
	c.state.mu.Lock()
	c.state.disks = merged
	c.state.mu.Unlock()
}

func (c *Collector) mergeStandby(ctx context.Context, parsed []disks.Disk) []disks.Disk {
	needsMerge := false
	for _, d := range parsed {
		if d.Standby {
			needsMerge = true
			break
		}
	}
	if !needsMerge {
		return parsed
	}
	stored, err := c.store.Disks(ctx)
	if err != nil {
		return parsed
	}
	byDevice := map[string]store.DiskRecord{}
	for _, r := range stored {
		byDevice[r.Device] = r
	}
	for i, d := range parsed {
		if !d.Standby {
			continue
		}
		r, ok := byDevice[d.Device]
		if !ok {
			continue
		}
		parsed[i] = disks.Disk{
			Device: d.Device, Model: r.Model, Serial: r.Serial, Firmware: r.Firmware, CapacityBytes: uint64(r.Capacity),
			RotationRPM: r.Rotation, Transport: r.Transport, Temperature: r.Temp, PowerOnHours: r.PowerOnHours,
			PowerCycles: r.PowerCycles, Reallocated: r.Reallocated, Pending: r.Pending, Uncorrectable: r.Uncorrectable,
			CRCErrors: r.CRCErrors, SmartPassed: r.SmartStatus == "passed", SmartKnown: r.SmartStatus != "unknown",
			Standby: true, PercentUsed: r.PercentUsed,
		}
	}
	return parsed
}

func (c *Collector) rollupLoop(ctx context.Context) {
	c.runRollup(ctx)
	c.ticker(ctx, time.Hour, c.runRollup)
}

func (c *Collector) runRollup(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	res, err := c.store.Rollup(ctx, time.Now(), time.Duration(c.cfg.RetentionDays)*24*time.Hour)
	if err != nil {
		c.log.Error("rollup", "error", err)
		return
	}
	if err := c.store.Checkpoint(ctx); err != nil {
		c.log.Warn("checkpoint", "error", err)
	}
	c.log.Info("rollup complete", "container_rows", res.ContainerRows, "host_rows", res.HostRows,
		"pruned_raw", res.PrunedRaw, "pruned_rollups", res.PrunedRollups, "db_bytes", c.store.SizeBytes())
}
