package docker

type Usage struct {
	CPUPercent float64
	MemBytes   uint64
	MemLimit   uint64
	NetRx      uint64
	NetTx      uint64
	BlkRead    uint64
	BlkWrite   uint64
	Pids       uint64
}

type CPUSnapshot struct {
	Total  uint64
	System uint64
	CPUs   uint32
}

func SnapshotCPU(s Stats) CPUSnapshot {
	cpus := s.CPUStats.OnlineCPUs
	if cpus == 0 {
		cpus = uint32(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpus == 0 {
		cpus = 1
	}
	return CPUSnapshot{
		Total:  s.CPUStats.CPUUsage.TotalUsage,
		System: s.CPUStats.SystemCPUUsage,
		CPUs:   cpus,
	}
}

func CPUPercent(prev, cur CPUSnapshot) (float64, bool) {
	if prev.Total == 0 && prev.System == 0 {
		return 0, false
	}
	if cur.Total < prev.Total || cur.System <= prev.System {
		return 0, false
	}
	cpuDelta := float64(cur.Total - prev.Total)
	sysDelta := float64(cur.System - prev.System)
	if sysDelta <= 0 {
		return 0, false
	}
	pct := cpuDelta / sysDelta * float64(cur.CPUs) * 100
	if pct < 0 {
		pct = 0
	}
	return pct, true
}

func MemoryUsed(m MemoryStats) uint64 {
	usage := m.Usage
	if inactive, ok := m.Stats["inactive_file"]; ok && inactive < usage {
		return usage - inactive
	}
	if cache, ok := m.Stats["cache"]; ok && cache < usage {
		return usage - cache
	}
	return usage
}

func NetworkTotals(s Stats) (rx, tx uint64) {
	for _, n := range s.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}
	return rx, tx
}

func BlockTotals(s Stats) (read, write uint64) {
	for _, e := range s.BlkioStats.IoServiceBytesRecursive {
		switch e.Op {
		case "Read", "read":
			read += e.Value
		case "Write", "write":
			write += e.Value
		}
	}
	return read, write
}

func ExtractUsage(prev *CPUSnapshot, s Stats) (Usage, CPUSnapshot, bool) {
	cur := SnapshotCPU(s)
	u := Usage{
		MemBytes: MemoryUsed(s.MemoryStats),
		MemLimit: s.MemoryStats.Limit,
		Pids:     s.PidsStats.Current,
	}
	u.NetRx, u.NetTx = NetworkTotals(s)
	u.BlkRead, u.BlkWrite = BlockTotals(s)
	if prev == nil {
		return u, cur, false
	}
	pct, ok := CPUPercent(*prev, cur)
	u.CPUPercent = pct
	return u, cur, ok
}
