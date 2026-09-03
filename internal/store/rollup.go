package store

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const (
	bucketSize        = 5 * time.Minute
	rollupWatermark   = "rollup_watermark"
	logErrorRetention = 7 * 24 * time.Hour
)

type RollupResult struct {
	Watermark      time.Time
	ContainerRows  int64
	HostRows       int64
	MountRows      int64
	PrunedRaw      int64
	PrunedRollups  int64
	PrunedEvents   int64
	PrunedLogLines int64
}

func (s *Store) Rollup(ctx context.Context, now time.Time, retention time.Duration) (RollupResult, error) {
	var res RollupResult
	cutoff := now.Truncate(bucketSize)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	var watermark int64
	var raw string
	if err := tx.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = ?", rollupWatermark).Scan(&raw); err == nil {
		watermark, _ = strconv.ParseInt(raw, 10, 64)
	}
	from := watermark
	to := unix(cutoff)
	if to > from {
		r, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO samples_5m
			(container_id, ts, cpu_avg, cpu_max, mem_avg, mem_max, mem_limit,
			 net_rx_avg, net_rx_max, net_tx_avg, net_tx_max, blk_read_avg, blk_read_max, blk_write_avg, blk_write_max, samples)
			SELECT container_id, (ts / 300) * 300,
				avg(cpu_pct), max(cpu_pct), avg(mem_bytes), max(mem_bytes), max(mem_limit),
				avg(net_rx), max(net_rx), avg(net_tx), max(net_tx),
				avg(blk_read), max(blk_read), avg(blk_write), max(blk_write), count(*)
			FROM samples_raw WHERE ts >= ? AND ts < ?
			GROUP BY container_id, (ts / 300) * 300`, from, to)
		if err != nil {
			return res, fmt.Errorf("rollup containers: %w", err)
		}
		res.ContainerRows, _ = r.RowsAffected()

		r, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO host_samples_5m
			(ts, cpu_avg, cpu_max, load1, load5, load15, mem_avg, mem_max, mem_total, swap_avg, swap_total,
			 net_rx_avg, net_rx_max, net_tx_avg, net_tx_max, cpu_temp_avg, cpu_temp_max, samples)
			SELECT (ts / 300) * 300,
				avg(cpu_pct), max(cpu_pct), avg(load1), avg(load5), avg(load15),
				avg(mem_used), max(mem_used), max(mem_total), avg(swap_used), max(swap_total),
				avg(net_rx), max(net_rx), avg(net_tx), max(net_tx), avg(cpu_temp), max(cpu_temp), count(*)
			FROM host_samples_raw WHERE ts >= ? AND ts < ?
			GROUP BY (ts / 300) * 300`, from, to)
		if err != nil {
			return res, fmt.Errorf("rollup host: %w", err)
		}
		res.HostRows, _ = r.RowsAffected()

		r, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO mount_samples_5m (ts, mount, used_avg, used_max, total)
			SELECT (ts / 300) * 300, mount, avg(used), max(used), max(total)
			FROM mount_samples_raw WHERE ts >= ? AND ts < ?
			GROUP BY (ts / 300) * 300, mount`, from, to)
		if err != nil {
			return res, fmt.Errorf("rollup mounts: %w", err)
		}
		res.MountRows, _ = r.RowsAffected()

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
			rollupWatermark, strconv.FormatInt(to, 10)); err != nil {
			return res, err
		}
	}
	res.Watermark = cutoff

	rawCutoff := unix(now.Add(-rawWindow))
	if rawCutoff > to {
		rawCutoff = to
	}
	for _, table := range []string{"samples_raw", "host_samples_raw", "mount_samples_raw"} {
		r, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE ts < ?", rawCutoff)
		if err != nil {
			return res, fmt.Errorf("prune %s: %w", table, err)
		}
		n, _ := r.RowsAffected()
		res.PrunedRaw += n
	}

	retentionCutoff := unix(now.Add(-retention))
	for _, table := range []string{"samples_5m", "host_samples_5m", "mount_samples_5m", "disk_temps"} {
		r, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE ts < ?", retentionCutoff)
		if err != nil {
			return res, fmt.Errorf("prune %s: %w", table, err)
		}
		n, _ := r.RowsAffected()
		res.PrunedRollups += n
	}

	r, err := tx.ExecContext(ctx, "DELETE FROM events WHERE ts < ?", retentionCutoff)
	if err != nil {
		return res, fmt.Errorf("prune events: %w", err)
	}
	res.PrunedEvents, _ = r.RowsAffected()

	r, err = tx.ExecContext(ctx, "DELETE FROM log_errors WHERE ts < ?", unix(now.Add(-logErrorRetention)))
	if err != nil {
		return res, fmt.Errorf("prune log errors: %w", err)
	}
	res.PrunedLogLines, _ = r.RowsAffected()

	if _, err := tx.ExecContext(ctx, "DELETE FROM alerts WHERE state = 'resolved' AND resolved_at < ?", retentionCutoff); err != nil {
		return res, fmt.Errorf("prune alerts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM containers WHERE present = 0 AND last_seen < ?", retentionCutoff); err != nil {
		return res, fmt.Errorf("prune containers: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return res, err
	}
	return res, nil
}
