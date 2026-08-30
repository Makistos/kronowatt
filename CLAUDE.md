# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Current state

Spec §43 Step 3 (Cozify) is blocked — no HAN device available yet. Step 4
(FMI weather, observations only) is implemented instead, since it needs
only public internet access. What's real:

- `backend/`: full core DB schema + migrations (see "Database schema"
  below). FMI weather observation collector — fetch, parse, store, health
  tracking — implemented and verified end-to-end against the real FMI API
  and a real TimescaleDB container (`go run ./cmd/server collect weather`
  for one-shot testing; runs continuously inside `go run ./cmd/server` via
  `internal/scheduler`). A REST API (`internal/api`) now serves what's
  collected — `GET /health` (collector status + disk usage) and
  `GET /api/weather/observations` — see "REST API" below.
  `internal/collectors/{cozify,spotprice,ev}` and `internal/analysis` are
  still empty stubs. Config is env-var based (`internal/config`) — see
  "Configuration" below.
- `frontend/`: a real dashboard exists (`src/routes/+page.svelte`) —
  year/compare-year filters, KPI tiles, monthly consumption (grouped bars),
  a temperature-vs-consumption scatter, daily electricity/spot-price lines,
  and EV monthly energy — still built against the fake-data fixtures, not
  the new real API yet (that swap hasn't been done). Chart components live
  in `src/lib/charts/` — see "Frontend dashboard" below for the dataviz
  approach and what's not done (dark-mode rendering unverified — see that
  section). Localized via `svelte-i18n`, English only so far — see
  "Frontend localization".
- `deployment/`: custom TimescaleDB Docker image (builds, extension
  verified), systemd unit files, `deploy.sh` (not yet exercised against a
  real target box).

Treat `kronowatt_spec_v1.1.md` as the source of truth for anything this file
doesn't cover; when the two disagree, the spec wins (it's versioned — check
for a newer `kronowatt_spec_v*.md` before starting work).

Commands:
```
cd backend && go build ./... && go vet ./... && go test ./...
cd frontend && npm run build && npm run check
```

## Configuration

