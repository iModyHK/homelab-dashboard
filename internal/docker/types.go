package docker

import "time"

type ContainerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Labels  map[string]string `json:"Labels"`
	Ports   []Port            `json:"Ports"`
}

type Port struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

func (c ContainerSummary) Name() string {
	if len(c.Names) == 0 {
		return c.ID[:min(12, len(c.ID))]
	}
	n := c.Names[0]
	if len(n) > 0 && n[0] == '/' {
		return n[1:]
	}
	return n
}

type ContainerInspect struct {
	ID              string          `json:"Id"`
	Name            string          `json:"Name"`
	Created         string          `json:"Created"`
	Image           string          `json:"Image"`
	RestartCount    int             `json:"RestartCount"`
	State           ContainerState  `json:"State"`
	Config          ContainerConfig `json:"Config"`
	HostConfig      HostConfig      `json:"HostConfig"`
	Mounts          []Mount         `json:"Mounts"`
	NetworkSettings NetworkSettings `json:"NetworkSettings"`
}

type ContainerState struct {
	Status     string  `json:"Status"`
	Running    bool    `json:"Running"`
	Paused     bool    `json:"Paused"`
	Restarting bool    `json:"Restarting"`
	OOMKilled  bool    `json:"OOMKilled"`
	Dead       bool    `json:"Dead"`
	Pid        int     `json:"Pid"`
	ExitCode   int     `json:"ExitCode"`
	Error      string  `json:"Error"`
	StartedAt  string  `json:"StartedAt"`
	FinishedAt string  `json:"FinishedAt"`
	Health     *Health `json:"Health"`
}

type Health struct {
	Status        string      `json:"Status"`
	FailingStreak int         `json:"FailingStreak"`
	Log           []HealthLog `json:"Log"`
}

type HealthLog struct {
	Start    string `json:"Start"`
	End      string `json:"End"`
	ExitCode int    `json:"ExitCode"`
	Output   string `json:"Output"`
}

type ContainerConfig struct {
	Hostname     string              `json:"Hostname"`
	Image        string              `json:"Image"`
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	Labels       map[string]string   `json:"Labels"`
	Tty          bool                `json:"Tty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
}

type HostConfig struct {
	RestartPolicy RestartPolicy            `json:"RestartPolicy"`
	Memory        int64                    `json:"Memory"`
	NanoCPUs      int64                    `json:"NanoCpus"`
	PortBindings  map[string][]PortBinding `json:"PortBindings"`
}

type RestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type Mount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
}

type NetworkSettings struct {
	Networks map[string]EndpointSettings `json:"Networks"`
}

type EndpointSettings struct {
	IPAddress string `json:"IPAddress"`
	Gateway   string `json:"Gateway"`
}

type Stats struct {
	Read        time.Time           `json:"read"`
	PreRead     time.Time           `json:"preread"`
	CPUStats    CPUStats            `json:"cpu_stats"`
	PreCPUStats CPUStats            `json:"precpu_stats"`
	MemoryStats MemoryStats         `json:"memory_stats"`
	Networks    map[string]NetStats `json:"networks"`
	BlkioStats  BlkioStats          `json:"blkio_stats"`
	PidsStats   PidsStats           `json:"pids_stats"`
}

type CPUStats struct {
	CPUUsage       CPUUsage `json:"cpu_usage"`
	SystemCPUUsage uint64   `json:"system_cpu_usage"`
	OnlineCPUs     uint32   `json:"online_cpus"`
}

type CPUUsage struct {
	TotalUsage  uint64   `json:"total_usage"`
	PercpuUsage []uint64 `json:"percpu_usage"`
}

type MemoryStats struct {
	Usage uint64            `json:"usage"`
	Limit uint64            `json:"limit"`
	Stats map[string]uint64 `json:"stats"`
}

type NetStats struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}

type BlkioStats struct {
	IoServiceBytesRecursive []BlkioEntry `json:"io_service_bytes_recursive"`
}

type BlkioEntry struct {
	Op    string `json:"op"`
	Value uint64 `json:"value"`
}

type PidsStats struct {
	Current uint64 `json:"current"`
}

type Event struct {
	Type     string     `json:"Type"`
	Action   string     `json:"Action"`
	Actor    EventActor `json:"Actor"`
	Time     int64      `json:"time"`
	TimeNano int64      `json:"timeNano"`
}

type EventActor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

type ImageSummary struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	RepoDigests []string `json:"RepoDigests"`
	Created     int64    `json:"Created"`
	Size        int64    `json:"Size"`
}

type Version struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	Os         string `json:"Os"`
	Arch       string `json:"Arch"`
}

type Info struct {
	ID                string `json:"ID"`
	Name              string `json:"Name"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	Images            int    `json:"Images"`
	NCPU              int    `json:"NCPU"`
	MemTotal          int64  `json:"MemTotal"`
	ServerVersion     string `json:"ServerVersion"`
	OperatingSystem   string `json:"OperatingSystem"`
	KernelVersion     string `json:"KernelVersion"`
}

const (
	LabelComposeProject = "com.docker.compose.project"
	LabelComposeService = "com.docker.compose.service"
)
