package collector

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/disks"
	"github.com/iModyHK/homelab-dashboard/internal/docker"
	"github.com/iModyHK/homelab-dashboard/internal/host"
)

type SourceStatus struct {
	OK        bool   `json:"ok"`
	LastOK    int64  `json:"lastOk"`
	LastError string `json:"lastError"`
	ErrorAt   int64  `json:"errorAt"`
}

type PortMapping struct {
	HostIP    string `json:"hostIp"`
	HostPort  int    `json:"hostPort"`
	Container int    `json:"containerPort"`
	Protocol  string `json:"protocol"`
}

type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readOnly"`
}

type HealthCheck struct {
	Start    int64  `json:"start"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

type Live struct {
	TS         int64   `json:"ts"`
	CPUPercent float64 `json:"cpu"`
	MemBytes   uint64  `json:"mem"`
	MemLimit   uint64  `json:"memLimit"`
	NetRxRate  uint64  `json:"netRx"`
	NetTxRate  uint64  `json:"netTx"`
	BlkRdRate  uint64  `json:"blkRead"`
	BlkWrRate  uint64  `json:"blkWrite"`
	Pids       uint64  `json:"pids"`
}

type Container struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Image         string            `json:"image"`
	ImageID       string            `json:"imageId"`
	ImageDigest   string            `json:"imageDigest"`
	Stack         string            `json:"stack"`
	StackSource   string            `json:"stackSource"`
	Service       string            `json:"service"`
	EndpointID    int               `json:"endpointId"`
	State         string            `json:"state"`
	Status        string            `json:"status"`
	Health        string            `json:"health"`
	FailingStreak int               `json:"failingStreak"`
	HealthLog     []HealthCheck     `json:"healthLog"`
	RestartCount  int               `json:"restartCount"`
	RestartPolicy string            `json:"restartPolicy"`
	Created       int64             `json:"created"`
	StartedAt     int64             `json:"startedAt"`
	FinishedAt    int64             `json:"finishedAt"`
	ExitCode      int               `json:"exitCode"`
	OOMKilled     bool              `json:"oomKilled"`
	Error         string            `json:"error"`
	Tty           bool              `json:"tty"`
	Ports         []PortMapping     `json:"ports"`
	Mounts        []MountInfo       `json:"mounts"`
	Env           []string          `json:"env"`
	Labels        map[string]string `json:"labels"`
	Networks      map[string]string `json:"networks"`
	MemoryLimit   int64             `json:"memoryLimit"`
	CPULimit      float64           `json:"cpuLimit"`
	UpdateReady   bool              `json:"updateAvailable"`
	Live          *Live             `json:"live"`
	Sparkline     []float64         `json:"sparkline,omitempty"`
	LastInspected time.Time         `json:"-"`
}

func (c *Container) Running() bool {
	return c.State == "running"
}

type StackSummary struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	EndpointID  int     `json:"endpointId"`
	Total       int     `json:"total"`
	Running     int     `json:"running"`
	Unhealthy   int     `json:"unhealthy"`
	Exited      int     `json:"exited"`
	CPUPercent  float64 `json:"cpu"`
	MemBytes    uint64  `json:"mem"`
	NetRxRate   uint64  `json:"netRx"`
	NetTxRate   uint64  `json:"netTx"`
	Updates     int     `json:"updates"`
	Health      string  `json:"health"`
	PortainerID int     `json:"portainerId"`
	StackType   string  `json:"stackType"`
	StackStatus string  `json:"stackStatus"`
}

type HostState struct {
	TS         int64             `json:"ts"`
	Hostname   string            `json:"hostname"`
	OS         string            `json:"os"`
	Kernel     string            `json:"kernel"`
	Docker     string            `json:"dockerVersion"`
	CPUs       int               `json:"cpus"`
	CPUPercent float64           `json:"cpu"`
	CPUTemp    float64           `json:"cpuTemp"`
	Load1      float64           `json:"load1"`
	Load5      float64           `json:"load5"`
	Load15     float64           `json:"load15"`
	MemUsed    uint64            `json:"memUsed"`
	MemTotal   uint64            `json:"memTotal"`
	SwapUsed   uint64            `json:"swapUsed"`
	SwapTotal  uint64            `json:"swapTotal"`
	NetRxRate  uint64            `json:"netRx"`
	NetTxRate  uint64            `json:"netTx"`
	Uptime     int64             `json:"uptime"`
	Mounts     []host.MountUsage `json:"mounts"`
}

type PortainerState struct {
	Version   string               `json:"version"`
	Endpoints []EndpointSummary    `json:"endpoints"`
	Stacks    map[string]StackMeta `json:"-"`
}

type EndpointSummary struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

type StackMeta struct {
	ID         int
	EndpointID int
	Type       string
	Status     string
}

type Snapshot struct {
	Host       HostState               `json:"host"`
	Portainer  PortainerState          `json:"portainer"`
	Stacks     []StackSummary          `json:"stacks"`
	Containers []*Container            `json:"containers"`
	Disks      []disks.Disk            `json:"disks"`
	Arrays     []disks.Array           `json:"arrays"`
	Sources    map[string]SourceStatus `json:"sources"`
	DBBytes    int64                   `json:"dbBytes"`
	StartedAt  int64                   `json:"startedAt"`
	Version    string                  `json:"version"`
}

type state struct {
	mu         sync.RWMutex
	host       HostState
	portainer  PortainerState
	containers map[string]*Container
	disks      []disks.Disk
	arrays     []disks.Array
	sources    map[string]SourceStatus
	updates    map[string]bool
}

func newState() *state {
	return &state{
		containers: map[string]*Container{},
		sources:    map[string]SourceStatus{},
		updates:    map[string]bool{},
		portainer:  PortainerState{Stacks: map[string]StackMeta{}},
	}
}

func (s *state) setSource(name string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.sources[name]
	now := time.Now().Unix()
	if err == nil {
		cur.OK = true
		cur.LastOK = now
		cur.LastError = ""
	} else {
		cur.OK = false
		cur.LastError = err.Error()
		cur.ErrorAt = now
	}
	s.sources[name] = cur
}

func (s *state) container(id string) (*Container, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.containers[id]
	if !ok {
		for _, cand := range s.containers {
			if strings.HasPrefix(cand.ID, id) {
				return cand, true
			}
		}
	}
	return c, ok
}

func (s *state) containerCopies() []*Container {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Container, 0, len(s.containers))
	for _, c := range s.containers {
		cp := *c
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Stack != out[j].Stack {
			return out[i].Stack < out[j].Stack
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *state) snapshot() Snapshot {
	containers := s.containerCopies()
	s.mu.RLock()
	defer s.mu.RUnlock()
	sources := make(map[string]SourceStatus, len(s.sources))
	for k, v := range s.sources {
		sources[k] = v
	}
	for _, c := range containers {
		c.UpdateReady = s.updates[c.Image]
	}
	return Snapshot{
		Host:       s.host,
		Portainer:  s.portainer,
		Stacks:     summarizeStacks(containers, s.portainer.Stacks),
		Containers: containers,
		Disks:      append([]disks.Disk(nil), s.disks...),
		Arrays:     append([]disks.Array(nil), s.arrays...),
		Sources:    sources,
	}
}

func summarizeStacks(containers []*Container, meta map[string]StackMeta) []StackSummary {
	byName := map[string]*StackSummary{}
	for _, c := range containers {
		st, ok := byName[c.Stack]
		if !ok {
			st = &StackSummary{Name: c.Stack, Source: c.StackSource, EndpointID: c.EndpointID}
			if m, ok := meta[c.Stack]; ok {
				st.PortainerID = m.ID
				st.StackType = m.Type
				st.StackStatus = m.Status
			}
			byName[c.Stack] = st
		}
		st.Total++
		switch {
		case c.Running():
			st.Running++
		case c.State == "exited" || c.State == "dead":
			st.Exited++
		}
		if c.Health == "unhealthy" {
			st.Unhealthy++
		}
		if c.UpdateReady {
			st.Updates++
		}
		if c.Live != nil && c.Running() {
			st.CPUPercent += c.Live.CPUPercent
			st.MemBytes += c.Live.MemBytes
			st.NetRxRate += c.Live.NetRxRate
			st.NetTxRate += c.Live.NetTxRate
		}
	}
	out := make([]StackSummary, 0, len(byName))
	for _, st := range byName {
		switch {
		case st.Unhealthy > 0:
			st.Health = "unhealthy"
		case st.Running == 0 && st.Total > 0:
			st.Health = "down"
		case st.Running < st.Total:
			st.Health = "partial"
		default:
			st.Health = "healthy"
		}
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == unmanagedStack {
			return false
		}
		if out[j].Name == unmanagedStack {
			return true
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func liveFromUsage(ts time.Time, u docker.Usage, rates rateSet) *Live {
	return &Live{
		TS:         ts.Unix(),
		CPUPercent: u.CPUPercent,
		MemBytes:   u.MemBytes,
		MemLimit:   u.MemLimit,
		NetRxRate:  rates.netRx,
		NetTxRate:  rates.netTx,
		BlkRdRate:  rates.blkRead,
		BlkWrRate:  rates.blkWrite,
		Pids:       u.Pids,
	}
}
