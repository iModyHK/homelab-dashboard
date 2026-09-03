package store

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRollupAggregatesAndPrunes(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	bucketStart := now.Add(-30 * time.Minute).Truncate(5 * time.Minute)

	var samples []Sample
	for i := 0; i < 20; i++ {
		samples = append(samples, Sample{
			ContainerID: "abc",
			TS:          bucketStart.Add(time.Duration(i) * 15 * time.Second),
			CPUPct:      float64(i + 1),
			MemBytes:    uint64(1000 + i*10),
			MemLimit:    8192,
			NetRx:       100,
			NetTx:       50,
		})
	}
	samples = append(samples, Sample{ContainerID: "abc", TS: now.Add(-2 * 24 * time.Hour), CPUPct: 99, MemBytes: 1, MemLimit: 1})
	if err := s.InsertSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertHostSample(ctx, HostSample{
		TS: bucketStart.Add(time.Minute), CPUPct: 10, Load1: 1, MemUsed: 100, MemTotal: 200,
		Mounts: []MountSample{{Mount: "/", Used: 50, Total: 100}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertHostSample(ctx, HostSample{
		TS: bucketStart.Add(2 * time.Minute), CPUPct: 30, Load1: 3, MemUsed: 300, MemTotal: 200,
		Mounts: []MountSample{{Mount: "/", Used: 70, Total: 100}},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := s.Rollup(ctx, now, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.ContainerRows != 2 {
		t.Fatalf("expected the live bucket plus the old one, got %d", res.ContainerRows)
	}
	if res.HostRows != 1 || res.MountRows != 1 {
		t.Fatalf("expected host and mount buckets, got %+v", res)
	}
	if res.PrunedRaw != 1 {
		t.Fatalf("expected the 2-day-old raw sample pruned, got %d", res.PrunedRaw)
	}

	var cpuAvg, cpuMax float64
	var memAvg, memMax, count int64
	err = s.db.QueryRowContext(ctx, "SELECT cpu_avg, cpu_max, mem_avg, mem_max, samples FROM samples_5m WHERE container_id = 'abc' AND ts = ?", bucketStart.Unix()).
		Scan(&cpuAvg, &cpuMax, &memAvg, &memMax, &count)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(cpuAvg-10.5) > 0.001 || cpuMax != 20 || count != 20 {
		t.Fatalf("cpu avg %.3f max %.1f samples %d", cpuAvg, cpuMax, count)
	}
	if memAvg != 1095 || memMax != 1190 {
		t.Fatalf("mem avg %d max %d", memAvg, memMax)
	}

	var hostCPU, hostMax float64
	if err := s.db.QueryRowContext(ctx, "SELECT cpu_avg, cpu_max FROM host_samples_5m").Scan(&hostCPU, &hostMax); err != nil {
		t.Fatal(err)
	}
	if hostCPU != 20 || hostMax != 30 {
		t.Fatalf("host cpu avg %.1f max %.1f", hostCPU, hostMax)
	}

	var usedAvg, usedMax int64
	if err := s.db.QueryRowContext(ctx, "SELECT used_avg, used_max FROM mount_samples_5m WHERE mount = '/'").Scan(&usedAvg, &usedMax); err != nil {
		t.Fatal(err)
	}
	if usedAvg != 60 || usedMax != 70 {
		t.Fatalf("mount used avg %d max %d", usedAvg, usedMax)
	}

	watermark, err := s.GetMeta(ctx, rollupWatermark)
	if err != nil || watermark == "" {
		t.Fatalf("watermark %q err %v", watermark, err)
	}

	again, err := s.Rollup(ctx, now, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if again.ContainerRows != 0 {
		t.Fatalf("second rollup must be idempotent, got %d rows", again.ContainerRows)
	}
}

func TestRollupSkipsIncompleteBucket(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Date(2026, 9, 3, 12, 2, 30, 0, time.UTC)
	openBucket := now.Truncate(5 * time.Minute)
	if err := s.InsertSamples(ctx, []Sample{
		{ContainerID: "x", TS: openBucket.Add(30 * time.Second), CPUPct: 50, MemLimit: 1},
		{ContainerID: "x", TS: openBucket.Add(-30 * time.Second), CPUPct: 10, MemLimit: 1},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Rollup(ctx, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.ContainerRows != 1 {
		t.Fatalf("only the closed bucket should roll up, got %d", res.ContainerRows)
	}
	var ts int64
	if err := s.db.QueryRowContext(ctx, "SELECT ts FROM samples_5m WHERE container_id = 'x'").Scan(&ts); err != nil {
		t.Fatal(err)
	}
	if ts >= openBucket.Unix() {
		t.Fatalf("rolled up the open bucket at %d", ts)
	}
}

func TestRollupRetentionPrunesOldRollups(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO samples_5m VALUES ('old', ?, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1)`,
		now.Add(-100*24*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO samples_5m VALUES ('new', ?, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1)`,
		now.Add(-10*24*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	res, err := s.Rollup(ctx, now, 90*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.PrunedRollups != 1 {
		t.Fatalf("expected 1 pruned rollup, got %d", res.PrunedRollups)
	}
	var remaining int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM samples_5m").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 remaining, got %d", remaining)
	}
}

func TestContainerSeriesPicksTableByRange(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	now := time.Now()
	var samples []Sample
	for i := 0; i < 240; i++ {
		samples = append(samples, Sample{ContainerID: "c", TS: now.Add(-time.Duration(i) * 15 * time.Second), CPUPct: 5, MemBytes: 10, MemLimit: 100})
	}
	if err := s.InsertSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	pts, err := s.ContainerSeries(ctx, "c", now.Add(-time.Hour), now, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 || len(pts) > 400 {
		t.Fatalf("raw series returned %d points", len(pts))
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO samples_5m VALUES ('c', ?, 7, 9, 10, 12, 100, 0, 0, 0, 0, 0, 0, 0, 0, 20)`,
		now.Add(-3*24*time.Hour).Truncate(5*time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	pts, err = s.ContainerSeries(ctx, "c", now.Add(-7*24*time.Hour), now, 400)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 1 || pts[0].CPU != 7 || pts[0].CPUMax != 9 {
		t.Fatalf("5m series %+v", pts)
	}
}