Env vars, loaded by `internal/config.Load()` — not a YAML/config file.
Chosen because the target deployment is a systemd service with an
`EnvironmentFile` (see `deployment/kronowatt-backend.service`); a config
file parser would be an extra dependency for no real benefit at this scale.
Current vars: `KRONOWATT_DB_DSN` (required), `KRONOWATT_HTTP_ADDR` (default
`:8080`), `KRONOWATT_FMI_STATION_FMISID` (default `101786`),
`KRONOWATT_FMI_POLL_INTERVAL` (default `10m`). Revisit if a future
collector needs config too structured for flat env vars (Cozify's spec
§3.3 example shows YAML, but that's illustrative, not a firm requirement).

## FMI weather collector

`internal/collectors/weather` calls FMI's public open-data WFS API
(`https://opendata.fmi.fi/wfs`, `fmi::observations::weather::simple` stored
query) — no auth, unlike Cozify (no device yet) or Defa (unofficial/
reverse-engineered). Verified against a live response for FMISID 101786
(Oulu lentoasema, spec §4.1) rather than assumed from documentation:

- Response is WFS/GML XML; `encoding/xml` matches struct tags against
  element *local* names, ignoring the `wfs:`/`BsWfs:`/`gml:` namespace
  prefixes, since the struct tags here don't specify a namespace. This was
  confirmed against real output, not assumed.
- **FMI encodes "no reading for this parameter at this time" as literal
  `NaN` text**, not by omitting the element (confirmed live — `r_1h`,
  `ri_10min`, `snow_aws` are frequently `NaN`). The collector drops any
  `NaN` value entirely — both from the domain struct (stays `nil`, per the
  spec §4.2 "missing parameter must not fail the observation" rule) and
  from `RawPayload` (`encoding/json` errors on `NaN`, and it isn't a real
  reading anyway).
- Ten parameter codes are mapped to columns: `t2m`→AirTemperature,
  `rh`→RelativeHumidity, `td`→DewPoint, `p_sea`→AirPressure,
  `ws_10min`→WindSpeed, `wd_10min`→WindDirection, `wg_10min`→WindGust,
  `r_1h`→Precipitation, `n_man`→CloudCover, `vis`→Visibility. Confirmed
  live rather than guessed from the spec's field list. Unmapped codes the
  API also returns (`ri_10min`, `snow_aws`, `wawa`) still land in
  `RawPayload` per-timestamp.
- The default query window (no `starttime`) is ~12h at 10-min resolution
  for this station — confirmed live. That's the natural gap-backfill
  margin for a `KRONOWATT_FMI_POLL_INTERVAL` in the spec's suggested
  10-15min range; a genuinely longer outage would need an explicit
  `since` argument to `FetchObservations` (already supported, just not
  wired to persisted "last successful collection time" yet).
- `weather_test.go` uses a trimmed real fixture
  (`testdata/observations_101786.xml`, 4 timestamps) rather than a
  hand-written one or a live network call in tests.

Forecast collection (spec §4.3, separate endpoint/stored query,
never-overwrite-old-versions semantics) is not implemented yet.

## Scheduler

`internal/scheduler.Run(ctx, name, interval, job)` is a generic ticker
loop: run immediately, then on every tick, until `ctx` is cancelled. A
failed run is logged and the loop continues — it deliberately does not
know about storage or collector-specific error handling, so one collector
misbehaving can't take down the loop running another (spec §2.1). Each
collector's actual fetch/store/health-recording logic is composed into a
`scheduler.Job` closure in `cmd/server` (see `newWeatherJob` in
`cmd/server/collect.go`) — the composition root, not the collector package
itself, is what wires a collector to storage.

Logging is `log/slog` with the default text handler (`slog.Default()`),
used for anything recurring/collector-related; plain `log.Fatal` is still
fine for one-shot startup failures in `main()`.

## REST API

`internal/api.NewRouter(pool, diskCheckPath)` builds the `http.Handler`
`cmd/server` serves — it depends only on `storage` repositories, never on
collector internals (spec §2.5), and is the only package that knows about
JSON wire shapes (DTOs are defined per-handler file, e.g.
`weatherObservationDTO` in `weather.go`, kept separate from the `domain`
structs storage returns).

- `GET /health` — collector status (from the `collector` table) plus disk
  usage (`syscall.Statfs` on `KRONOWATT_DISK_CHECK_PATH`, default `/` — the
  target box has one SSD per spec §9a, so any path reflects overall
  pressure). Reports `"degraded"` if any collector's status is `"error"` or
  disk usage is ≥80% (spec §9a/§44.17's threshold) — verified live against
  a real container (13.9% disk usage, `fmi_observation` status `"ok"`).
- `GET /api/weather/observations` — real rows from `weather_observation`.
  Takes either `?year=2025` (maps to `[Jan 1, Jan 1 next year)` UTC —
  chosen to mirror how the frontend's fake-data fixtures are split one file
  per year, since that's the shape a future real-data frontend integration
  will want) or explicit `?start=...&end=...` (RFC3339). Both forms share
  `parseDateRange` in `daterange.go` — reuse it for any future time-series
  endpoint rather than re-implementing range parsing per handler. Verified
  live: real FMI data round-tripped through the API with all fields
  present, missing params correctly 400, malformed `year` correctly 400.
- CORS is wide open (`Access-Control-Allow-Origin: *`) for all GET/OPTIONS
  requests. Deliberate, not an oversight: frontend and backend are separate
  services on separate ports even in production (spec §9's deployment
  split), and this is a LAN-only app (spec §1) — an allowlist would just be
  one more thing to keep in sync with whatever host/port the frontend is
  served from, for little real security benefit here.
- **Not yet done**: the frontend dashboard still reads the fake-data JSON
  fixtures, not this API — swapping `src/lib/fakeData.ts`'s loaders for
  real `fetch('/api/...')` calls (behind a dev-time proxy or absolute
  backend URL) is a separate task. Electricity/spot-price/contract/EV
  endpoints don't exist yet because those collectors don't exist yet.

## What this system is

A local-first home energy monitoring and analysis system for a single
household, deployed on one dedicated machine (Fujitsu Esprimo Q510, Debian
13, 30GB SSD, ~22GB free). It collects, stores, and analyzes:

- electricity measurements from a Cozify HAN interface (local WebSocket/REST)
- weather observations/forecasts from FMI
- Finnish electricity spot prices
- manually-entered electricity contract terms
- EV charging data from a Defa Power charger via an unofficial cloud API (the
  one deliberate exception to local-first)

Read `kronowatt_spec_v1.1.md` in full before implementing any part of this
system — it is dense and implementation decisions in one section (e.g.
storage budget, §9a) constrain choices in others (e.g. retention policy,
compression, schema design).

## Core architectural principles (§2)

These are load-bearing constraints, not suggestions:

- **Local-first**: all data lives locally; external services are sources,
  not the database of record. The system must keep working when an external
  API is down. The sole exception is the EV/Defa collector, which is
  cloud-dependent by necessity and must be fully isolated — its failure,
  rate-limiting, or expired auth must never affect any other collector or
  the rest of the app.
- **Raw preservation, budget-bounded**: collectors normalize source data into
  domain models but also keep raw payloads — subject to the storage budget
  in §9a, not unconditionally forever. Never store auth tokens/secrets
  inside a raw payload (especially the Defa cloud token).
- **Time-series first**: timestamps/intervals are first-class across every
  data type.
- **Historical correctness**: cost calculations must use the contract/price
  rules valid at the time in question. Editing or adding a current contract
  must never retroactively change historical results.
- **Provider isolation**: each external source sits behind its own
  provider/collector interface (`Cozify HAN -> ElectricityProvider`,
  `FMI -> WeatherProvider`, `spot price provider -> SpotPriceProvider`,
  `Defa cloud -> EVProvider`) feeding a shared internal model. Provider-
  specific API shapes must not leak into the rest of the application.
- **Idempotent collection**: re-running a collector must never create
  duplicate logical measurements — enforce via source identifiers and/or DB
  uniqueness constraints, not application-level dedup logic alone.
- **Explainable, deterministic calculations**: no ML in the initial
  implementation; every cost/analysis number must be traceable to inputs.

## Planned repository structure (§10)

```
kronowatt/
├── backend/
│   ├── cmd/server/
│   ├── internal/
│   │   ├── collectors/{cozify,weather,spotprice,ev}/
│   │   ├── domain/
│   │   ├── storage/
│   │   ├── analysis/
│   │   ├── api/
│   │   ├── scheduler/
│   │   └── config/
│   └── migrations/
├── frontend/
│   └── src/
├── deployment/
│   ├── docker-compose.yml        -- TimescaleDB only
│   ├── kronowatt-backend.service
│   ├── kronowatt-frontend.service
│   └── deploy.sh                 -- rsync/scp build artifacts to the box
├── docs/
└── README.md
```

Each collector directory under `internal/collectors/` should stay behind its
provider interface and not be imported directly by `api/` or `analysis/` —
those should depend on `domain`/`storage`, not on collector internals.

## Technology stack (§9)

- **Backend**: Go, standard `net/http` stack, pgx driver, a migration
  framework, structured logging, `context.Context` throughout, standard
  `testing` (avoid unnecessary frameworks). Compiles to a single static
  binary, built on the dev machine and copied to the target box.
- **Database**: TimescaleDB on PostgreSQL. Native compression is mandatory
  from the *first* migration (compress chunks older than ~3–7 days) — this
  is what makes 5+ years of data fit the 22GB budget. Do not treat
  compression as a later optimization.
- **Frontend**: SvelteKit + TypeScript, prebuilt to static assets on the dev
  machine, served by a small static file server (or the backend) on-target.
  Talks only to the backend API.
- **Deployment**: hybrid, not full containerization. Only TimescaleDB runs in
  Docker (for extension/version management and `pg_dump`/`pg_basebackup`
  tooling); the Go backend and frontend static server run as systemd
  services from build artifacts. See §9 for the full rationale table.

## Storage budget (§9a) — a real constraint, not boilerplate

30GB disk, ~22GB free, this service only, fixed overhead ~2.5–4GB. Electricity
data (Cozify, ~10s samples) dominates growth and is the only source worth
budgeting carefully — expect ~150–400MB/year *with* compression vs.
~1–1.5GB/year without it. Retention policy (replaces any "keep everything
forever" assumption):

```
Electricity, spot prices, contracts: permanent, compressed after ~7 days
Weather observations:                permanent, compressed after ~7 days
Weather forecasts:                   1-2 years, then dropped
Raw payloads:                        configurable cap (default: keep;
                                      revisit if disk usage crosses ~70%)
Logs:                                7 days, size-capped
```

The health endpoint must expose disk usage and report "degraded" above ~80%
usage (§35/§44.17). When adding any new persistent data path, check it
against this budget and retention table rather than defaulting to "store
everything forever."

## Fake data for frontend development

There is no backend API yet, so `frontend/scripts/generate-fake-data.mjs`
generates static JSON fixtures instead — a stand-in until spec §43 Step 8
exists, not a preview of the real API's response shape. Run
`npm run generate:fake-data` (optionally `-- --start-year 2020 --end-year
2025 --annual-kwh 12000 --ev-sessions-per-week 3 --ev-kwh-per-session 35`)
to (re)generate `frontend/static/fake-data/{electricity,weather,spot_price,
ev_sessions}_{year}.json` plus a `manifest.json` listing the years/params
used. Output is gitignored and deterministic per `--seed` (default 42) —
regenerate rather than editing the JSON by hand.

Modeling choices worth knowing if this needs adjusting:
- Hourly resolution, not the real 10s Cozify sampling rate — a full year at
  10s would be ~3M rows, unworkable for a browser fetch. If per-source
  raw-resolution testing is ever needed, that's a different, much smaller
  fixture, not a resolution bump here.
- Household consumption uses a fixed month-weight curve
  (`MONTH_WEIGHT` in the script) forced to put exactly 50% of annual kWh in
  Jan-Mar, then a shared per-day random draw nudges both that day's
  synthetic temperature *and* its consumption in the same direction
  (colder day → more heating load) — so a temperature-vs-consumption chart
  built on this data will show a real, not just coincidental, correlation.
- Temperature is a synthetic annual cosine (Jan ≈ -12°C, Jul ≈ +14°C, loosely
  Oulu-shaped) plus daily/hourly noise — not calibrated against real FMI
  normals.
- Spot price and EV sessions are generated independently of the
  temperature/consumption model (no cross-correlation with weather).

## Frontend dashboard

`src/routes/+page.svelte` fetches manifest + per-year fixtures client-side
(`onMount`, no SvelteKit `load` function) rather than at prerender time —
deliberate, since `static/fake-data/` is gitignored and generated on demand;
coupling it to SSR/prerendering would either bake in stale data or break a
fresh clone's `npm run build`. If fetching fails (fixtures not generated),
the page shows instructions to run `npm run generate:fake-data` instead of
a raw error.

Chart components (`src/lib/charts/`) are hand-rolled inline SVG, not a
charting library — built by following the `dataviz` skill's method
end-to-end rather than improvising:
- Color is the validated default palette from the skill's `palette.md`,
  copied verbatim into `src/app.css` as CSS custom properties (`--series-1`
  … `--series-8`, plus text/surface/gridline tokens for light and dark).
  Re-ran the skill's `validate_palette.js` against both modes after
  copying — both pass. Only slots 1-2 (blue/orange) are actually used, for
  year-over-year comparison; the palette's light-mode contrast WARN
  (slots 3/4/5 — aqua/yellow/magenta) doesn't apply here because nothing
  uses those slots as a series encoding yet.
- Monthly views are grouped bar charts (`MonthlyBarChart.svelte`) with a
  table-view toggle (the skill's accessibility-twin requirement). Daily
  views are multi-series line charts with a shared crosshair + one tooltip
  listing every series (`DailyLineChart.svelte`). The temperature vs.
  consumption relationship is a scatter plot (`ScatterChart.svelte`), not
  two line charts on a dual axis — the skill flags dual-axis charts as the
  single most common charting mistake, and a scatter is the honest form for
  "does X relate to Y," which is exactly what connects them.
- **Not verified**: dark mode. This sandbox's headless Chrome doesn't honor
  `prefers-color-scheme` emulation via CLI flags (confirmed with a minimal
  `matchMedia` test — a tooling limitation, not something to fix in the
  app), so only light mode has actually been screenshotted. The dark CSS
  values follow the skill's documented pattern exactly (same selectors as
  `palette.md`'s example) but haven't been visually confirmed — check this
  in a real browser before trusting it.
- Legend/tooltip identity is always color + text label together (never
  color-only), per the skill's "text never wears the data color" rule —
  legend swatches and tooltip series-keys are small colored rects/lines
  beside plain-ink text, not colored text.

## Frontend localization

Uses `svelte-i18n` (store-based, no compiler step) rather than a
compile-time solution like Paraglide — simpler to wire up correctly for a
single-locale start, and doesn't add a build-time codegen step for one
language. English (`src/lib/i18n/locales/en.json`) is the only registered
locale; `src/lib/i18n/index.ts` hardcodes `initialLocale: 'en'` rather than
detecting the browser's language, since there's nothing else to fall back
to yet.

- **Every UI string is a translation key** — dashboard chrome, chart
  titles/axis labels, table headers, the fake-data-missing empty state.
  Don't add a new hardcoded English string to a component; add a key to
  `en.json` instead, even though only English exists right now.
- **Month names are language-neutral keys, not labels**:
  `MONTH_KEYS` in `aggregate.ts` (`'jan'`…`'dec'`) index the 12 monthly
  buckets; a component resolves them to display text via
  `$translate(\`months.${key}\`)`. Never reintroduce an English
  `MONTH_LABELS` array — that was the pre-i18n version of this and it's
  gone for a reason.
- **Numbers and dates are locale-aware, not just strings**: `formatNumber`,
  `formatCompact`, `formatDate` (`src/lib/format.ts`) all take a `locale`
  parameter, threaded from svelte-i18n's `locale` store (`$locale ?? 'en'`)
  at every call site. This matters once a second locale exists (e.g.
  Finnish formats decimals/thousands differently) even though it's
  invisible with only English registered.
- **Import alias is `_ as translate`, never `_ as t`.** Several charts loop
  `{#each yTicks as t (t)}` — aliasing the translation store to `t` would
  get shadowed by that loop variable and silently break inside it (caught
  this once already; don't reintroduce it).
- **Avoid `{@html}` for interpolated translations.** Tried wrapping part of
  a translated sentence in `<code>` via an interpolated value once; reverted
  it before committing — it forces `{@html}` on translated content (a risk
  vector if a value is ever less trusted than today's static string) and
  splitting a sentence around inline markup reads badly once a language
  with different word order is added. Keep translated strings plain text.
- `+layout.svelte` gates rendering on svelte-i18n's `$isLoading` store so
  there's no flash of untranslated keys before the locale JSON loads.

## Migrations

Tool: [goose](https://github.com/pressly/goose) (`github.com/pressly/goose/v3`),
chosen over golang-migrate/tern for its `go:embed` support — migrations
compile into the binary, matching the single-static-binary deployment model
(no separate migration tool needs to exist on the target box).

- SQL migration files live in `backend/migrations/*.sql`
  (`-- +goose Up` / `-- +goose Down` markers) and are embedded via
  `backend/migrations/embed.go` (`package migrations`, `embed.FS`). Because
  `go:embed` patterns can't reference parent directories, that package must
  stay inside `backend/migrations/` itself — don't move the embed directive
  into `internal/storage` or elsewhere.
- Run via the server binary's `migrate` subcommand, not a standalone CLI:
  `KRONOWATT_DB_DSN=postgres://... go run ./cmd/server migrate up` (also
  supports `down`, `status`, etc. — anything `goose.RunContext` accepts).
  `KRONOWATT_DB_DSN` is a placeholder until `internal/config` settles on a
  real config format; expect this env var to move once that happens.
- `00001_timescaledb_extension.sql` (`CREATE EXTENSION IF NOT EXISTS
  timescaledb;`) is deliberately the only migration so far — it's
  infrastructure, not domain schema. The Docker image
  (`deployment/timescaledb/init/`) already creates this extension too; the
  migration is a no-op belt-and-suspenders for anyone running the binary
  against a Postgres that isn't the shipped image.
## Database schema

All §11 domain tables exist (migrations `00002`–`00007`), verified against a
real TimescaleDB container (hypertables created, compression/retention
policies active, idempotency constraints enforced, up/down/reset all
tested). Design decisions worth knowing before touching this:

- **Hypertables** (partitioned on a `time`/`interval_start`/`generated_at`
  column): `electricity_measurement`, `weather_observation`,
  `weather_forecast`, `spot_price`, `ev_measurement`. Compression (§9a,
  ~7-day-old chunks) is on for all of these *except* `weather_forecast`
  (which gets a 2-year drop/retention policy instead — spec §9a puts it in
  a different bucket than the others) and `ev_measurement` (spec's
  compression table doesn't mention EV at all, and volume is negligible).
- **Plain tables** (not hypertables): `collector` (mutable health state),
  `electricity_contract` / `contract_price_period` (small, manually
  entered, mutable), `ev_charging_session` (sparse, mutable while a
  session is in progress — doesn't fit hypertables' append-only chunk
  model).
- **Idempotency (§2.6)** is enforced via `UNIQUE` constraints, not
  application-level dedup: `electricity_measurement` and `ev_measurement`
  are unique on `time` alone (*not* `(time, source)`) — a live sample and a
  history-backfilled sample for the same device timestamp are the same
  logical measurement regardless of which endpoint produced it, so
  `source` must not be part of the key. `weather_observation` is unique on
  `(time, station_fmisid)`; `spot_price` on `(interval_start, source)`;
  `weather_forecast` on `(generated_at, target_time, provider)` (deliberately
  *not* deduped across generation runs — old forecast versions must survive,
  spec §4.3); `ev_charging_session` on `(source, start_time)`.
- **Contract pricing is period-scoped, not contract-scoped**: `energy_price`,
  `spot_margin`, `monthly_fee`, `transfer_price`, `taxes`, `vat`, and
  `other_fees` all live on `contract_price_period`, not
  `electricity_contract`. This is an interpretation of spec §6 (the fields
  list doesn't explicitly say which level they belong on) driven by §2.4:
  "effective price determined by timestamp" only works if pricing is
  time-sliced. If this turns out wrong, it's a migration away, not a big
  rewrite — nothing else depends on it yet.
- **No FK between `ev_measurement` and `ev_charging_session`** — the spec's
  field lists (§7.3) don't include a linking column, and a time-range join
  (`start_time <= time <= end_time`) is enough for analysis. Revisit if that
  turns out to be wrong once real Defa data exists.
- EV field lists (`ev_charging_session`, `ev_measurement`) are explicitly
  provisional per spec §7.3/§37 — expect to revise columns once verified
  against `ha-defa-power`'s actual schema.

## Source-specific notes worth remembering

- **Cozify HAN (§3)**: primary collection is via WebSocket (`/ws`,
  `HAN_METER_MESSAGE` events, ~10s device sampling). The device also exposes
  on-device rolling history (`/history/hourly|weekly|monthly|yearly` at
  decreasing resolution) — use it to backfill gaps on collector
  startup/reconnect instead of leaving holes in the timeseries. No auth is
  needed for read access. Do not invent fields beyond what a live device
  response actually contains; the field list in §3.2 is provisional pending
  confirmation against real hardware. Don't hard-code the device's IP —
  make host configurable.
- **FMI weather (§4)**: fixed observation station (Oulu lentoasema,
  FMISID 101786) plus point forecasts for the user's actual coordinates.
  Forecast versions must never be overwritten — old forecasts are needed
  later for forecast-accuracy evaluation. Missing one optional weather
  parameter must not fail the whole observation.
- **Spot prices (§5)**: model as 15-minute intervals
  (`interval_start, interval_end, price, ...`), not points. Distinguish
  "when the price applies" from "when the app learned it" — store both.
- **Contracts (§6)**: manually entered, support multiple price periods per
  contract, effective price resolved by timestamp, historical contract data
  immutable unless explicitly edited by the user.
- **EV / Defa Power (§7)**: no official API exists. The only integration
  path is the community `ha-defa-power` project against Defa's unofficial,
  reverse-engineered CloudCharge cloud API (rate-limited, can break without
  notice). This collector is read-only (no charge control), must poll
  conservatively (1–5 min, backing off when idle), and must degrade
  independently — health status should show a specific "EV: degraded /
  needs re-auth" state rather than a generic error. Expect to build a manual
  re-auth flow for token expiry.
- **Backups (§40)**: off-box only — `pg_dump`/`pg_basebackup` pushed to the
  dev rig, then synced offsite. No long-term backup retention lives on the
  target box itself (the 22GB budget is for live data). Restore procedure
  must be tested on the dev rig, not just the target box.

## Implementation sequence (§43)

The spec defines an explicit build order — Step 0 (environment/storage
policy) → Step 1 (repo/build) → Step 2 (DB/migrations) → Step 3 (Cozify) →
Step 4 (FMI) → Step 5 (spot price) → Step 6 (contracts) → Step 7 (cost
engine) → Step 8 (frontend MVP) → Step 9 (EV) → Step 10 (analysis) → Step 11
(Fingrid, optional) → Step 12 (forecasting, later). Each step lists its own
deliverable in the spec. Follow this order for greenfield work unless the
user directs otherwise — later steps (e.g. cost engine, contract simulation)
assume earlier ones (contracts, spot prices, electricity measurements) are
already in place.

## Sections referenced but not expanded in v1.1

§§13–38 (time handling, normalization, collector architecture, data quality,
missing-data handling, cost engine, contract-vs-spot simulation,
consumption-weighted spot price, weather correlation, heating degree
calculation, REST API design, frontend MVP/design/dashboards, auth/security,
configuration, logging, monitoring, migrations, testing) are carried forward
unchanged from spec v1.0, which is not present in this repo. If detail beyond
what v1.1 restates is needed for one of these areas, ask the user for the
v1.0 document rather than inventing the missing detail.
