# Homelab Dashboard

Self-hosted monitoring for a TerraMaster NAS running Docker under Portainer. One Go binary with the React UI embedded, SQLite on disk, a read-only Docker socket proxy in front of the daemon, and an optional privileged SMART sidecar. Built for a TerraMaster F6-424 on TOS 6 with Portainer CE 2.24, and generic enough for any x86_64 Linux Docker host.

What it shows:

- Host CPU, memory, swap, load, network, uptime, and CPU temperature from the NAS `/proc` and `/sys`.
- Every container grouped into stacks. Portainer stacks first, then the compose project label, then an "unmanaged" group.
- Live per-container CPU, memory, network, block I/O, restarts, health, image tag, and an "update available" badge.
- Per-container history charts at 1h, 6h, 24h, 7d, and 30d, restart history, health check output, masked environment, mounts, ports, and a log viewer with tail-follow, level filter, and regex search.
- An error feed built from log pattern matches, non-zero exits, OOM kills, and failing health checks.
- Disk cards with SMART attributes and temperature trend, mdadm array state, and used/total bars per mount.
- Alerts with hysteresis and debounce, delivered to Telegram, acknowledged in the UI.
- Image update checks against Docker Hub, GHCR, and any registry that speaks the v2 API.

Nothing in v1 writes to containers. The socket proxy blocks every POST.

## Prerequisites

- TerraMaster TOS 6 or any Linux host with Docker Engine 24 or newer and Docker Compose v2.
- Portainer CE or BE 2.19 or newer, reachable from inside a container. On TOS 6 the bundled Portainer listens directly on port 19000.
- Nginx Proxy Manager or another reverse proxy that terminates HTTPS. The dashboard sets `Secure` cookies by default, so plain HTTP on a LAN needs `SECURE_COOKIES=false`.
- For the SMART panel: `smartctl` must be able to see the drives from a privileged container. Test with:

```bash
docker run --rm --privileged -v /dev:/dev alpine sh -c "apk add -q smartmontools && smartctl -a /dev/sda" | head -20
```

## Mint a Portainer API key

1. Sign in to Portainer.
2. Click your username in the top right, then **My account**.
3. Under **Access tokens**, click **Add access token**, give it a description such as `homelab-dashboard`, and confirm your password.
4. Copy the token. It is shown once. It starts with `ptr_`.

The token inherits your user's permissions. The dashboard only issues GET requests to `/api/system/status`, `/api/endpoints`, and `/api/stacks`.

## Install

```bash
mkdir -p /Volume1/DockerAppsData/homelab-dashboard
chown 65532:65532 /Volume1/DockerAppsData/homelab-dashboard
git clone https://github.com/iModyHK/homelab-dashboard.git /Volume1/DockerAppsData/homelab-dashboard-src
cd /Volume1/DockerAppsData/homelab-dashboard-src/deploy
cp .env.example .env
```

Edit `.env`. The three required values are `PORTAINER_URL`, `PORTAINER_API_KEY`, and `ADMIN_PASSWORD`. Everything else has a documented default.

Then start it. With the SMART sidecar:

```bash
docker compose --profile smart up -d
```

Without SMART:

```bash
docker compose up -d
```

The compose file pulls `ghcr.io/imodyhk/homelab-dashboard` and `ghcr.io/imodyhk/homelab-dashboard-smart`, which GitHub Actions builds from this repository on every push to `main`. To build on the NAS instead, add `--build`:

```bash
docker compose --profile smart up -d --build
```

Check that all services are healthy:

```bash
docker compose ps
```

Open `http://<nas-ip>:8087` and sign in with `ADMIN_PASSWORD`. First paint arrives from a single `/api/overview` call and the WebSocket takes over from there.

### Installing through Portainer instead of the CLI

Stacks, Add stack, Repository. Repository URL `https://github.com/iModyHK/homelab-dashboard`, compose path `deploy/docker-compose.yml`. Paste the contents of `.env.example` into the environment variables editor and fill in the required values. Enable the `smart` profile by adding `COMPOSE_PROFILES=smart` as an environment variable.

