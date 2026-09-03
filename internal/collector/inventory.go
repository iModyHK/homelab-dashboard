package collector

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/portainer"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

func (c *Collector) portainerLoop(ctx context.Context) {
	c.ticker(ctx, c.cfg.HostInterval, c.refreshPortainer)
}

func (c *Collector) refreshPortainer(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	status, err := c.portainer.Status(ctx)
	if err != nil {
		c.state.setSource(sourcePortainer, err)
		c.log.Warn("portainer status", "error", err)
		return
	}
	endpoints, err := c.portainer.Endpoints(ctx)
	if err != nil {
		c.state.setSource(sourcePortainer, err)
		c.log.Warn("portainer endpoints", "error", err)
		return
	}
	stacks, err := c.portainer.Stacks(ctx)
	if err != nil {
		c.state.setSource(sourcePortainer, err)
		c.log.Warn("portainer stacks", "error", err)
		return
	}
	c.state.setSource(sourcePortainer, nil)

	wanted := map[int]bool{}
	for _, id := range c.cfg.PortainerEndpointIDs {
		wanted[id] = true
	}
	var summaries []EndpointSummary
	allowed := map[int]bool{}
	for _, e := range endpoints {
		if len(wanted) > 0 && !wanted[e.ID] {
			continue
		}
		allowed[e.ID] = true
		summaries = append(summaries, EndpointSummary{ID: e.ID, Name: portainer.EndpointLabel(e), Online: e.Online()})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })

	meta := map[string]StackMeta{}
	for _, s := range stacks {
		if len(allowed) > 0 && !allowed[s.EndpointID] {
			continue
		}
		status := "inactive"
		if s.Active() {
			status = "active"
		}
		meta[s.Name] = StackMeta{ID: s.ID, EndpointID: s.EndpointID, Type: s.TypeName(), Status: status}
	}

	c.state.mu.Lock()
	c.state.portainer = PortainerState{Version: status.Version, Endpoints: summaries, Stacks: meta}
	for _, cont := range c.state.containers {
		c.assignStack(cont, cont.Labels)
	}
	c.state.mu.Unlock()
}

func (c *Collector) assignStack(cont *Container, labels map[string]string) {
	project := labels[docker.LabelComposeProject]
	cont.Service = labels[docker.LabelComposeService]
	switch {
	case project != "":
		if meta, ok := c.state.portainer.Stacks[project]; ok {
			cont.Stack = project
			cont.StackSource = "portainer"
			cont.EndpointID = meta.EndpointID
			return
		}
		cont.Stack = project
		cont.StackSource = "compose"
	default:
		cont.Stack = unmanagedStack
		cont.StackSource = "unmanaged"
	}
	if cont.EndpointID == 0 && len(c.state.portainer.Endpoints) == 1 {
		cont.EndpointID = c.state.portainer.Endpoints[0].ID
	}
}

func (c *Collector) inventoryLoop(ctx context.Context) {
	c.ticker(ctx, c.cfg.StatsInterval, c.refreshInventory)
}

func (c *Collector) refreshInventory(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.StatsInterval)
	defer cancel()

	list, err := c.docker.ListContainers(ctx)
	if err != nil {
		c.state.setSource(sourceDocker, err)
		c.log.Warn("list containers", "error", err)
		return
	}
	c.state.setSource(sourceDocker, nil)

	now := time.Now()
	seen := map[string]bool{}
	var toInspect []string
	changed := false

	c.state.mu.Lock()
	for _, summary := range list {
		name := summary.Name()
		project := summary.Labels[docker.LabelComposeProject]
		if c.excluded(name, project) {
			continue
		}
		seen[summary.ID] = true
		cont, exists := c.state.containers[summary.ID]
		if !exists {
			cont = &Container{ID: summary.ID, Created: summary.Created}
			c.state.containers[summary.ID] = cont
			changed = true
		}
		if cont.State != summary.State || cont.Name != name || cont.Image != summary.Image {
			changed = true
		}
		cont.Name = name
		cont.Image = summary.Image
		cont.ImageID = summary.ImageID
		cont.State = summary.State
		cont.Status = summary.Status
		cont.Labels = summary.Labels
		c.assignStack(cont, summary.Labels)
		if !exists || now.Sub(cont.LastInspected) > reinspectEvery {
			toInspect = append(toInspect, summary.ID)
		}
	}
	for id, cont := range c.state.containers {
		if !seen[id] {
			delete(c.state.containers, id)
			changed = true
			c.log.Info("container gone", "name", cont.Name)
		}
	}
	c.state.mu.Unlock()

	c.mu.Lock()
	for id := range c.cpuPrev {
		if !seen[id] {
			delete(c.cpuPrev, id)
			delete(c.counters, id)
			delete(c.logCursor, id)
		}
	}
	c.mu.Unlock()

	for _, id := range toInspect {
		c.inspect(ctx, id)
	}

	c.persistInventory(ctx, now)
	if changed {
		c.publishContainers()
	}
}

