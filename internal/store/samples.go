package store

import (
	"context"
	"fmt"
	"time"
)

type Sample struct {
	ContainerID string
	TS          time.Time
	CPUPct      float64
	MemBytes    uint64
	MemLimit    uint64
	NetRx       uint64
	NetTx       uint64
	BlkRead     uint64
	BlkWrite    uint64
}

type HostSample struct {
	TS        time.Time
	CPUPct    float64
	Load1     float64
	Load5     float64
	Load15    float64
	MemUsed   uint64
	MemTotal  uint64
	SwapUsed  uint64
	SwapTotal uint64
	NetRx     uint64
	NetTx     uint64
	CPUTemp   float64
	Mounts    []MountSample
}

type MountSample struct {
	Mount string
	Used  uint64
	Total uint64
}

func (s *Store) InsertSamples(ctx context.Context, samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO samples_raw
		(container_id, ts, cpu_pct, mem_bytes, mem_limit, net_rx, net_tx, blk_read, blk_write)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, sm := range samples {
		if _, err := stmt.ExecContext(ctx, sm.ContainerID, unix(sm.TS), sm.CPUPct,
			int64(sm.MemBytes), int64(sm.MemLimit), int64(sm.NetRx), int64(sm.NetTx),
			int64(sm.BlkRead), int64(sm.BlkWrite)); err != nil {
			return fmt.Errorf("insert sample: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) InsertHostSample(ctx context.Context, h HostSample) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := unix(h.TS)
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO host_samples_raw
		(ts, cpu_pct, load1, load5, load15, mem_used, mem_total, swap_used, swap_total, net_rx, net_tx, cpu_temp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, h.CPUPct, h.Load1, h.Load5, h.Load15, int64(h.MemUsed), int64(h.MemTotal),
		int64(h.SwapUsed), int64(h.SwapTotal), int64(h.NetRx), int64(h.NetTx), h.CPUTemp); err != nil {
		return fmt.Errorf("insert host sample: %w", err)
	}
	for _, m := range h.Mounts {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO mount_samples_raw (ts, mount, used, total) VALUES (?, ?, ?, ?)`,
			ts, m.Mount, int64(m.Used), int64(m.Total)); err != nil {
			return fmt.Errorf("insert mount sample: %w", err)
		}
	}
	return tx.Commit()
}

type Point struct {
	TS       int64   `json:"ts"`
	CPU      float64 `json:"cpu"`
	CPUMax   float64 `json:"cpuMax"`
	Mem      int64   `json:"mem"`
	MemMax   int64   `json:"memMax"`
	MemLimit int64   `json:"memLimit"`
	NetRx    int64   `json:"netRx"`
	NetTx    int64   `json:"netTx"`
	BlkRead  int64   `json:"blkRead"`
	BlkWrite int64   `json:"blkWrite"`
}

type HostPoint struct {
	TS        int64   `json:"ts"`
	CPU       float64 `json:"cpu"`
	CPUMax    float64 `json:"cpuMax"`
	Load1     float64 `json:"load1"`
	Load5     float64 `json:"load5"`
	Load15    float64 `json:"load15"`
	MemUsed   int64   `json:"memUsed"`
	MemTotal  int64   `json:"memTotal"`
	SwapUsed  int64   `json:"swapUsed"`
	SwapTotal int64   `json:"swapTotal"`
	NetRx     int64   `json:"netRx"`
	NetTx     int64   `json:"netTx"`
	CPUTemp   float64 `json:"cpuTemp"`
}

type MountPoint struct {
	TS    int64  `json:"ts"`
	Mount string `json:"mount"`
	Used  int64  `json:"used"`
	Total int64  `json:"total"`
}

const rawWindow = 24 * time.Hour

func bucketSeconds(from, to time.Time, maxPoints int, minimum int64) int64 {
	span := to.Sub(from).Seconds()
	if maxPoints <= 0 {
		maxPoints = 400
	}
	b := int64(span / float64(maxPoints))
	if b < minimum {
		b = minimum
	}
	return b
}

func (s *Store) ContainerSeries(ctx context.Context, id string, from, to time.Time, maxPoints int) ([]Point, error) {
	useRaw := time.Since(from) <= rawWindow
	var query string
	var bucket int64
	if useRaw {
		bucket = bucketSeconds(from, to, maxPoints, 15)
		query = `SELECT (ts / ?) * ? AS b, avg(cpu_pct), max(cpu_pct), avg(mem_bytes), max(mem_bytes), max(mem_limit),
			avg(net_rx), avg(net_tx), avg(blk_read), avg(blk_write)
			FROM samples_raw WHERE container_id = ? AND ts >= ? AND ts < ? GROUP BY b ORDER BY b`
	} else {
		bucket = bucketSeconds(from, to, maxPoints, 300)
		query = `SELECT (ts / ?) * ? AS b, avg(cpu_avg), max(cpu_max), avg(mem_avg), max(mem_max), max(mem_limit),
			avg(net_rx_avg), avg(net_tx_avg), avg(blk_read_avg), avg(blk_write_avg)
			FROM samples_5m WHERE container_id = ? AND ts >= ? AND ts < ? GROUP BY b ORDER BY b`
	}
	rows, err := s.db.QueryContext(ctx, query, bucket, bucket, id, unix(from), unix(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Point
	for rows.Next() {
		var p Point
		var mem, memMax, memLimit, rx, tx, br, bw float64
		if err := rows.Scan(&p.TS, &p.CPU, &p.CPUMax, &mem, &memMax, &memLimit, &rx, &tx, &br, &bw); err != nil {
			return nil, err
		}
		p.Mem, p.MemMax, p.MemLimit = int64(mem), int64(memMax), int64(memLimit)
		p.NetRx, p.NetTx, p.BlkRead, p.BlkWrite = int64(rx), int64(tx), int64(br), int64(bw)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) HostSeries(ctx context.Context, from, to time.Time, maxPoints int) ([]HostPoint, error) {
	useRaw := time.Since(from) <= rawWindow
	var query string
	var bucket int64
	if useRaw {
		bucket = bucketSeconds(from, to, maxPoints, 60)
		query = `SELECT (ts / ?) * ? AS b, avg(cpu_pct), max(cpu_pct), avg(load1), avg(load5), avg(load15),
			avg(mem_used), max(mem_total), avg(swap_used), max(swap_total), avg(net_rx), avg(net_tx), avg(cpu_temp)
			FROM host_samples_raw WHERE ts >= ? AND ts < ? GROUP BY b ORDER BY b`
	} else {
		bucket = bucketSeconds(from, to, maxPoints, 300)
		query = `SELECT (ts / ?) * ? AS b, avg(cpu_avg), max(cpu_max), avg(load1), avg(load5), avg(load15),
			avg(mem_avg), max(mem_total), avg(swap_avg), max(swap_total), avg(net_rx_avg), avg(net_tx_avg), avg(cpu_temp_avg)
			FROM host_samples_5m WHERE ts >= ? AND ts < ? GROUP BY b ORDER BY b`
	}
	rows, err := s.db.QueryContext(ctx, query, bucket, bucket, unix(from), unix(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HostPoint
	for rows.Next() {
		var p HostPoint
		var memUsed, memTotal, swapUsed, swapTotal, rx, tx float64
		if err := rows.Scan(&p.TS, &p.CPU, &p.CPUMax, &p.Load1, &p.Load5, &p.Load15,
			&memUsed, &memTotal, &swapUsed, &swapTotal, &rx, &tx, &p.CPUTemp); err != nil {
			return nil, err
		}
		p.MemUsed, p.MemTotal, p.SwapUsed, p.SwapTotal = int64(memUsed), int64(memTotal), int64(swapUsed), int64(swapTotal)
		p.NetRx, p.NetTx = int64(rx), int64(tx)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) MountSeries(ctx context.Context, from, to time.Time, maxPoints int) ([]MountPoint, error) {
	useRaw := time.Since(from) <= rawWindow
	var query string
	var bucket int64
	if useRaw {
		bucket = bucketSeconds(from, to, maxPoints, 60)
		query = `SELECT (ts / ?) * ? AS b, mount, avg(used), max(total)
			FROM mount_samples_raw WHERE ts >= ? AND ts < ? GROUP BY b, mount ORDER BY b`
	} else {
		bucket = bucketSeconds(from, to, maxPoints, 300)
		query = `SELECT (ts / ?) * ? AS b, mount, avg(used_avg), max(total)
			FROM mount_samples_5m WHERE ts >= ? AND ts < ? GROUP BY b, mount ORDER BY b`
	}
	rows, err := s.db.QueryContext(ctx, query, bucket, bucket, unix(from), unix(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MountPoint
	for rows.Next() {
		var p MountPoint
		var used, total float64
		if err := rows.Scan(&p.TS, &p.Mount, &used, &total); err != nil {
			return nil, err
		}
		p.Used, p.Total = int64(used), int64(total)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) RecentSparkline(ctx context.Context, id string, since time.Time, maxPoints int) ([]float64, error) {
	bucket := bucketSeconds(since, time.Now(), maxPoints, 15)
	rows, err := s.db.QueryContext(ctx, `SELECT avg(cpu_pct) FROM samples_raw
		WHERE container_id = ? AND ts >= ? GROUP BY ts / ? ORDER BY ts / ?`, id, unix(since), bucket, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
