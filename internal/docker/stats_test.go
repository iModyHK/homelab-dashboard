package docker

import (
	"math"
	"testing"
)

func TestCPUPercentFromDelta(t *testing.T) {
	prev := CPUSnapshot{Total: 1_000_000_000, System: 100_000_000_000, CPUs: 4}
	cur := CPUSnapshot{Total: 1_500_000_000, System: 101_000_000_000, CPUs: 4}
	pct, ok := CPUPercent(prev, cur)
	if !ok {
		t.Fatal("expected a valid sample")
	}
	if math.Abs(pct-200) > 0.001 {
		t.Fatalf("expected 200%%, got %.3f", pct)
	}
}

func TestCPUPercentDiscardsFirstSample(t *testing.T) {
	cur := CPUSnapshot{Total: 5, System: 10, CPUs: 2}
	if _, ok := CPUPercent(CPUSnapshot{}, cur); ok {
		t.Fatal("first sample must be discarded")
	}
}

func TestCPUPercentDiscardsCounterReset(t *testing.T) {
	prev := CPUSnapshot{Total: 900, System: 9000, CPUs: 2}
	cur := CPUSnapshot{Total: 100, System: 10000, CPUs: 2}
	if _, ok := CPUPercent(prev, cur); ok {
		t.Fatal("a container restart resets counters and must be discarded")
	}
}

func TestCPUPercentDiscardsZeroSystemDelta(t *testing.T) {
	prev := CPUSnapshot{Total: 100, System: 1000, CPUs: 2}
	cur := CPUSnapshot{Total: 200, System: 1000, CPUs: 2}
	if _, ok := CPUPercent(prev, cur); ok {
		t.Fatal("zero system delta must not divide by zero")
	}
}

func TestMemoryUsedSubtractsInactiveFile(t *testing.T) {
	m := MemoryStats{Usage: 1000, Stats: map[string]uint64{"inactive_file": 300}}
	if got := MemoryUsed(m); got != 700 {
		t.Fatalf("expected 700, got %d", got)
	}
}

func TestMemoryUsedFallsBackToCacheOnCgroupV1(t *testing.T) {
	m := MemoryStats{Usage: 1000, Stats: map[string]uint64{"cache": 250}}
	if got := MemoryUsed(m); got != 750 {
		t.Fatalf("expected 750, got %d", got)
	}
}

func TestMemoryUsedNeverUnderflows(t *testing.T) {
	m := MemoryStats{Usage: 100, Stats: map[string]uint64{"inactive_file": 500}}
	if got := MemoryUsed(m); got != 100 {
		t.Fatalf("expected raw usage 100, got %d", got)
	}
}

func TestExtractUsageSequence(t *testing.T) {
	first := Stats{
		CPUStats:    CPUStats{CPUUsage: CPUUsage{TotalUsage: 1000}, SystemCPUUsage: 100000, OnlineCPUs: 4},
		MemoryStats: MemoryStats{Usage: 2048, Limit: 8192, Stats: map[string]uint64{"inactive_file": 48}},
		Networks:    map[string]NetStats{"eth0": {RxBytes: 10, TxBytes: 20}, "eth1": {RxBytes: 5, TxBytes: 5}},
		BlkioStats:  BlkioStats{IoServiceBytesRecursive: []BlkioEntry{{Op: "Read", Value: 100}, {Op: "Write", Value: 200}}},
	}
	u, snap, ok := ExtractUsage(nil, first)
	if ok {
		t.Fatal("first extraction must report no CPU figure")
	}
	if u.MemBytes != 2000 || u.NetRx != 15 || u.NetTx != 25 || u.BlkRead != 100 || u.BlkWrite != 200 {
		t.Fatalf("unexpected usage %+v", u)
	}

	second := first
	second.CPUStats = CPUStats{CPUUsage: CPUUsage{TotalUsage: 2000}, SystemCPUUsage: 104000, OnlineCPUs: 4}
	u2, _, ok := ExtractUsage(&snap, second)
	if !ok {
		t.Fatal("second extraction must produce CPU")
	}
	if math.Abs(u2.CPUPercent-100) > 0.001 {
		t.Fatalf("expected 100%%, got %.3f", u2.CPUPercent)
	}
}
