# Installing Kronowatt

This is a living document — it reflects what is actually implemented and
deployable right now, and will be extended as each part of the system
(migrations, collectors, config) lands. Sections marked **TODO** describe
what the spec (`kronowatt_spec_v1.1.md`) calls for but that doesn't exist in
the codebase yet; don't follow them as if they were tested instructions.

Target environment throughout: a dedicated Debian 13 box (spec assumes a
Fujitsu Esprimo Q510, 30GB SSD) reachable from your dev machine over SSH and
on the LAN. "LAN only" means no inbound access from outside the LAN — the
box still needs outbound internet for FMI, the spot price provider, and the
Defa cloud API.

## 1. Prerequisites

**Dev machine** (where you build):
- Go (version matching `backend/go.mod`)
- Node.js + npm (for the frontend build)
- `rsync` and `ssh`

**Target box** (where it runs):
- Debian 13, minimal server install
- Docker Engine (for TimescaleDB only — see spec §9)
- A dedicated `kronowatt` system user/group
- SSH access for the dev machine to deploy over

## 2. Provision the target box

```
# on the target box
sudo adduser --system --group --home /opt/kronowatt kronowatt
sudo mkdir -p /opt/kronowatt/backend /opt/kronowatt/frontend
sudo chown -R kronowatt:kronowatt /opt/kronowatt

# install Docker Engine per https://docs.docker.com/engine/install/debian/
```

## 3. Database — TimescaleDB via Docker

Copy the whole `deployment/` directory to the box (e.g. into
`/opt/kronowatt/deployment/`) — `docker-compose.yml` builds the DB image
from `deployment/timescaledb/` rather than pulling a pre-tagged one, so that
directory needs to come along too. Then create the DB password secret the
compose file expects — this file is git-ignored and must never be
committed:

```
mkdir -p deployment/secrets
echo -n '<a-strong-password>' > deployment/secrets/db_password
```

Build and start it:

```
cd deployment
docker compose up -d --build
docker compose ps   # confirm kronowatt-timescaledb is healthy
```

The image is a thin layer on the official `timescale/timescaledb` image
(see `deployment/timescaledb/Dockerfile`) that just runs an init script to
create the `timescaledb` extension on first start — verify with:

```
docker exec kronowatt-timescaledb psql -U kronowatt -d kronowatt -c '\dx'
```

The container binds Postgres to `127.0.0.1:5432` only (not exposed on the
LAN) — the Go backend connects to it locally on the same box.

## 4. Database migrations

Migrations run from inside the server binary itself (via
[goose](https://github.com/pressly/goose), embedded — see `CLAUDE.md` for
why), so no separate migration tool needs to be installed on the target box.

```
KRONOWATT_DB_DSN='postgres://kronowatt:<password>@127.0.0.1:5432/kronowatt?sslmode=disable' \
  ./kronowatt-server migrate up
```

The full core schema exists (all spec §11 domain tables, hypertables,
compression/retention policies per §9a) — see `CLAUDE.md` for the table
list and design notes.

## 4a. Seeding test data (optional, dev/testing only)

Real collectors only exist for weather so far (spec §43 Steps 3/5/9 —
Cozify/spot-price/EV — aren't implemented). To get multi-year data into
every table for testing without those collectors, generate the fake-data
fixtures on the dev machine and seed them into the real database:

```
cd frontend && npm run generate:fake-data && cd ../backend
KRONOWATT_DB_DSN='postgres://kronowatt:<password>@127.0.0.1:5432/kronowatt?sslmode=disable' \
  go run ./cmd/server seed ../frontend/static/fake-data
```

Seeded rows are tagged (`source = "fake_seed"`, or `station_fmisid = "fake"`
for weather, which has no `source` column) so they never collide with real
collected data — see `CLAUDE.md` "Seeding fake data into the real database"
for the full mapping. Skip this entirely for a production install; it's
purely for exercising the API/frontend before real collectors exist for
everything.

## 5. Build and deploy the backend + frontend

The frontend fetches the backend API from `VITE_API_BASE_URL`, a Vite
build-time env var (default `http://localhost:8080` — only correct for
local dev). Set it to wherever the backend will actually be reachable
*before* building — adapter-static has no server to read env vars at
runtime, so this can't be fixed after the fact without rebuilding:

```
cd frontend && cp .env.example .env
# edit .env: VITE_API_BASE_URL=http://<box hostname or IP>:8080
cd ..
```

From the repo root, on the dev machine:

```
export KRONOWATT_HOST=<box hostname or IP>
export KRONOWATT_USER=kronowatt   # defaults to "kronowatt" if unset
./deployment/deploy.sh
```

This builds the backend (`GOOS=linux GOARCH=amd64`) and the frontend static
assets locally, rsyncs both to `/opt/kronowatt/` on the target box, and
restarts the systemd services (step 6 must have run at least once first).

## 6. Install the systemd services (first time only)

```
# on the target box
sudo cp deployment/kronowatt-backend.service deployment/kronowatt-frontend.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now kronowatt-backend kronowatt-frontend
```

`kronowatt-frontend.service` currently serves the built static assets with
`python3 -m http.server` as a placeholder — fine for LAN use, but expect
this to change (e.g. to Caddy/nginx, or the backend serving the frontend
directly) before this is considered final.

## 7. Verify

```
curl http://<box>:8080/health     # -> {"status":"ok","disk_used_pct":...,"collectors":[...]}
curl http://<box>:8081/           # frontend
```

## 8. Configuration

Env vars only (no config file) — see `CLAUDE.md` "Configuration" for the
full list and why. At minimum, the backend needs `KRONOWATT_DB_DSN` set
(also used for the systemd service via `/opt/kronowatt/backend/kronowatt.env`,
referenced by `EnvironmentFile=` in `kronowatt-backend.service`).

## 9. Collectors

- **FMI weather (observations)**: implemented and running continuously
  inside the server process (spec §43 Step 4, observations only — forecast
  collection is still TODO). No setup needed beyond outbound internet
  access; it's a public, unauthenticated API. Test manually with
  `KRONOWATT_DB_DSN=... ./kronowatt-server collect weather`.
- **Cozify, spot price, EV (Defa)** — **TODO** (spec §43 Steps 3, 5, 9).
  Cozify is blocked on hardware availability. Each will need its own setup
  notes here, including — per spec §7.4 — the manual re-auth flow for the
  Defa CloudCharge token, which must never be committed to the repo or
  stored in a raw payload.

## 10. Backups — **TODO**

Spec §40 requires off-box-only backups (`pg_dump`/`pg_basebackup` pushed to
the dev rig, then synced offsite) with no long-term retention on the target
box itself. Document the actual cron/script and the restore procedure here
once set up — and verify a restore on the dev rig, not just the target box,
before considering this done.