func (c *Collector) inspect(ctx context.Context, id string) {
	ictx, cancel := context.WithTimeout(ctx, c.cfg.StatsTimeout)
	defer cancel()
	info, err := c.docker.Inspect(ictx, id)
	if err != nil {
		if !docker.IsNotFound(err) {
			c.log.Warn("inspect", "id", id[:min(12, len(id))], "error", err)
		}
		return
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	cont, ok := c.state.containers[id]
	if !ok {
		return
	}
	applyInspect(cont, info)
	cont.LastInspected = time.Now()
}

func applyInspect(cont *Container, info docker.ContainerInspect) {
	cont.State = info.State.Status
	cont.RestartCount = info.RestartCount
	cont.RestartPolicy = info.HostConfig.RestartPolicy.Name
	cont.ExitCode = info.State.ExitCode
	cont.OOMKilled = info.State.OOMKilled
	cont.Error = info.State.Error
	cont.Tty = info.Config.Tty
	cont.StartedAt = parseDockerTime(info.State.StartedAt)
	cont.FinishedAt = parseDockerTime(info.State.FinishedAt)
	cont.MemoryLimit = info.HostConfig.Memory
	cont.CPULimit = float64(info.HostConfig.NanoCPUs) / 1e9
	if cont.Created == 0 {
		cont.Created = parseDockerTime(info.Created)
	}
	if info.State.Health != nil {
		cont.Health = info.State.Health.Status
		cont.FailingStreak = info.State.Health.FailingStreak
		cont.HealthLog = cont.HealthLog[:0]
		logs := info.State.Health.Log
		if len(logs) > 5 {
			logs = logs[len(logs)-5:]
		}
		for _, l := range logs {
			cont.HealthLog = append(cont.HealthLog, HealthCheck{
				Start:    parseDockerTime(l.Start),
				ExitCode: l.ExitCode,
				Output:   truncate(l.Output, 2000),
			})
		}
	} else {
		cont.Health = ""
		cont.FailingStreak = 0
		cont.HealthLog = nil
	}
	cont.Env = maskEnv(info.Config.Env)
	cont.Mounts = cont.Mounts[:0]
	for _, m := range info.Mounts {
		src := m.Source
		if m.Type == "volume" && m.Name != "" {
			src = m.Name
		}
		cont.Mounts = append(cont.Mounts, MountInfo{Type: m.Type, Source: src, Destination: m.Destination, ReadOnly: !m.RW})
	}
	sort.Slice(cont.Mounts, func(i, j int) bool { return cont.Mounts[i].Destination < cont.Mounts[j].Destination })
	cont.Ports = cont.Ports[:0]
	for spec, bindings := range info.HostConfig.PortBindings {
		portStr, proto, _ := strings.Cut(spec, "/")
		port, _ := strconv.Atoi(portStr)
		for _, b := range bindings {
			hostPort, _ := strconv.Atoi(b.HostPort)
			cont.Ports = append(cont.Ports, PortMapping{HostIP: b.HostIP, HostPort: hostPort, Container: port, Protocol: proto})
		}
	}
	sort.Slice(cont.Ports, func(i, j int) bool { return cont.Ports[i].HostPort < cont.Ports[j].HostPort })
	cont.Networks = map[string]string{}
	for name, ep := range info.NetworkSettings.Networks {
		cont.Networks[name] = ep.IPAddress
	}
}

var secretEnvHints = []string{"PASS", "SECRET", "TOKEN", "KEY", "PWD", "CREDENTIAL", "AUTH", "PRIVATE", "DSN", "DATABASE_URL", "COOKIE", "SALT", "HASH"}

func maskEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		upper := strings.ToUpper(key)
		masked := false
		for _, hint := range secretEnvHints {
			if strings.Contains(upper, hint) {
				masked = true
				break
			}
		}
		if masked && value != "" {
			out = append(out, key+"=••••••")
		} else {
			out = append(out, kv)
		}
	}
	sort.Strings(out)
	return out
}

func parseDockerTime(s string) int64 {
	if s == "" || strings.HasPrefix(s, "0001-") {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *Collector) persistInventory(ctx context.Context, now time.Time) {
	c.state.mu.RLock()
	records := make([]store.ContainerRecord, 0, len(c.state.containers))
	for _, cont := range c.state.containers {
		records = append(records, store.ContainerRecord{
			ID: cont.ID, Name: cont.Name, Image: cont.Image, ImageID: cont.ImageID, ImageDigest: cont.ImageDigest,
			Stack: cont.Stack, Service: cont.Service, EndpointID: cont.EndpointID, State: cont.State, Health: cont.Health,
			RestartCount: cont.RestartCount, Created: cont.Created, StartedAt: cont.StartedAt, ExitCode: cont.ExitCode,
		})
	}
	c.state.mu.RUnlock()
	if err := c.store.UpsertContainers(ctx, now, records); err != nil {
		c.log.Warn("persist containers", "error", err)
	}
}

func (c *Collector) publishContainers() {
	snap := c.Snapshot()
	c.bus.Publish("containers", map[string]any{"containers": snap.Containers, "stacks": snap.Stacks})
}
