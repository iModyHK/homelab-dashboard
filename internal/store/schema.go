package store

var migrations = []string{
	`
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS containers (
	id            TEXT PRIMARY KEY,
	name          TEXT NOT NULL,
	image         TEXT NOT NULL,
	image_id      TEXT NOT NULL DEFAULT '',
	image_digest  TEXT NOT NULL DEFAULT '',
	stack         TEXT NOT NULL DEFAULT '',
	service       TEXT NOT NULL DEFAULT '',
	endpoint_id   INTEGER NOT NULL DEFAULT 0,
	state         TEXT NOT NULL,
	health        TEXT NOT NULL DEFAULT '',
	restart_count INTEGER NOT NULL DEFAULT 0,
	created       INTEGER NOT NULL,
	started_at    INTEGER NOT NULL DEFAULT 0,
	exit_code     INTEGER NOT NULL DEFAULT 0,
	first_seen    INTEGER NOT NULL,
	last_seen     INTEGER NOT NULL,
	present       INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_containers_stack ON containers(stack);

CREATE TABLE IF NOT EXISTS samples_raw (
	container_id TEXT    NOT NULL,
	ts           INTEGER NOT NULL,
	cpu_pct      REAL    NOT NULL,
	mem_bytes    INTEGER NOT NULL,
	mem_limit    INTEGER NOT NULL,
	net_rx       INTEGER NOT NULL,
	net_tx       INTEGER NOT NULL,
	blk_read     INTEGER NOT NULL,
	blk_write    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_samples_raw_cid_ts ON samples_raw(container_id, ts);
CREATE INDEX IF NOT EXISTS idx_samples_raw_ts ON samples_raw(ts);

CREATE TABLE IF NOT EXISTS samples_5m (
	container_id  TEXT    NOT NULL,
	ts            INTEGER NOT NULL,
	cpu_avg       REAL    NOT NULL,
	cpu_max       REAL    NOT NULL,
	mem_avg       INTEGER NOT NULL,
	mem_max       INTEGER NOT NULL,
	mem_limit     INTEGER NOT NULL,
	net_rx_avg    INTEGER NOT NULL,
	net_rx_max    INTEGER NOT NULL,
	net_tx_avg    INTEGER NOT NULL,
	net_tx_max    INTEGER NOT NULL,
	blk_read_avg  INTEGER NOT NULL,
	blk_read_max  INTEGER NOT NULL,
	blk_write_avg INTEGER NOT NULL,
	blk_write_max INTEGER NOT NULL,
	samples       INTEGER NOT NULL,
	PRIMARY KEY (container_id, ts)
);
CREATE INDEX IF NOT EXISTS idx_samples_5m_ts ON samples_5m(ts);

CREATE TABLE IF NOT EXISTS host_samples_raw (
	ts         INTEGER PRIMARY KEY,
	cpu_pct    REAL    NOT NULL,
	load1      REAL    NOT NULL,
	load5      REAL    NOT NULL,
	load15     REAL    NOT NULL,
	mem_used   INTEGER NOT NULL,
	mem_total  INTEGER NOT NULL,
	swap_used  INTEGER NOT NULL,
	swap_total INTEGER NOT NULL,
	net_rx     INTEGER NOT NULL,
	net_tx     INTEGER NOT NULL,
	cpu_temp   REAL    NOT NULL
);

CREATE TABLE IF NOT EXISTS host_samples_5m (
	ts           INTEGER PRIMARY KEY,
	cpu_avg      REAL    NOT NULL,
	cpu_max      REAL    NOT NULL,
	load1        REAL    NOT NULL,
	load5        REAL    NOT NULL,
	load15       REAL    NOT NULL,
	mem_avg      INTEGER NOT NULL,
	mem_max      INTEGER NOT NULL,
	mem_total    INTEGER NOT NULL,
	swap_avg     INTEGER NOT NULL,
	swap_total   INTEGER NOT NULL,
	net_rx_avg   INTEGER NOT NULL,
	net_rx_max   INTEGER NOT NULL,
	net_tx_avg   INTEGER NOT NULL,
	net_tx_max   INTEGER NOT NULL,
	cpu_temp_avg REAL    NOT NULL,
	cpu_temp_max REAL    NOT NULL,
	samples      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS mount_samples_raw (
	ts    INTEGER NOT NULL,
	mount TEXT    NOT NULL,
	used  INTEGER NOT NULL,
	total INTEGER NOT NULL,
	PRIMARY KEY (ts, mount)
);

CREATE TABLE IF NOT EXISTS mount_samples_5m (
	ts       INTEGER NOT NULL,
	mount    TEXT    NOT NULL,
	used_avg INTEGER NOT NULL,
	used_max INTEGER NOT NULL,
	total    INTEGER NOT NULL,
	PRIMARY KEY (ts, mount)
);

CREATE TABLE IF NOT EXISTS disks (
	device         TEXT PRIMARY KEY,
	model          TEXT NOT NULL,
	serial         TEXT NOT NULL,
	firmware       TEXT NOT NULL,
	capacity       INTEGER NOT NULL,
	rotation       INTEGER NOT NULL,
	transport      TEXT NOT NULL,
	temp           INTEGER NOT NULL,
	power_on_hours INTEGER NOT NULL,
	power_cycles   INTEGER NOT NULL,
	reallocated    INTEGER NOT NULL,
	pending        INTEGER NOT NULL,
	uncorrectable  INTEGER NOT NULL,
	crc_errors     INTEGER NOT NULL,
	smart_status   TEXT NOT NULL,
	standby        INTEGER NOT NULL,
	percent_used   INTEGER NOT NULL,
	last_seen      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS disk_temps (
	device TEXT    NOT NULL,
	ts     INTEGER NOT NULL,
	temp   INTEGER NOT NULL,
	PRIMARY KEY (device, ts)
);

CREATE TABLE IF NOT EXISTS events (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	ts             INTEGER NOT NULL,
	container_id   TEXT    NOT NULL,
	container_name TEXT    NOT NULL,
	type           TEXT    NOT NULL,
	detail         TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_cid_ts ON events(container_id, ts);

CREATE TABLE IF NOT EXISTS log_errors (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	ts             INTEGER NOT NULL,
	container_id   TEXT    NOT NULL,
	container_name TEXT    NOT NULL,
	kind           TEXT    NOT NULL,
	stream         TEXT    NOT NULL,
	line           TEXT    NOT NULL,
	UNIQUE (container_id, ts, line)
);
CREATE INDEX IF NOT EXISTS idx_log_errors_ts ON log_errors(ts);
CREATE INDEX IF NOT EXISTS idx_log_errors_cid_ts ON log_errors(container_id, ts);

CREATE TABLE IF NOT EXISTS alerts (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	rule        TEXT    NOT NULL,
	target      TEXT    NOT NULL,
	target_name TEXT    NOT NULL,
	severity    TEXT    NOT NULL,
	message     TEXT    NOT NULL,
	state       TEXT    NOT NULL,
	fired_at    INTEGER NOT NULL,
	resolved_at INTEGER NOT NULL DEFAULT 0,
	notified_at INTEGER NOT NULL DEFAULT 0,
	acked_at    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_alerts_state ON alerts(state);
CREATE INDEX IF NOT EXISTS idx_alerts_rule_target ON alerts(rule, target);

CREATE TABLE IF NOT EXISTS image_updates (
	image            TEXT PRIMARY KEY,
	local_digest     TEXT NOT NULL,
	remote_digest    TEXT NOT NULL,
	update_available INTEGER NOT NULL,
	checked_at       INTEGER NOT NULL,
	error            TEXT NOT NULL
);
`,
}
