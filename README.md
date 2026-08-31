# Kronowatt

A local-first home energy monitoring and analysis system. See
[`kronowatt_spec_v1.1.md`](./kronowatt_spec_v1.1.md) for the full
implementation specification, [`CLAUDE.md`](./CLAUDE.md) for a condensed
architecture/constraints summary, and [`INSTALL.md`](./INSTALL.md) for
step-by-step installation onto the target box.

## Layout

- `backend/` — Go backend (collectors, domain model, storage, analysis, API, scheduler).
- `frontend/` — SvelteKit + TypeScript frontend, built to static assets.
- `deployment/` — Docker Compose (TimescaleDB), systemd units, deploy script.
- `docs/` — supplementary documentation.

## Status

Core DB schema, a REST API, and a FMI weather collector exist. Cozify is
blocked on hardware; spot-price/EV collectors aren't implemented yet
(`backend/cmd/server seed` loads synthetic test data for those instead —
see `CLAUDE.md`). Follow the implementation sequence in spec §43.

## Database (dev)

```
cd deployment
docker compose up -d
```

## Backend

```
cd backend
go build ./... && go vet ./... && go test ./...
KRONOWATT_DB_DSN='postgres://kronowatt:<password>@127.0.0.1:5432/kronowatt?sslmode=disable' go run ./cmd/server migrate up
go run ./cmd/server            # serves the API + runs the weather collector
```

## Frontend

Fetches from the backend API (`VITE_API_BASE_URL`, default
`http://localhost:8080`) — start the backend first, and seed it with test
data if no real collectors have run yet:

```
cd frontend
npm install
npm run generate:fake-data                      # (re)generate test fixtures
cd ../backend && go run ./cmd/server seed ../frontend/static/fake-data
cd ../frontend
npm run dev                  # local dev server
npm run build                # static assets to frontend/build
```
