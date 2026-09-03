package alerts

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iModyHK/homelab-dashboard/internal/collector"
	"github.com/iModyHK/homelab-dashboard/internal/config"
	"github.com/iModyHK/homelab-dashboard/internal/store"
)

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeNotifier) Send(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, text)
	return nil
}

func (f *fakeNotifier) Name() string { return "fake" }

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func newTestEngine(t *testing.T) (*Engine, *fakeNotifier, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := &config.Config{
		Timezone:   time.UTC,
		DBMaxBytes: 1 << 30,
		Alerts: config.AlertThresholds{
			CPUPercent: 90, MemoryPercent: 90, DiskPercent: 90, DiskTempCelsius: 55,
			RestartLoopCount: 3, RestartLoopWindow: 10 * time.Minute,
			SustainedFor: 50 * time.Millisecond, Debounce: 50 * time.Millisecond, HostUnreachableFor: 50 * time.Millisecond,
		},
	}
	n := &fakeNotifier{}
	e := NewEngine(cfg, st, collector.NewBus(), n, slog.New(slog.NewTextHandler(discard{}, nil)))
	return e, n, st
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func downSnapshot(state string) collector.Snapshot {
	return collector.Snapshot{
		Host:    collector.HostState{CPUs: 4, MemTotal: 100, MemUsed: 10},
		Sources: map[string]collector.SourceStatus{"docker": {OK: true}, "portainer": {OK: true}},
		Containers: []*collector.Container{{
			ID: "c1", Name: "immich", State: state, RestartPolicy: "unless-stopped", ExitCode: 137,
		}},
	}
}

func TestContainerDownFiresAfterHoldAndResolvesAfterDebounce(t *testing.T) {
	e, n, st := newTestEngine(t)
	ctx := context.Background()
	go e.Run(ctx)

	e.Evaluate(ctx, downSnapshot("exited"), nil)
	firing, _ := st.FiringAlerts(ctx)
	if len(firing) != 0 {
		t.Fatalf("must not fire before the hold elapses, got %d", len(firing))
	}

	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, downSnapshot("exited"), nil)
	firing, _ = st.FiringAlerts(ctx)
	if len(firing) != 1 || firing[0].Rule != "container_down" || firing[0].TargetName != "immich" {
		t.Fatalf("expected one container_down alert, got %+v", firing)
	}

	e.Evaluate(ctx, downSnapshot("exited"), nil)
	firing, _ = st.FiringAlerts(ctx)
	if len(firing) != 1 {
		t.Fatalf("a firing alert must not duplicate, got %d", len(firing))
	}

	e.Evaluate(ctx, downSnapshot("running"), nil)
	firing, _ = st.FiringAlerts(ctx)
	if len(firing) != 1 {
		t.Fatal("must not resolve before the debounce window")
	}
	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, downSnapshot("running"), nil)
	firing, _ = st.FiringAlerts(ctx)
	if len(firing) != 0 {
		t.Fatal("expected the alert to resolve")
	}

	deadline := time.Now().Add(2 * time.Second)
	for n.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n.count() != 2 {
		t.Fatalf("expected one firing and one resolved notification, got %d", n.count())
	}
}

func TestThresholdHysteresis(t *testing.T) {
	e, _, st := newTestEngine(t)
	ctx := context.Background()
	snap := func(cpu float64) collector.Snapshot {
		return collector.Snapshot{
			Host:    collector.HostState{CPUs: 1, MemTotal: 100, MemUsed: 10},
			Sources: map[string]collector.SourceStatus{"docker": {OK: true}},
			Containers: []*collector.Container{{
				ID: "c2", Name: "db", State: "running", Live: &collector.Live{CPUPercent: cpu, MemLimit: 100, MemBytes: 1},
			}},
		}
	}
	e.Evaluate(ctx, snap(95), nil)
	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, snap(95), nil)
	firing, _ := st.FiringAlerts(ctx)
	if len(firing) != 1 || firing[0].Rule != "cpu_high" {
		t.Fatalf("expected cpu_high, got %+v", firing)
	}

	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, snap(87), nil)
	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, snap(87), nil)
	firing, _ = st.FiringAlerts(ctx)
	if len(firing) != 1 {
		t.Fatal("87% is inside the hysteresis band and must keep firing")
	}

	e.Evaluate(ctx, snap(50), nil)
	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, snap(50), nil)
	firing, _ = st.FiringAlerts(ctx)
	if len(firing) != 0 {
		t.Fatal("50% must resolve the alert")
	}
}

func TestRestoreDoesNotRefire(t *testing.T) {
	e, _, st := newTestEngine(t)
	ctx := context.Background()
	if _, err := st.OpenAlert(ctx, store.AlertRecord{Rule: "container_down", Target: "c1", TargetName: "immich", Severity: "critical", Message: "x", FiredAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	if err := e.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(70 * time.Millisecond)
	e.Evaluate(ctx, downSnapshot("exited"), nil)
	all, _ := st.Alerts(ctx, 10)
	if len(all) != 1 {
		t.Fatalf("restored alert must not be duplicated, got %d", len(all))
	}
}

func TestRaidDegradedFiresImmediately(t *testing.T) {
	e, _, st := newTestEngine(t)
	ctx := context.Background()
	snap := collector.Snapshot{Sources: map[string]collector.SourceStatus{"docker": {OK: true}}}
	snap.Arrays = append(snap.Arrays, degradedArray())
	e.Evaluate(ctx, snap, nil)
	firing, _ := st.FiringAlerts(ctx)
	if len(firing) != 1 || firing[0].Rule != "raid_degraded" {
		t.Fatalf("expected raid_degraded immediately, got %+v", firing)
	}
}