## Nginx Proxy Manager

Add a proxy host:

| Field | Value |
|---|---|
| Domain | `dash.example.com` |
| Scheme | `http` |
| Forward hostname | the NAS LAN IP, for example `192.168.0.17` |
| Forward port | `8087` or whatever `DASHBOARD_PORT` is set to |
| Websockets support | on |
| Block common exploits | on |
| SSL | your certificate, Force SSL on, HTTP/2 on |

Websockets support is required for live updates. Without it the UI still works but polls the overview every time you navigate.

If NPM runs in a container on the same NAS and you want to keep the dashboard off the LAN entirely, set `DASHBOARD_BIND=127.0.0.1` in `.env` and point NPM at `host.docker.internal:8087`, or attach NPM to the `homelab-dashboard_egress` network and forward to `dashboard:8080`.

## What runs and with what access

| Service | Image | Privileges | Purpose |
|---|---|---|---|
| `socket-proxy` | `tecnativa/docker-socket-proxy` | Docker socket read-only. Only `CONTAINERS`, `IMAGES`, `INFO`, `VERSION`, `EVENTS`, `PING` are enabled. `POST` is off. | The only path from the dashboard to the Docker API |
| `dashboard` | `ghcr.io/imodyhk/homelab-dashboard` | Non-root UID 65532, read-only root filesystem, all capabilities dropped. Read-only mounts of `/proc`, `/sys`, and `/usr` | Collector, API, UI |
| `smart` | `ghcr.io/imodyhk/homelab-dashboard-smart` | Privileged, `/dev` read-only, attached to an internal network only, no published ports | Runs `smartctl` and serves JSON |

The dashboard never mounts the Docker socket. The host root filesystem is not mounted either. `/usr` is bound read-only only so that `statfs` can report the root filesystem's usage, and `/Volume1` usage comes from the data volume that already lives there. `TRACKED_MOUNTS` documents how to add more.

Secrets come from `.env` or from `*_FILE` variants pointing at Docker secrets. They are never logged. Portainer error messages have the query string and credentials stripped. Environment variables shown in the container detail page are masked when the key looks like a secret.

## Alerts

Rules evaluated after every stats and host cycle:

| Rule | Fires when | Severity |
|---|---|---|
| `container_down` | A container with `always` or `unless-stopped` restart policy is not running, or any container exited non-zero, held for `ALERT_DEBOUNCE` | critical |
| `restart_loop` | `ALERT_RESTART_COUNT` die or restart events inside `ALERT_RESTART_WINDOW` | critical |
| `health_failing` | Docker health check reports unhealthy, held for `ALERT_DEBOUNCE` | warning |
| `cpu_high` | Container CPU above `ALERT_CPU_PCT` of its allowance for `ALERT_SUSTAINED_FOR` | warning |
| `memory_high` | Container memory above `ALERT_MEM_PCT` of its limit for `ALERT_SUSTAINED_FOR` | warning |
| `host_cpu_high`, `host_memory_high` | Same thresholds applied to the NAS | warning |
| `disk_high` | A tracked mount above `ALERT_DISK_PCT` for `ALERT_SUSTAINED_FOR` | warning |
| `smart_failing` | SMART self-assessment failed, or any reallocated, pending, or uncorrectable sectors | critical |
| `disk_temp_high` | Drive at or above `ALERT_DISK_TEMP_C` for `ALERT_SUSTAINED_FOR` | warning |
| `raid_degraded` | An mdadm array is degraded, inactive, or rebuilding | critical |
| `host_unreachable` | Docker or Portainer has been failing for `ALERT_HOST_UNREACHABLE_FOR` | critical |
| `db_size` | The SQLite file is larger than `DB_MAX_MB` | warning |

Each rule fires once on the transition to firing and once on resolve. Percent thresholds resolve five points below the trigger, temperatures three degrees below, so a value hovering at the line does not flap. Firing alerts survive a restart without being re-sent.

### Telegram

1. Message `@BotFather`, send `/newbot`, follow the prompts, and copy the token.
2. Send your new bot any message so it can see your chat.
3. Fetch your chat ID:

```bash
curl -s "https://api.telegram.org/bot<TOKEN>/getUpdates" | jq '.result[0].message.chat.id'
```

4. Put both values in `.env` as `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`, then `docker compose up -d`.

Delivery retries five times with backoff up to five minutes and never blocks the collector. If both variables are empty, alerts still show in the UI and the banner, nothing is sent.

## Data and retention

Everything lives in one SQLite file at `DATA_PATH/dashboard.db` in WAL mode.

| Table | Contents | Retained |
|---|---|---|
| `samples_raw`, `host_samples_raw`, `mount_samples_raw` | Every poll | 24 hours |
| `samples_5m`, `host_samples_5m`, `mount_samples_5m` | Average and max per 5 minute bucket | `RETENTION_DAYS` |
| `events` | Container lifecycle events | `RETENTION_DAYS` |
| `log_errors` | Error feed | 7 days |
| `alerts` | Firing and resolved alerts | resolved ones pruned after `RETENTION_DAYS` |
| `disks`, `disk_temps`, `containers`, `image_updates` | Current state and temperature trend | `RETENTION_DAYS` for temperatures |

Network and block I/O columns store bytes per second, computed from the delta between consecutive polls, so averages in the rollup tables are meaningful.

Rollup and pruning run hourly in one transaction, followed by a WAL checkpoint. At the defaults with 23 containers the file grows about 8 MB per month.

### Backup

The database is a single file plus its WAL. Copy it while the dashboard runs using SQLite's own backup, which is consistent even mid-write:

```bash
docker run --rm -v /Volume1/DockerAppsData/homelab-dashboard:/data alpine sh -c "apk add -q sqlite && sqlite3 /data/dashboard.db '.backup /data/dashboard-backup.db'"
```

Then copy `dashboard-backup.db` wherever your backups go. The `session.secret` file in the same directory is optional to back up. Without it every browser has to sign in again after a restore.

### Restore

```bash
cd /Volume1/DockerAppsData/homelab-dashboard-src/deploy
docker compose stop dashboard
cp /path/to/dashboard-backup.db /Volume1/DockerAppsData/homelab-dashboard/dashboard.db
rm -f /Volume1/DockerAppsData/homelab-dashboard/dashboard.db-wal /Volume1/DockerAppsData/homelab-dashboard/dashboard.db-shm
chown 65532:65532 /Volume1/DockerAppsData/homelab-dashboard/dashboard.db
docker compose start dashboard
```

## Upgrade

```bash
cd /Volume1/DockerAppsData/homelab-dashboard-src
git pull
cd deploy
docker compose pull
docker compose --profile smart up -d
```

Schema migrations run automatically at startup and are forward-only. Take a backup before upgrading across a major version. To pin a version, set `DASHBOARD_TAG` in `.env` to a release tag.

## Configuration reference

Every variable is documented with its type and default in [`deploy/.env.example`](deploy/.env.example). A few that deserve a note:

- `TRACKED_MOUNTS` takes `containerPath=label` pairs. The container path is any directory that sits on the host filesystem you want measured. The default reads the root filesystem through `/hostfs/usr` and `/Volume1` through the data volume.
- `EXCLUDE_PATTERNS` takes shell globs against container names. The dashboard's own stack is always hidden.
- `LOG_ERROR_PATTERNS` is a comma separated list of Go regular expressions. The default matches error, exception, fatal, panic, and traceback case-insensitively.
- `PORTAINER_ENDPOINT_IDS` restricts monitoring to specific endpoints. Empty means every endpoint Portainer returns.
- `ADMIN_PASSWORD_HASH` lets you keep the plaintext password out of `.env`. Generate a hash with:

```bash
docker run --rm -e ADMIN_PASSWORD='your password' ghcr.io/imodyhk/homelab-dashboard hash-password
```

## Troubleshooting

### The dashboard container restarts and the log says `configuration:`

The config loader fails fast and prints every problem at once, for example `PORTAINER_API_KEY or PORTAINER_API_KEY_FILE is required`. Read the whole block with:

```bash
docker compose logs dashboard | head -20
```

Fix `.env` and `docker compose up -d` again. If it says `data dir: permission denied`, the `DATA_PATH` directory is not owned by `PUID:PGID`. Run the `chown` from the install section.

### Every source dot is red, or the header says "reconnecting" forever

The dashboard cannot reach the socket proxy, or the proxy cannot reach the socket. Check:

```bash
docker compose ps
docker compose logs socket-proxy | tail
docker compose exec socket-proxy wget -qO- http://127.0.0.1:2375/version | head -c 200
```

If the proxy is healthy but the dashboard log shows `docker /containers/json returned 403`, the proxy's environment lost one of the allow flags. Compare it with `deploy/docker-compose.yml`.

If only the `live` dot is amber, the WebSocket cannot connect. In NPM, confirm **Websockets support** is on for the proxy host. If you access the dashboard over plain HTTP, set `SECURE_COOKIES=false`, because browsers drop `Secure` cookies on HTTP and the WebSocket upgrade then arrives unauthenticated.

### The portainer dot is red and the log says `portainer rejected the API key`

The token was revoked, or the Portainer user it belongs to was deleted or demoted. Mint a new token and update `PORTAINER_API_KEY`. The dashboard re-reads `PORTAINER_API_KEY_FILE` automatically on a 401, so with a Docker secret you only need to update the file. With a plain variable, `docker compose up -d` to apply it.

If the dot is red with a connection error instead, the container cannot reach `PORTAINER_URL`. On TOS 6 the working value is `http://<nas-ip>:19000`, not the `:8181/portainer/` path the TOS UI opens. Verify from inside the container's network:

```bash
docker run --rm --network homelab-dashboard_egress curlimages/curl -s http://192.168.0.17:19000/api/system/status
```

### Host metrics show dashes and the disk panel is empty

The `/proc` and `/sys` bind mounts are missing or the compose file was edited. The dashboard log shows `host readers` with the exact path that failed. For the SMART cards, the `smart` profile must be enabled and the sidecar healthy:

```bash
docker compose --profile smart ps smart
docker compose --profile smart logs smart | tail
```

A drive in standby is reported as `standby` and keeps its last known attributes. The sidecar never wakes sleeping drives.

## Development

```bash
make web
make test
make lint
make dev
```

`make dev` runs Vite on port 5173 with `/api` proxied to the Go server on port 8080. Export the environment variables from `deploy/.env.example` first, and point `DOCKER_HOST` at a socket proxy such as:

```bash
docker run -d --name proxy -p 127.0.0.1:2375:2375 -e CONTAINERS=1 -e IMAGES=1 -e INFO=1 -e VERSION=1 -e EVENTS=1 -e POST=0 -v /var/run/docker.sock:/var/run/docker.sock:ro tecnativa/docker-socket-proxy:0.3.0
```

Dependencies beyond the standard library, chi, SQLite, and the frontend toolchain:

- `github.com/coder/websocket`. Replaces a hand-written RFC 6455 framing, masking, ping, and close-handshake implementation.
- `golang.org/x/crypto/argon2`. The standard library has no Argon2id.

Layout:

```
cmd/dashboard        main binary, hash-password and healthcheck subcommands
cmd/smart            SMART sidecar, wraps smartctl and serves JSON
internal/config      env loading and validation
internal/portainer   Portainer API client
internal/docker      Docker API client over the socket proxy, stats math, log demux
internal/host        /proc and /sys readers, statfs
internal/disks       smartctl JSON parser, /proc/mdstat parser
internal/registry    image reference parsing, digest lookups with bearer auth
internal/collector   scheduler, worker pool, event stream, in-memory state, message bus
internal/store       SQLite schema, migrations, series queries, rollup and pruning
internal/alerts      rule evaluation, hysteresis, debounce, Telegram delivery
internal/api         REST handlers, WebSocket hub, static file serving
internal/auth        argon2id, signed session cookies, CSRF, login rate limit
web                  Vite app, embedded into the binary from web/dist
deploy               docker-compose.yml and .env.example
```
