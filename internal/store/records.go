package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ContainerRecord struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Image        string `json:"image"`
	ImageID      string `json:"imageId"`
	ImageDigest  string `json:"imageDigest"`
	Stack        string `json:"stack"`
	Service      string `json:"service"`
	EndpointID   int    `json:"endpointId"`
	State        string `json:"state"`
	Health       string `json:"health"`
	RestartCount int    `json:"restartCount"`
	Created      int64  `json:"created"`
	StartedAt    int64  `json:"startedAt"`
	ExitCode     int    `json:"exitCode"`
	FirstSeen    int64  `json:"firstSeen"`
	LastSeen     int64  `json:"lastSeen"`
	Present      bool   `json:"present"`
}

func (s *Store) UpsertContainers(ctx context.Context, now time.Time, records []ContainerRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := unix(now)
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO containers
		(id, name, image, image_id, image_digest, stack, service, endpoint_id, state, health, restart_count,
		 created, started_at, exit_code, first_seen, last_seen, present)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, image = excluded.image, image_id = excluded.image_id,
			image_digest = excluded.image_digest, stack = excluded.stack, service = excluded.service,
			endpoint_id = excluded.endpoint_id, state = excluded.state, health = excluded.health,
			restart_count = excluded.restart_count, started_at = excluded.started_at,
			exit_code = excluded.exit_code, last_seen = excluded.last_seen, present = 1`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
		if _, err := stmt.ExecContext(ctx, r.ID, r.Name, r.Image, r.ImageID, r.ImageDigest, r.Stack, r.Service,
			r.EndpointID, r.State, r.Health, r.RestartCount, r.Created, r.StartedAt, r.ExitCode, ts, ts); err != nil {
			return fmt.Errorf("upsert container %s: %w", r.Name, err)
		}
	}
	if len(ids) == 0 {
		if _, err := tx.ExecContext(ctx, "UPDATE containers SET present = 0 WHERE present = 1"); err != nil {
			return err
		}
	} else {
		placeholders := strings.Repeat("?,", len(ids))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		if _, err := tx.ExecContext(ctx, "UPDATE containers SET present = 0 WHERE present = 1 AND id NOT IN ("+placeholders+")", args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Container(ctx context.Context, id string) (ContainerRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, image, image_id, image_digest, stack, service, endpoint_id, state, health,
		restart_count, created, started_at, exit_code, first_seen, last_seen, present FROM containers WHERE id = ? OR id LIKE ?`, id, id+"%")
	return scanContainer(row)
}

