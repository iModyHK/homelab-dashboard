package host

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeProc(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCPUPercentFromProcStat(t *testing.T) {
	prev := CPUTimes{User: 100, System: 50, Idle: 850}
	cur := CPUTimes{User: 150, System: 70, Idle: 930}
	pct, ok := CPUPercent(prev, cur)
	if !ok {
		t.Fatal("expected valid")
	}
	if math.Abs(pct-46.666) > 0.01 {
		t.Fatalf("expected 46.67, got %.3f", pct)
	}
}

func TestReadersParseProcFiles(t *testing.T) {
	dir := t.TempDir()
	writeProc(t, dir, "stat", "cpu  10 2 5 100 3 0 1 0 0 0\ncpu0 5 1 2 50 1 0 0 0 0 0\n")
	writeProc(t, dir, "meminfo", "MemTotal:       7864320 kB\nMemFree:         250000 kB\nMemAvailable:   3145728 kB\nSwapTotal:      1998844 kB\nSwapFree:        102400 kB\n")
	writeProc(t, dir, "loadavg", "0.52 0.61 0.70 2/1200 4242\n")
	writeProc(t, dir, "uptime", "86400.55 300000.00\n")
	writeProc(t, dir, "net/dev", `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000 10 0 0 0 0 0 0 1000 10 0 0 0 0 0 0
  eth0: 5000 50 0 0 0 0 0 0 7000 70 0 0 0 0 0 0
veth1a: 9999 99 0 0 0 0 0 0 9999 99 0 0 0 0 0 0
docker0: 123 1 0 0 0 0 0 0 456 4 0 0 0 0 0 0
`)
	r := New(dir, dir)

	cpu, err := r.CPU()
	if err != nil || cpu.User != 10 || cpu.Idle != 100 || cpu.IOWait != 3 {
		t.Fatalf("cpu %+v err %v", cpu, err)
	}
	mem, err := r.Memory()
	if err != nil || mem.Total != 7864320*1024 || mem.Used() != (7864320-3145728)*1024 {
		t.Fatalf("mem %+v err %v", mem, err)
	}
	if mem.SwapUsed() != (1998844-102400)*1024 {
		t.Fatalf("swap %d", mem.SwapUsed())
	}
	load, err := r.Load()
	if err != nil || load.One != 0.52 || load.Fifteen != 0.70 {
		t.Fatalf("load %+v err %v", load, err)
	}
	up, err := r.Uptime()
	if err != nil || up.Hours() < 23.9 || up.Hours() > 24.1 {
		t.Fatalf("uptime %v err %v", up, err)
	}
	net, err := r.Network()
	if err != nil || net.RxBytes != 5000 || net.TxBytes != 7000 {
		t.Fatalf("net %+v err %v", net, err)
	}
}
