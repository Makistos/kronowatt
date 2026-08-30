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

Copy `deployment/docker-compose.yml` to the box (e.g. into
`/opt/kronowatt/deployment/`), then create the DB password secret it
expects — this file is git-ignored and must never be committed:

```
mkdir -p deployment/secrets
echo -n '<a-strong-password>' > deployment/secrets/db_password
```

Start it:

```
cd deployment
docker compose up -d
docker compose ps   # confirm kronowatt-timescaledb is healthy
```

The container binds Postgres to `127.0.0.1:5432` only (not exposed on the
LAN) — the Go backend connects to it locally on the same box.

## 4. Database migrations — **TODO**

Not implemented yet (spec §43 Step 2). Once a migration framework is chosen
and `backend/migrations/` has real migrations, this section will document
the exact command to run them, including confirming the TimescaleDB
compression policy is applied (spec §9a) as part of the first migration.

## 5. Build and deploy the backend + frontend

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
curl http://<box>:8080/health     # backend -> {"status":"ok"}
curl http://<box>:8081/           # frontend
```

## 8. Configuration — **TODO**

`backend/internal/config` is currently an empty package. Once it's
implemented, this section will document the config file / environment
variables (Cozify host, FMI coordinates, spot price provider, Defa
credentials, DB connection string, disk-usage alert threshold, etc.).

## 9. Collectors — **TODO**

Cozify, FMI, spot price, and EV (Defa) collectors are not implemented yet
(spec §43 Steps 3, 4, 5, 9). Each will need its own setup notes here,
including — per spec §7.4 — the manual re-auth flow for the Defa CloudCharge
token, which must never be committed to the repo or stored in a raw
payload.

## 10. Backups — **TODO**

Spec §40 requires off-box-only backups (`pg_dump`/`pg_basebackup` pushed to
the dev rig, then synced offsite) with no long-term retention on the target
box itself. Document the actual cron/script and the restore procedure here
once set up — and verify a restore on the dev rig, not just the target box,
before considering this done.
