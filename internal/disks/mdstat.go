package disks

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"
)

type Array struct {
	Name         string        `json:"name"`
	Level        string        `json:"level"`
	State        string        `json:"state"`
	Active       bool          `json:"active"`
	Blocks       uint64        `json:"blocks"`
	Members      []ArrayMember `json:"members"`
	SlotsTotal   int           `json:"slotsTotal"`
	SlotsActive  int           `json:"slotsActive"`
	Degraded     bool          `json:"degraded"`
	SyncAction   string        `json:"syncAction"`
	SyncPercent  float64       `json:"syncPercent"`
	SyncFinishIn string        `json:"syncFinishIn"`
}

type ArrayMember struct {
	Device string `json:"device"`
	Slot   int    `json:"slot"`
	Faulty bool   `json:"faulty"`
	Spare  bool   `json:"spare"`
}

var (
	memberRe = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\[(\d+)\](\([A-Z]\))*$`)
	slotsRe  = regexp.MustCompile(`\[(\d+)/(\d+)\]\s+\[([U_]+)\]`)
	syncRe   = regexp.MustCompile(`(resync|recovery|reshape|check|repair)\s*=\s*([\d.]+)%`)
	finishRe = regexp.MustCompile(`finish=([\d.]+)(min|sec|hour|day)`)
	blocksRe = regexp.MustCompile(`^\s*(\d+) blocks`)
)

func ParseMdstat(raw string) []Array {
	var arrays []Array
	var current *Array
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current != nil {
				arrays = append(arrays, *current)
				current = nil
			}
			continue
		}
		if strings.HasPrefix(trimmed, "Personalities") || strings.HasPrefix(trimmed, "unused devices") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.Contains(line, " : ") {
			if current != nil {
				arrays = append(arrays, *current)
			}
			current = parseArrayHeader(line)
			continue
		}
		if current == nil {
			continue
		}
		parseArrayDetail(current, trimmed)
	}
	if current != nil {
		arrays = append(arrays, *current)
	}
	for i := range arrays {
		finalizeState(&arrays[i])
	}
	return arrays
}

func parseArrayHeader(line string) *Array {
	name, rest, _ := strings.Cut(line, " : ")
	a := &Array{Name: strings.TrimSpace(name)}
	fields := strings.Fields(rest)
	for _, f := range fields {
		switch {
		case f == "active":
			a.Active = true
		case f == "inactive":
			a.Active = false
		case f == "(read-only)" || f == "(auto-read-only)":
		case strings.HasPrefix(f, "raid") || f == "linear" || f == "multipath":
			a.Level = f
		default:
			if m := memberRe.FindStringSubmatch(f); m != nil {
				slot, _ := strconv.Atoi(m[2])
				member := ArrayMember{Device: m[1], Slot: slot}
				if strings.Contains(f, "(F)") {
					member.Faulty = true
				}
				if strings.Contains(f, "(S)") {
					member.Spare = true
				}
				a.Members = append(a.Members, member)
			}
		}
	}
	return a
}

func parseArrayDetail(a *Array, line string) {
	if m := blocksRe.FindStringSubmatch(line); m != nil {
		a.Blocks, _ = strconv.ParseUint(m[1], 10, 64)
	}
	if m := slotsRe.FindStringSubmatch(line); m != nil {
		a.SlotsTotal, _ = strconv.Atoi(m[1])
		a.SlotsActive, _ = strconv.Atoi(m[2])
		a.Degraded = strings.Contains(m[3], "_")
	}
	if m := syncRe.FindStringSubmatch(line); m != nil {
		a.SyncAction = m[1]
		a.SyncPercent, _ = strconv.ParseFloat(m[2], 64)
	}
	if m := finishRe.FindStringSubmatch(line); m != nil {
		a.SyncFinishIn = m[1] + " " + m[2]
	}
}

func finalizeState(a *Array) {
	switch {
	case !a.Active:
		a.State = "inactive"
	case a.SyncAction == "recovery" || a.SyncAction == "resync" || a.SyncAction == "reshape":
		a.State = a.SyncAction
	case a.Degraded:
		a.State = "degraded"
	case a.SyncAction == "check" || a.SyncAction == "repair":
		a.State = a.SyncAction
	default:
		a.State = "clean"
	}
	for _, m := range a.Members {
		if m.Faulty {
			a.Degraded = true
			if a.State == "clean" {
				a.State = "degraded"
			}
		}
	}
}

func (a Array) Healthy() bool {
	return a.Active && !a.Degraded
}
