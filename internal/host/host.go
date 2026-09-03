package host

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Reader struct {
	procPath string
	sysPath  string
	rootPath string
}

func New(procPath, sysPath, rootPath string) *Reader {
	return &Reader{procPath: procPath, sysPath: sysPath, rootPath: rootPath}
}

type CPUTimes struct {
	User    uint64
	Nice    uint64
	System  uint64
	Idle    uint64
	IOWait  uint64
	IRQ     uint64
	SoftIRQ uint64
	Steal   uint64
}

func (c CPUTimes) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.IOWait + c.IRQ + c.SoftIRQ + c.Steal
}

func (c CPUTimes) Busy() uint64 {
	return c.Total() - c.Idle - c.IOWait
}

func CPUPercent(prev, cur CPUTimes) (float64, bool) {
	total := cur.Total()
	prevTotal := prev.Total()
	if prevTotal == 0 || total <= prevTotal {
		return 0, false
	}
	busy := cur.Busy()
	prevBusy := prev.Busy()
	if busy < prevBusy {
		return 0, false
	}
	return float64(busy-prevBusy) / float64(total-prevTotal) * 100, true
}

func (r *Reader) CPU() (CPUTimes, error) {
	f, err := os.Open(filepath.Join(r.procPath, "stat"))
	if err != nil {
		return CPUTimes{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 8 || fields[0] != "cpu" {
			continue
		}
		vals := make([]uint64, 8)
		for i := range vals {
			if i+1 < len(fields) {
				vals[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
			}
		}
		return CPUTimes{
			User: vals[0], Nice: vals[1], System: vals[2], Idle: vals[3],
			IOWait: vals[4], IRQ: vals[5], SoftIRQ: vals[6], Steal: vals[7],
		}, nil
	}
	return CPUTimes{}, errors.New("no aggregate cpu line in /proc/stat")
}

type Memory struct {
	Total     uint64
	Available uint64
	SwapTotal uint64
	SwapFree  uint64
}

func (m Memory) Used() uint64 {
	if m.Available > m.Total {
		return 0
	}
	return m.Total - m.Available
}

func (m Memory) SwapUsed() uint64 {
	if m.SwapFree > m.SwapTotal {
		return 0
	}
	return m.SwapTotal - m.SwapFree
}

func (r *Reader) Memory() (Memory, error) {
	f, err := os.Open(filepath.Join(r.procPath, "meminfo"))
	if err != nil {
		return Memory{}, err
	}
	defer f.Close()
	var m Memory
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		bytes := kb * 1024
		switch key {
		case "MemTotal":
			m.Total = bytes
		case "MemAvailable":
			m.Available = bytes
		case "SwapTotal":
			m.SwapTotal = bytes
		case "SwapFree":
			m.SwapFree = bytes
		}
	}
	if m.Total == 0 {
		return m, errors.New("MemTotal missing from meminfo")
	}
	return m, nil
}

type Load struct {
	One     float64
	Five    float64
	Fifteen float64
}

func (r *Reader) Load() (Load, error) {
	data, err := os.ReadFile(filepath.Join(r.procPath, "loadavg"))
	if err != nil {
		return Load{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return Load{}, errors.New("loadavg malformed")
	}
	var l Load
	l.One, _ = strconv.ParseFloat(fields[0], 64)
	l.Five, _ = strconv.ParseFloat(fields[1], 64)
	l.Fifteen, _ = strconv.ParseFloat(fields[2], 64)
	return l, nil
}

func (r *Reader) Uptime() (time.Duration, error) {
	data, err := os.ReadFile(filepath.Join(r.procPath, "uptime"))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, errors.New("uptime malformed")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs * float64(time.Second)), nil
}

type NetCounters struct {
	RxBytes uint64
	TxBytes uint64
}

func (r *Reader) Network() (NetCounters, error) {
	f, err := os.Open(filepath.Join(r.procPath, "net", "dev"))
	if err != nil {
		return NetCounters{}, err
	}
	defer f.Close()
	var total NetCounters
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if isVirtualInterface(name) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		total.RxBytes += rx
		total.TxBytes += tx
	}
	return total, sc.Err()
}

func isVirtualInterface(name string) bool {
	prefixes := []string{"lo", "veth", "docker", "br-", "virbr", "tailscale", "wg", "tun", "dummy"}
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

type MountUsage struct {
	Mount string
	Used  uint64
	Total uint64
	Free  uint64
}

func (r *Reader) Mounts(mounts []string) ([]MountUsage, error) {
	var out []MountUsage
	var errs []error
	for _, m := range mounts {
		u, err := statMount(filepath.Join(r.rootPath, m))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m, err))
			continue
		}
		u.Mount = m
		out = append(out, u)
	}
	return out, errors.Join(errs...)
}

func (r *Reader) CPUTemperature() (float64, bool) {
	hwmon := filepath.Join(r.sysPath, "class", "hwmon")
	entries, err := os.ReadDir(hwmon)
	if err != nil {
		return 0, false
	}
	var fallback float64
	var hasFallback bool
	for _, e := range entries {
		dir := filepath.Join(hwmon, e.Name())
		nameBytes, err := os.ReadFile(filepath.Join(dir, "name"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(nameBytes))
		raw, err := os.ReadFile(filepath.Join(dir, "temp1_input"))
		if err != nil {
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err != nil {
			continue
		}
		celsius := milli / 1000
		switch name {
		case "coretemp", "k10temp", "zenpower", "cpu_thermal":
			return celsius, true
		}
		if !hasFallback {
			fallback, hasFallback = celsius, true
		}
	}
	return fallback, hasFallback
}
