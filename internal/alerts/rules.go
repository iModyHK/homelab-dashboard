package alerts

import (
	"fmt"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/collector"
)

const (
	percentHysteresis = 5.0
	tempHysteresis    = 3
)

func (e *Engine) conditions(snap collector.Snapshot, restarts map[string]int) []condition {
	t := e.thresholds
	var out []condition

	for _, c := range snap.Containers {
		if e.containerDown(c) {
			msg := fmt.Sprintf("state %s, exit code %d", c.State, c.ExitCode)
			if c.OOMKilled {
				msg += ", OOM killed"
			}
			out = append(out, condition{
				rule: "container_down", target: c.ID, targetName: c.Name, severity: "critical",
				message: msg, holdFor: t.Debounce,
			})
		}
		if n := restarts[c.ID]; n >= t.RestartLoopCount {
			out = append(out, condition{
				rule: "restart_loop", target: c.ID, targetName: c.Name, severity: "critical",
				message: fmt.Sprintf("%d restarts in %s", n, t.RestartLoopWindow), holdFor: 0,
			})
		}
		if c.Health == "unhealthy" {
			out = append(out, condition{
				rule: "health_failing", target: c.ID, targetName: c.Name, severity: "warning",
				message: fmt.Sprintf("health check failing, streak %d", c.FailingStreak), holdFor: t.Debounce,
			})
		}
		if c.Live != nil && c.Running() {
			cpuLimit := t.CPUPercent
			if e.isFiring("cpu_high", c.ID) {
				cpuLimit -= percentHysteresis
			}
			cpuScale := 1.0
			if c.CPULimit > 0 {
				cpuScale = c.CPULimit
			} else if snap.Host.CPUs > 0 {
				cpuScale = float64(snap.Host.CPUs)
			}
			cpuPct := c.Live.CPUPercent / cpuScale
			if cpuPct >= cpuLimit {
				out = append(out, condition{
					rule: "cpu_high", target: c.ID, targetName: c.Name, severity: "warning",
					message: fmt.Sprintf("CPU at %.0f%% of its allowance", cpuPct), holdFor: t.SustainedFor,
				})
			}
			if c.Live.MemLimit > 0 && c.MemoryLimit > 0 {
				memLimit := t.MemoryPercent
				if e.isFiring("memory_high", c.ID) {
					memLimit -= percentHysteresis
				}
				memPct := float64(c.Live.MemBytes) / float64(c.Live.MemLimit) * 100
				if memPct >= memLimit {
					out = append(out, condition{
						rule: "memory_high", target: c.ID, targetName: c.Name, severity: "warning",
						message: fmt.Sprintf("memory at %.0f%% of its %s limit", memPct, humanBytes(c.Live.MemLimit)), holdFor: t.SustainedFor,
					})
				}
			}
		}
	}

	if snap.Host.MemTotal > 0 {
		limit := t.MemoryPercent
		if e.isFiring("host_memory_high", "host") {
			limit -= percentHysteresis
		}
		pct := float64(snap.Host.MemUsed) / float64(snap.Host.MemTotal) * 100
		if pct >= limit {
			out = append(out, condition{
				rule: "host_memory_high", target: "host", targetName: snap.Host.Hostname, severity: "warning",
				message: fmt.Sprintf("host memory at %.0f%%", pct), holdFor: t.SustainedFor,
			})
		}
	}
	if snap.Host.CPUs > 0 {
		limit := t.CPUPercent
		if e.isFiring("host_cpu_high", "host") {
			limit -= percentHysteresis
		}
		if snap.Host.CPUPercent >= limit {
			out = append(out, condition{
				rule: "host_cpu_high", target: "host", targetName: snap.Host.Hostname, severity: "warning",
				message: fmt.Sprintf("host CPU at %.0f%%", snap.Host.CPUPercent), holdFor: t.SustainedFor,
			})
		}
	}

	for _, m := range snap.Host.Mounts {
		if m.Total == 0 {
			continue
		}
		limit := t.DiskPercent
		if e.isFiring("disk_high", m.Mount) {
			limit -= percentHysteresis
		}
		pct := float64(m.Used) / float64(m.Total) * 100
		if pct >= limit {
			out = append(out, condition{
				rule: "disk_high", target: m.Mount, targetName: m.Mount, severity: "warning",
				message: fmt.Sprintf("%.0f%% used, %s free", pct, humanBytes(m.Free)), holdFor: t.SustainedFor,
			})
		}
	}

	for _, d := range snap.Disks {
		name := d.Device
		if d.Model != "" {
			name = fmt.Sprintf("%s (%s)", d.Device, d.Model)
		}
		if !d.Healthy() {
			out = append(out, condition{
				rule: "smart_failing", target: d.Device, targetName: name, severity: "critical",
				message: fmt.Sprintf("reallocated %d, pending %d, uncorrectable %d, smart passed %v",
					d.Reallocated, d.Pending, d.Uncorrectable, d.SmartPassed), holdFor: 0,
			})
		}
		if d.Standby || d.Temperature == 0 {
			continue
		}
		limit := t.DiskTempCelsius
		if e.isFiring("disk_temp_high", d.Device) {
			limit -= tempHysteresis
		}
		if d.Temperature >= limit {
			out = append(out, condition{
				rule: "disk_temp_high", target: d.Device, targetName: name, severity: "warning",
				message: fmt.Sprintf("%d°C", d.Temperature), holdFor: t.SustainedFor,
			})
		}
	}

	for _, a := range snap.Arrays {
		if a.Healthy() {
			continue
		}
		msg := a.State
		if a.SyncAction != "" {
			msg = fmt.Sprintf("%s %.1f%%", a.SyncAction, a.SyncPercent)
		}
		out = append(out, condition{
			rule: "raid_degraded", target: a.Name, targetName: a.Name + " " + a.Level, severity: "critical",
			message: fmt.Sprintf("%s, %d of %d members active", msg, a.SlotsActive, a.SlotsTotal), holdFor: 0,
		})
	}

	for _, name := range []string{"docker", "portainer"} {
		src, ok := snap.Sources[name]
		if !ok || src.OK {
			continue
		}
		out = append(out, condition{
			rule: "host_unreachable", target: name, targetName: name, severity: "critical",
			message: src.LastError, holdFor: t.HostUnreachableFor,
		})
	}

	if e.dbMax > 0 && snap.DBBytes > e.dbMax {
		out = append(out, condition{
			rule: "db_size", target: "sqlite", targetName: "dashboard.db", severity: "warning",
			message: fmt.Sprintf("%s on disk, limit %s", humanBytes(uint64(snap.DBBytes)), humanBytes(uint64(e.dbMax))), holdFor: 0,
		})
	}

	return out
}

func (e *Engine) containerDown(c *collector.Container) bool {
	if c.Running() || c.State == "paused" || c.State == "created" || c.State == "restarting" {
		return c.State == "restarting" && time.Since(time.Unix(c.FinishedAt, 0)) > e.thresholds.Debounce
	}
	switch c.RestartPolicy {
	case "always", "unless-stopped":
		return true
	case "on-failure":
		return c.ExitCode != 0
	default:
		return c.ExitCode != 0 && !stoppedLongAgo(c)
	}
}

func stoppedLongAgo(c *collector.Container) bool {
	if c.FinishedAt == 0 {
		return false
	}
	return time.Since(time.Unix(c.FinishedAt, 0)) > 24*time.Hour
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