func (s *Store) Containers(ctx context.Context, includeGone bool) ([]ContainerRecord, error) {
	q := `SELECT id, name, image, image_id, image_digest, stack, service, endpoint_id, state, health,
		restart_count, created, started_at, exit_code, first_seen, last_seen, present FROM containers`
	if !includeGone {
		q += " WHERE present = 1"
	}
	q += " ORDER BY stack, name"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContainerRecord
	for rows.Next() {
		r, err := scanContainer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanContainer(sc scanner) (ContainerRecord, error) {
	var r ContainerRecord
	var present int
	err := sc.Scan(&r.ID, &r.Name, &r.Image, &r.ImageID, &r.ImageDigest, &r.Stack, &r.Service, &r.EndpointID, &r.State, &r.Health,
		&r.RestartCount, &r.Created, &r.StartedAt, &r.ExitCode, &r.FirstSeen, &r.LastSeen, &present)
	r.Present = present == 1
	return r, err
}

type EventRecord struct {
	ID            int64  `json:"id"`
	TS            int64  `json:"ts"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	Type          string `json:"type"`
	Detail        string `json:"detail"`
}

func (s *Store) InsertEvent(ctx context.Context, e EventRecord) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (ts, container_id, container_name, type, detail) VALUES (?, ?, ?, ?, ?)`,
		e.TS, e.ContainerID, e.ContainerName, e.Type, e.Detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Events(ctx context.Context, since time.Time, containerID string, limit int) ([]EventRecord, error) {
	q := `SELECT id, ts, container_id, container_name, type, detail FROM events WHERE ts >= ?`
	args := []any{unix(since)}
	if containerID != "" {
		q += " AND container_id = ?"
		args = append(args, containerID)
	}
	q += " ORDER BY ts DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRecord
	for rows.Next() {
		var e EventRecord
		if err := rows.Scan(&e.ID, &e.TS, &e.ContainerID, &e.ContainerName, &e.Type, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type RestartWindowCount struct {
	ContainerID string
	Count       int
}

func (s *Store) RestartCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT container_id, count(*) FROM events
		WHERE ts >= ? AND type IN ('restart', 'die', 'oom') GROUP BY container_id`, unix(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

type LogErrorRecord struct {
	ID            int64  `json:"id"`
	TS            int64  `json:"ts"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	Kind          string `json:"kind"`
	Stream        string `json:"stream"`
	Line          string `json:"line"`
}

func (s *Store) InsertLogErrors(ctx context.Context, records []LogErrorRecord) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO log_errors (ts, container_id, container_name, kind, stream, line)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	var inserted int64
	for _, r := range records {
		res, err := stmt.ExecContext(ctx, r.TS, r.ContainerID, r.ContainerName, r.Kind, r.Stream, r.Line)
		if err != nil {
			return inserted, err
		}
		n, _ := res.RowsAffected()
		inserted += n
	}
	return inserted, tx.Commit()
}

func (s *Store) LogErrors(ctx context.Context, since time.Time, containerID string, limit int) ([]LogErrorRecord, error) {
	q := `SELECT id, ts, container_id, container_name, kind, stream, line FROM log_errors WHERE ts >= ?`
	args := []any{unix(since)}
	if containerID != "" {
		q += " AND container_id = ?"
		args = append(args, containerID)
	}
	q += " ORDER BY ts DESC, id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogErrorRecord
	for rows.Next() {
		var r LogErrorRecord
		if err := rows.Scan(&r.ID, &r.TS, &r.ContainerID, &r.ContainerName, &r.Kind, &r.Stream, &r.Line); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) LastLogScan(ctx context.Context, containerID string) (time.Time, error) {
	var ts sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT max(ts) FROM log_errors WHERE container_id = ?", containerID).Scan(&ts)
	if err != nil || !ts.Valid {
		return time.Time{}, err
	}
	return time.Unix(ts.Int64, 0), nil
}

type DiskRecord struct {
	Device        string `json:"device"`
	Model         string `json:"model"`
	Serial        string `json:"serial"`
	Firmware      string `json:"firmware"`
	Capacity      int64  `json:"capacity"`
	Rotation      int    `json:"rotation"`
	Transport     string `json:"transport"`
	Temp          int    `json:"temp"`
	PowerOnHours  int64  `json:"powerOnHours"`
	PowerCycles   int64  `json:"powerCycles"`
	Reallocated   int64  `json:"reallocated"`
	Pending       int64  `json:"pending"`
	Uncorrectable int64  `json:"uncorrectable"`
	CRCErrors     int64  `json:"crcErrors"`
	SmartStatus   string `json:"smartStatus"`
	Standby       bool   `json:"standby"`
	PercentUsed   int    `json:"percentUsed"`
	LastSeen      int64  `json:"lastSeen"`
}

func (s *Store) UpsertDisks(ctx context.Context, now time.Time, disks []DiskRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := unix(now)
	for _, d := range disks {
		standby := 0
		if d.Standby {
			standby = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO disks
			(device, model, serial, firmware, capacity, rotation, transport, temp, power_on_hours, power_cycles,
			 reallocated, pending, uncorrectable, crc_errors, smart_status, standby, percent_used, last_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(device) DO UPDATE SET
				model = CASE WHEN excluded.model = '' THEN disks.model ELSE excluded.model END,
				serial = CASE WHEN excluded.serial = '' THEN disks.serial ELSE excluded.serial END,
				firmware = CASE WHEN excluded.firmware = '' THEN disks.firmware ELSE excluded.firmware END,
				capacity = CASE WHEN excluded.capacity = 0 THEN disks.capacity ELSE excluded.capacity END,
				rotation = CASE WHEN excluded.rotation = 0 THEN disks.rotation ELSE excluded.rotation END,
				transport = CASE WHEN excluded.transport = '' THEN disks.transport ELSE excluded.transport END,
				temp = CASE WHEN excluded.standby = 1 THEN disks.temp ELSE excluded.temp END,
				power_on_hours = CASE WHEN excluded.standby = 1 THEN disks.power_on_hours ELSE excluded.power_on_hours END,
				power_cycles = CASE WHEN excluded.standby = 1 THEN disks.power_cycles ELSE excluded.power_cycles END,
				reallocated = CASE WHEN excluded.standby = 1 THEN disks.reallocated ELSE excluded.reallocated END,
				pending = CASE WHEN excluded.standby = 1 THEN disks.pending ELSE excluded.pending END,
				uncorrectable = CASE WHEN excluded.standby = 1 THEN disks.uncorrectable ELSE excluded.uncorrectable END,
				crc_errors = CASE WHEN excluded.standby = 1 THEN disks.crc_errors ELSE excluded.crc_errors END,
				smart_status = CASE WHEN excluded.standby = 1 THEN disks.smart_status ELSE excluded.smart_status END,
				standby = excluded.standby,
				percent_used = CASE WHEN excluded.standby = 1 THEN disks.percent_used ELSE excluded.percent_used END,
				last_seen = excluded.last_seen`,
			d.Device, d.Model, d.Serial, d.Firmware, d.Capacity, d.Rotation, d.Transport, d.Temp, d.PowerOnHours,
			d.PowerCycles, d.Reallocated, d.Pending, d.Uncorrectable, d.CRCErrors, d.SmartStatus, standby, d.PercentUsed, ts); err != nil {
			return fmt.Errorf("upsert disk %s: %w", d.Device, err)
		}
		if !d.Standby && d.Temp > 0 {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO disk_temps (device, ts, temp) VALUES (?, ?, ?)`,
				d.Device, ts, d.Temp); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) Disks(ctx context.Context) ([]DiskRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device, model, serial, firmware, capacity, rotation, transport, temp,
		power_on_hours, power_cycles, reallocated, pending, uncorrectable, crc_errors, smart_status, standby, percent_used, last_seen
		FROM disks ORDER BY device`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DiskRecord
	for rows.Next() {
		var d DiskRecord
		var standby int
		if err := rows.Scan(&d.Device, &d.Model, &d.Serial, &d.Firmware, &d.Capacity, &d.Rotation, &d.Transport, &d.Temp,
			&d.PowerOnHours, &d.PowerCycles, &d.Reallocated, &d.Pending, &d.Uncorrectable, &d.CRCErrors, &d.SmartStatus,
			&standby, &d.PercentUsed, &d.LastSeen); err != nil {
			return nil, err
		}
		d.Standby = standby == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

type TempPoint struct {
	TS   int64 `json:"ts"`
	Temp int   `json:"temp"`
}

func (s *Store) DiskTemps(ctx context.Context, since time.Time, maxPoints int) (map[string][]TempPoint, error) {
	bucket := bucketSeconds(since, time.Now(), maxPoints, 60)
	rows, err := s.db.QueryContext(ctx, `SELECT device, (ts / ?) * ?, avg(temp) FROM disk_temps WHERE ts >= ?
		GROUP BY device, (ts / ?) * ? ORDER BY device, 2`, bucket, bucket, unix(since), bucket, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]TempPoint{}
	for rows.Next() {
		var device string
		var p TempPoint
		var temp float64
		if err := rows.Scan(&device, &p.TS, &temp); err != nil {
			return nil, err
		}
		p.Temp = int(temp + 0.5)
		out[device] = append(out[device], p)
	}
	return out, rows.Err()
}

type AlertRecord struct {
	ID         int64  `json:"id"`
	Rule       string `json:"rule"`
	Target     string `json:"target"`
	TargetName string `json:"targetName"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	State      string `json:"state"`
	FiredAt    int64  `json:"firedAt"`
	ResolvedAt int64  `json:"resolvedAt"`
	NotifiedAt int64  `json:"notifiedAt"`
	AckedAt    int64  `json:"ackedAt"`
}

func (s *Store) OpenAlert(ctx context.Context, a AlertRecord) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO alerts (rule, target, target_name, severity, message, state, fired_at)
		VALUES (?, ?, ?, ?, ?, 'firing', ?)`, a.Rule, a.Target, a.TargetName, a.Severity, a.Message, a.FiredAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ResolveAlert(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET state = 'resolved', resolved_at = ? WHERE id = ? AND state = 'firing'`, unix(at), id)
	return err
}

func (s *Store) MarkAlertNotified(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET notified_at = ? WHERE id = ?`, unix(at), id)
	return err
}

func (s *Store) AckAlert(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE alerts SET acked_at = ? WHERE id = ? AND acked_at = 0`, unix(at), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) FiringAlerts(ctx context.Context) ([]AlertRecord, error) {
	return s.queryAlerts(ctx, `SELECT id, rule, target, target_name, severity, message, state, fired_at, resolved_at, notified_at, acked_at
		FROM alerts WHERE state = 'firing' ORDER BY fired_at DESC`)
}

func (s *Store) Alerts(ctx context.Context, limit int) ([]AlertRecord, error) {
	return s.queryAlerts(ctx, `SELECT id, rule, target, target_name, severity, message, state, fired_at, resolved_at, notified_at, acked_at
		FROM alerts ORDER BY CASE state WHEN 'firing' THEN 0 ELSE 1 END, fired_at DESC LIMIT ?`, limit)
}

func (s *Store) queryAlerts(ctx context.Context, q string, args ...any) ([]AlertRecord, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlertRecord
	for rows.Next() {
		var a AlertRecord
		if err := rows.Scan(&a.ID, &a.Rule, &a.Target, &a.TargetName, &a.Severity, &a.Message, &a.State,
			&a.FiredAt, &a.ResolvedAt, &a.NotifiedAt, &a.AckedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type ImageUpdateRecord struct {
	Image           string `json:"image"`
	LocalDigest     string `json:"localDigest"`
	RemoteDigest    string `json:"remoteDigest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	CheckedAt       int64  `json:"checkedAt"`
	Error           string `json:"error"`
}

func (s *Store) UpsertImageUpdates(ctx context.Context, records []ImageUpdateRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range records {
		avail := 0
		if r.UpdateAvailable {
			avail = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO image_updates (image, local_digest, remote_digest, update_available, checked_at, error)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(image) DO UPDATE SET local_digest = excluded.local_digest, remote_digest = excluded.remote_digest,
				update_available = excluded.update_available, checked_at = excluded.checked_at, error = excluded.error`,
			r.Image, r.LocalDigest, r.RemoteDigest, avail, r.CheckedAt, r.Error); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ImageUpdates(ctx context.Context) ([]ImageUpdateRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT image, local_digest, remote_digest, update_available, checked_at, error FROM image_updates ORDER BY image`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImageUpdateRecord
	for rows.Next() {
		var r ImageUpdateRecord
		var avail int
		if err := rows.Scan(&r.Image, &r.LocalDigest, &r.RemoteDigest, &avail, &r.CheckedAt, &r.Error); err != nil {
			return nil, err
		}
		r.UpdateAvailable = avail == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) PruneImageUpdates(ctx context.Context, keep []string) error {
	if len(keep) == 0 {
		_, err := s.db.ExecContext(ctx, "DELETE FROM image_updates")
		return err
	}
	placeholders := strings.Repeat("?,", len(keep))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(keep))
	for i, k := range keep {
		args[i] = k
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM image_updates WHERE image NOT IN ("+placeholders+")", args...)
	return err
}
