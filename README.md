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

Core database schema and migrations exist (see `CLAUDE.md`); no collectors
or API endpoints are implemented yet. Follow the implementation sequence in
spec §43.

## Backend

```
cd backend
go build ./...
go test ./...
go run ./cmd/server
```

## Frontend

```
cd frontend
npm install
npm run generate:fake-data   # test fixtures, no backend needed yet — see CLAUDE.md
npm run dev                  # local dev server
npm run build                # static assets to frontend/build
```

## Database (dev)

```
cd deployment
docker compose up -d
```
