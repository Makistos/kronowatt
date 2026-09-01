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
  `internal/scheduler`). A REST API (`internal/api`) serves electricity,
  weather, spot price, and EV session data plus health — see "REST API"
  below. A `seed` subcommand loads the frontend's fake-data fixtures into
  the real database for end-to-end testing without real collectors for
  everything — see "Seeding fake data into the real database".
  `internal/collectors/{cozify,spotprice,ev}` and `internal/analysis` are
  still empty stubs (no real spot-price or EV collector exists — only
  weather does — so `spot_price`/`ev_charging_session` only ever have
  seeded fake rows unless you run one manually). Config is env-var based
  (`internal/config`) — see "Configuration" below.
- `frontend/`: a real dashboard exists (`src/routes/+page.svelte`) —
  year/compare-year filters, KPI tiles, monthly consumption (grouped bars),
  a temperature-vs-consumption scatter, daily electricity/spot-price lines,
  and EV monthly energy — now fetches from the real backend API
  (`src/lib/api.ts`), not static fake-data JSON (that file, `fakeData.ts`,
  is gone). Chart components live in `src/lib/charts/` — see "Frontend
  dashboard" below for the dataviz approach and what's not done (dark-mode
  rendering unverified — see that section). Localized via `svelte-i18n`,
  English only so far — see "Frontend localization".
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
- `GET /api/weather/observations`, `GET /api/electricity/measurements`,
  `GET /api/spot-prices`, `GET /api/ev/sessions` — real rows from the
  corresponding table. All take either `?year=2025` (maps to
  `[Jan 1, Jan 1 next year)` UTC — chosen to mirror how the frontend's
  fake-data fixtures split one file per year) or explicit
  `?start=...&end=...` (RFC3339), sharing `parseDateRange` in
  `daterange.go` — reuse it for any future time-series endpoint rather
  than re-implementing range parsing per handler.
- `GET /api/meta/years` — distinct years present in
  `electricity_measurement` (the "spine" dataset), so the frontend can
  discover what's available instead of hardcoding a year list.
- `GET /api/meta/range` — earliest/latest `electricity_measurement`
  timestamps (`{"min":..., "max":...}`, both `null` if the table is empty).
  Exists so the frontend can default pickers to the *actual* latest data
  instead of assuming a full calendar year — early production has a
  partial first year (data starting mid-year, nothing yet for months that
  haven't happened), not a complete Jan-Dec span. See "Electricity tab"
  below for where this actually gets used.
- The electricity DTO exposes `power_kw`/`phases_kw` (from `p[0]`/`p[1:]`)
  but deliberately **not** an energy value — converting power to energy
  needs the sample interval, which this API doesn't track (real Cozify
  samples land every ~10s; seeded fake data is 15-min), so that conversion
  belongs wherever the caller knows what resolution it asked for. See the
  comment on `electricityMeasurementDTO`, and `RAW_SAMPLE_INTERVAL_HOURS`
  in the frontend's `api.ts` where the conversion actually happens.
- CORS is wide open (`Access-Control-Allow-Origin: *`) for all GET/OPTIONS
  requests. Deliberate, not an oversight: frontend and backend are separate
  services on separate ports even in production (spec §9's deployment
  split), and this is a LAN-only app (spec §1) — an allowlist would just be
  one more thing to keep in sync with whatever host/port the frontend is
  served from, for little real security benefit here.
- Verified live against a real TimescaleDB container: real FMI data and
  seeded fake electricity/spot-price/EV data all round-tripped through
  their endpoints with correct fields; missing/malformed params correctly
  400; the frontend dashboard rendered from these endpoints end-to-end
  (screenshotted), and correctly showed its empty-state error (not stale
  cached data) when the backend was killed mid-session.
- **Not yet done**: no contract endpoints (no `electricity_contract` table
  data exists, seeded or real — the fake-data generator doesn't produce
  contracts). No real spot-price or EV collectors, so those two tables only
  ever have seeded fake rows today.

## Seeding fake data into the real database

`go run ./cmd/server seed [dir]` (default `dir`:
`../frontend/static/fake-data`, i.e. run from `backend/`) reads the Node
generator's JSON output and inserts it into `electricity_measurement`,
`weather_observation`, `spot_price`, and `ev_charging_session` — so the API
(and the frontend, which now talks to the API, not the JSON files directly)
has multi-year data without needing real collectors for everything. This is
dev/test tooling, not a spec feature: the generator's fixtures are the seed
source specifically so there's one definition of "what the synthetic
dataset looks like," not a duplicated one in Go.

Every seeded row is tagged to keep it identifiable and non-colliding with
real collected data:
- `electricity_measurement.source` / `spot_price.source` /
  `ev_charging_session.source` = `"fake_seed"` — including for EV, where
  the generator's own JSON says `source: "defa_cloud"`; the seed command
  deliberately overrides that so a seeded session can never be confused
  with a real Defa one sharing that label.
- `weather_observation.station_fmisid` = `"fake"`, not the real `"101786"`
  — weather has no `source` column, so the station id is the only
  available marker. Verified live: seeding fake weather rows and running
  the real FMI collector at the same time produced no collisions (real
  data goes to `101786`, fake to `"fake"`).
- Electricity's `ic` (cumulative imported energy) is synthesized as a
  running sum of the fake data's `energy_kwh` — verified live that the
  cumulative total at year-end matches the generator's own reported annual
  kWh. `p` is stored as `[total, phase1, phase2, phase3]` (from the
  generator's `power_kw` + `phases_kw`), matching the real Cozify wire
  format's total-then-phases ordering (spec §3.2).

The command does per-row `INSERT ... ON CONFLICT DO NOTHING` (same
idempotency pattern as the collectors), so re-running it is safe but slow
(~20-40s for two years at 15-min electricity resolution — tens of
thousands of individual round trips, not batched). Fine for occasional dev
seeding; would need a bulk-insert path (e.g. `COPY`) if this ever needs to
run often or on much more data.

**Regenerating the fixtures shifts every dataset's specific values, even
unrelated ones** — the generator draws from one seeded PRNG sequentially
across electricity → weather → spot price → EV, so changing how much
randomness electricity consumes (e.g. going from hourly to 15-minute) shifts
what spot price and EV get, byte-for-byte, even though their own generation
code didn't change. This is harmless on its own (still deterministic, just
different specific values), but it means **re-running `seed` after
regenerating fixtures without clearing old rows first creates duplicates**,
not replacements: old rows keyed by their old timestamps survive
`ON CONFLICT DO NOTHING` (weather/spot price share timestamps so old values
just linger unchanged; EV sessions get entirely new `start_time`s and
straight-up double). Hit this once — EV session count silently doubled.
Always `DELETE FROM <table> WHERE source = 'fake_seed'` (and
`WHERE station_fmisid = 'fake'` for weather) for all four tables before
re-seeding after any generator change, not just for the table you think
changed.

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

## Fake data generation

`frontend/scripts/generate-fake-data.mjs` produces the synthetic dataset
used to seed the real database (see "Seeding fake data into the real
database") — its original purpose (something the browser fetched directly,
before the backend/API existed) is gone, but the generator itself is still
the one source of truth for "what the synthetic dataset looks like." Run
`npm run generate:fake-data` (optionally `-- --start-year 2020 --end-year
2025 --annual-kwh 12000 --ev-sessions-per-week 3 --ev-kwh-per-session 35`)
to (re)generate `frontend/static/fake-data/{electricity,weather,spot_price,
ev_sessions}_{year}.json` plus a `manifest.json` (read by
`backend/cmd/server seed`, not by the frontend anymore). Output is
gitignored and deterministic per `--seed` (default 42) — regenerate rather
than editing the JSON by hand, then re-run `seed` to load the changes into
the database.

Modeling choices worth knowing if this needs adjusting:
- Electricity is 15-minute resolution (~35k rows/year); weather, spot
  price, and EV sessions stay hourly/event-based. Not the real 10s Cozify
  sampling rate either way — a full year at 10s would be ~3M electricity
  rows. Electricity specifically needed finer-than-hourly resolution so the
  dashboard's hour/15-min chart views (see "Electricity tab" below) have
  real per-period data to drill into rather than an average; the other
  three datasets never got an hour/15-min view, so they didn't need it.
- Electricity also carries a synthetic 3-phase split (`phases_kw`,
  `splitPhases` in the script) — see "Electricity tab" below for why and
  how.
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

`src/routes/+page.svelte` fetches available years + per-year data
client-side (`onMount`, no SvelteKit `load` function) from the real backend
via `src/lib/api.ts`, rather than at prerender time — deliberate, since the
backend/DB may not be reachable at build time (this is a static-adapter
site with no server-side rendering of live data), so coupling data
fetching to SSR/prerendering would either bake in stale data or break a
build with no backend running. If fetching fails (backend down, or DB not
seeded), the page shows an explanatory empty state instead of a raw error
or, worse, silently showing nothing — verified live by killing the backend
mid-session and confirming the error state renders (not stale data).

`api.ts` talks to `VITE_API_BASE_URL` (default `http://localhost:8080`,
see `.env.example`) — a Vite build-time env var, since adapter-static has
no server to read env vars at runtime; a production deploy must set this
*before* `npm run build`, not after. It also adapts the backend's richer
DTOs (e.g. electricity's raw `ic`/`p[]` fields) back into the simple
`{time, value}` shapes `aggregate.ts` and the charts already expect, so
switching from the old fake-data fixtures to the real API didn't require
touching the aggregation/chart code at all — only the data-loading layer.
The old `fakeData.ts` (fetching `/fake-data/*.json` directly) is gone;
`frontend/scripts/generate-fake-data.mjs` and its output are still very
much alive, just repurposed as the seed source for the real database (see
"Seeding fake data into the real database") rather than something the
browser fetches directly.

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

## Electricity tab — resolution, phases, comparison, temperature

The dashboard's Electricity tab (`src/lib/charts/ElectricityChart.svelte` +
`ElectricityBarChart.svelte`) is the one chart with its own independent
controls — resolution, phase breakdown, comparison, date picker(s) — rather
than using the page-level Year/Compare filters that the other three tabs
(Weather, Spot price, EV) still share. It replaced the old separate
"Monthly electricity consumption" and "Daily electricity consumption"
charts entirely.

**Drill-down windowing** (`WINDOW_OF` in `electricityBuckets.ts`) — each
resolution operates within a fixed window, and the picker shown adapts
accordingly:
```
year            -> no window, shows every available year, no date picker
month, week     -> one year        -> year <select>
day             -> one month       -> <input type="month">
hour, quarter   -> one day         -> <input type="date">
```
This exists because fetching a full year at 15-minute resolution to show
one bar chart would be 35,040 bars — the window keeps each view's bar count
sane (max ~96, at 15-min-of-day) while still showing *real* per-period
values rather than an averaged profile.

**Positional bucketing, not date-keyed** (`electricityBuckets.ts`): buckets
are indexed by *position within the period* (month 0–11, day-of-month,
hour-of-day, etc.), not by absolute calendar date. This is what makes
comparison work at all — comparing March 2025 to March 2024 means aligning
"day 5" to "day 5", and the two periods essentially never share actual
dates. `bucketByWeek`/`bucketByDay` take an explicit bucket count from the
caller (e.g. `Math.max` of both compared periods' week/day counts) so a
53-week year or a 31-day month being compared against a shorter one doesn't
lose data.

**Phase data is synthetic**: the real Cozify device hasn't been confirmed
to report per-phase power at all (spec §3.2's field list is provisional).
`generate-fake-data.mjs` synthesizes a 3-phase split (`splitPhases`, base
weights 0.36/0.33/0.31 plus per-quarter-hour noise, renormalized so phases
always sum exactly to the total) purely so this feature has something to
render. The fake data generator also moved from hourly to **15-minute**
resolution for electricity specifically (weather stayed hourly) — the
hour/15-min chart views need real per-period data to drill into, not an
average.

**Color encodes the comparison period; opacity encodes phase** — not two
separate categorical hues. `ElectricityBarChart`'s `STACK_OPACITY = [1,
0.6, 0.35]` steps down the *group's own color* per phase, rather than
giving phases their own categorical slots. This was a deliberate choice
over spending 3 more categorical slots on phases: color stays reserved for
the dimension that's actually being compared (which period), and opacity
is a legitimate secondary encoding for a sub-breakdown within that. When
both comparison and phases are on at once, this becomes a grouped+stacked
bar chart — legend shows both the period-color swatches and the
phase-opacity swatches.

**A real latent bug found and fixed while building this**: `MonthlyBarChart`,
`DailyLineChart`, and the initial `ElectricityBarChart` all computed their
y-axis max as `niceMax(Math.max(1, ...values))`. `niceMax` already returns
1 for an empty/zero-only series — wrapping it in `Math.max(1, ...)` doesn't
just handle that case, it also floors any *real* positive max below 1 up to
1, silently compressing the scale. Invisible while every chart dealt in
hundreds of kWh; immediately visible once 15-minute buckets (~0.1–1 kWh)
existed — the y-axis showed "1, 1, 1, 0" because every tick rounded to the
same integer. Fixed by using `Math.max(0, ...)` in all three, plus adaptive
axis decimal places (`yAxisDigits` in `ElectricityBarChart`) so small-scale
data doesn't hit the same rounding collision again.

Verified interactively (not just screenshotted at defaults) using a
throwaway Puppeteer script driving the real dev server: every resolution,
phase toggle, comparison toggle, both together, and the table-view toggle
all confirmed rendering correctly against the real seeded backend.

**Temperature overlay** ("Show temperature" checkbox): a small-multiples
line chart (`DailyLineChart`, reused) rendered directly below the bar
chart, sharing the same x-axis category positions — not a second y-axis on
the same chart. Deliberate: the dataviz skill flags dual-axis (two y-scales
on one chart) as the single most common charting mistake, and this is
exactly the "two measures of different scale" case it says should be two
charts sharing an axis instead. `weatherBuckets.ts` mirrors
`electricityBuckets.ts`'s positional bucketing but **averages** for
year/month/week/day and shows the **actual** reading for hour/quarter — per
explicit instruction ("15m and 1h charts show actual temperature, for
others show average"). Because weather data is hourly (not 15-minute — see
"Fake data generation" above), "actual" at 15-min resolution means the real
hourly reading is *stepped* across its 4 quarter-hour buckets rather than
faked at finer granularity; the rendered line visibly shows this as flat
4-point segments, which is honest given there's no finer real data, not a
bug. Verified live across all six resolutions, including with comparison
on (two temperature lines, same color-per-period convention as the bar
chart).

**Fixing `DailyLineChart` to build this surfaced a second real bug**: its
y-scale assumed a `[0, max]` domain (bars-grow-from-zero thinking leaking
into a line chart). Temperature is routinely negative in Finnish winter, so
values below zero would have plotted off-canvas below the chart. Fixed by
computing a proper `[yMin, yMax]` domain that only extends below zero when
the data actually does, with the zero baseline drawn wherever `y(0)` falls
rather than assumed to be the bottom edge. This same component is also used
for spot price, which can go negative too (the fake-data generator
produces negative summer-night prices) — the bug was latent there as well,
just never visibly triggered because daily *averages* happened to stay
positive.

**KPI row**: the "Average temperature" tile was removed (now redundant
with the Electricity tab's own temperature overlay) — `avgTemp` in
`+page.svelte` was deleted along with it, not left as dead code.

**Spot price really is 15-minute now, not hourly** — `generateSpotPriceYear`
in the fake-data generator originally simplified spot price to hourly
resolution (documented at the time as an acceptable deviation, since
nothing depended on finer granularity yet). Once cost comparison needed to
match each electricity sample to *its own* spot price rather than an
hourly approximation, that deviation became a real accuracy problem — spec
§5 itself specifies 15-minute intervals, and Nordic spot markets actually
settle at 15-minute resolution in reality too, so this wasn't a stretch,
just finishing what was already simplified. Electricity and spot price are
now on an *identical* 15-min grid, so cost matching is an exact-timestamp
`Map` lookup, not a truncate-to-hour approximation. `seed.go`'s
`IntervalEnd` changed from `+1 hour` to `+15 minutes` to match. This
roughly doubles seed time (~40-60s for two years now, still per-row
`INSERT`) and needed a **full clean re-seed** of all four tables (not just
spot price) — see "Regenerating the fixtures shifts every dataset's
specific values" above; that gotcha applies to this change too, and was
re-confirmed hitting it again before remembering to clear old rows first.

## Spot price tab shares the Electricity tab's controls (`periodSelection.svelte.ts`)

The Spot price tab (`SpotPriceChart.svelte`) originally had its own simple
"whole year, optionally compare another whole year" view. Per explicit
instruction ("show same date range selection choices as electricity
consumption and calculate difference based on that with comparison
possible"), it now offers the *exact same* resolution/date-picker/
comparison system as the Electricity tab — not a lookalike, the same
underlying state machine.

**Extraction, not duplication**: the resolution/date-picker/comparison
logic that was originally inline in `ElectricityChart.svelte` (windowing,
positional-bucket counts, production-readiness defaults, the
`canCompare`/single-year gating) moved into a `PeriodSelection` class in
`periodSelection.svelte.ts` — a plain `.svelte.ts` module using Svelte 5's
class-field `$state`/getter pattern (runes work in class fields outside
`.svelte` files too, as long as the file has the `.svelte.ts` extension).
`ElectricityChart` and `SpotPriceChart` each construct their **own**
`PeriodSelection` instance (independent resolution/dates per tab — picking
"hour" on one tab doesn't affect the other) but run identical logic, so a
windowing bug fixed once is fixed for both, and there's no risk of the two
tabs' behavior silently diverging over time. `fetchForWindow` (same file)
resolves a `PeriodWindow` (`'all-years'` or a `{start,end}` range) into
actual rows, so every consumer's fetch function is a one-liner rather than
re-implementing the all-years-vs-range branch.
`ElectricityChart`/`SpotPriceChart` still each own what's specific to them:
phase breakdown and temperature overlay stayed in `ElectricityChart`; cost
comparison stayed in `SpotPriceChart`. Only the shared control/windowing
state moved.

`priceBuckets.ts` mirrors `weatherBuckets.ts`'s avg-vs-actual split for
bucketing the price line, but simpler: spot price is already true 15-min
data (see above), so unlike temperature's hour-stepped-to-quarter handling,
every resolution down to "quarter" is a real average (or, at quarter
resolution, an exact pass-through) of real samples — no faking finer
granularity than exists.

**Cost comparison now covers whatever window is selected, not always a
full year** — `computeCostComparison` in `SpotPriceChart.svelte` sums each
electricity sample's energy × the spot price at that exact timestamp
(`spotCostEur`) and × the flat paid rate (`paidCostEur`), over the
*currently selected* primary window (day/month/year/etc, whatever the
resolution picker resolves to), with a second set of tiles for the compare
window when comparison is on. Verified live at every resolution: annual
totals match what the previous always-annual version produced exactly (a
useful regression check after the rewrite); a single day's cost sensibly
shows single-day-scale numbers (a few euros, not hundreds); paid cost still
≈ window kWh × 0.11 exactly at every resolution (it's a flat rate, so this
must always hold, and did).

**Found via direct user feedback, not testing**: shipped an interim version
showing cost tiles for *both* the primary and (whenever the page-level
compare year was set, which defaults to "on" once 2+ years exist) the
compare year, unconditionally — six tiles by default. Reported back as
"shows the three boxes twice." It wasn't a rendering bug; it was 3+3
deliberate tiles reading as an accidental duplicate because nothing
distinguished the two groups clearly. Fixed by making comparison fully
opt-in (matching the Electricity tab's explicit checkbox) rather than
implicit-whenever-a-compare-year-exists — the same lesson as the
`canCompare` gating below, arrived at from the other direction: comparison
should never be default-on when it isn't yet clear the user knows they're
comparing something.

**A test-harness gotcha worth knowing for future e2e scripts**: both
`ElectricityChart` and `SpotPriceChart` render a `.controls` div
simultaneously — the inactive tab is just `display:none` on its parent,
still present in the DOM. A Puppeteer script doing
`document.querySelector('.controls')` silently grabs whichever tab's
controls happen to come first in the template, not the visible one. Hit
this firsthand verifying the resolution selector "wasn't working" on the
Spot price tab — it worked fine; the test was clicking the Electricity
tab's hidden dropdown. Scope any future control-manipulating script to the
`.tab-content` that lacks the `hidden` class first.

**Every control accounts for early production having partial-year,
single-year data** — instruction was explicit: "there won't be data for
whole year and no comparison data until year 2." Two changes, both in the
shared `PeriodSelection`, so they apply to both tabs identically:
- `GET /api/meta/range` (new) gives the actual earliest/latest data
  timestamps. Date/month pickers default to and bound against this real
  range (`minDate`/`maxDate`/`minMonth`/`maxMonth`) instead of assuming
  `{year}-01-01` to `{year}-12-31` — defaulting to "Dec 31" when the year
  is only half over would show an empty chart by default, which is a bad
  first impression for a genuinely bad reason. `sameMonthInYear`/
  `sameDateInYear` compute the compare picker's default as "same calendar
  position, other year" (clamped for day-count, e.g. Feb 29 -> Feb 28),
  not just "December" again.
- Comparison is gated on `availableYears.length > 1` (`canCompare`)
  everywhere it appears — the checkbox itself, and the global page-level
  "Compare with" selector in `+page.svelte`. Before this, the old inline
  `ElectricityChart` state defaulted `compareYear` to `maxYear` (i.e. the
  *same* year as primary) when only one year existed, which would have
  silently rendered a "comparison" against itself if ever enabled.
  Verified live end-to-end with a genuinely single-year database (a
  throwaway container, not the persistent dev one): both the global and
  per-tab comparison controls disappear entirely, cost comparison shows
  only one window's tiles, and nothing errors.

**Defaults switched from "latest date with data" to "today"** — instruction:
"Date should default to today." Previously every picker's default was
derived from the real data range (`GET /api/meta/range`'s max), which
always happened to be a date with data. `PeriodSelection`'s constructor
(and the equivalent global year/compare-year logic in `+page.svelte`'s
`onMount`) now compute `today`/`todayMonth`/`todayYear` directly and use
those as the default `primaryYear`/`primaryMonth`/`primaryDate`/`year`,
regardless of whether today has any data yet — a monitoring dashboard
should default to "now", not "whenever data last happened to exist".
`availableYears`/`minDate`/`maxDate` bounds are *extended* (not just
defaulted) to always include today, so today's year is always a pickable
option in the year dropdowns even before a single row exists for it.
`compareYear` still defaults to the most recent *other* year that actually
has data (today's year was just added and has none).

This makes "the current year/month/day has zero data" a normal, common
default state rather than a rare edge case only reachable by manual
selection — and that flushed out a real latent bug: `ScatterChart.svelte`
(Weather tab) computed its axis domain via `Math.min(...points)`/
`Math.max(...points)`, which on an empty `points` array is
`Infinity`/`-Infinity` in native JS, cascading into `NaN` domains and `NaN`
tick keys (a Svelte `each_key_volatile`/`each_key_duplicate` runtime
error). Fixed by falling back to an arbitrary `[0, 1]` domain when there
are no points — the same class of bug the `niceMax(Math.max(0, ...))` fix
addressed earlier for the bar/line charts, just not caught there since
those charts happened to get exercised with non-empty defaults first.

**Spot price chart displays c/kWh, not €/MWh** — instruction: "exis
should show price in kWh rather than MWh" (axis), later refined to "Spot
price should be in cents, not euros" — cents/kWh is what a Finnish
household actually reads their contract/spot price in (e.g. "5.6
snt/kWh"), not fractional euros. The API and `priceBuckets.ts` still
return raw €/MWh (that's what the market data is in); `SpotPriceChart.svelte`
divides by 10 at the point where chart points are built (`toPoints`, and
the paid flat-line's `PAID_PRICE_EUR_PER_MWH / 10` — €/MWh -> c/kWh is
÷1000 for €/kWh then ×100 for cents, i.e. ÷10 overall) and passes
`unit="c/kWh"` to `DailyLineChart` — a display-only conversion, matching
the established pattern of "convert at the edge, keep the data layer in
the source unit". `computeCostComparison` (the three cost tiles) is
deliberately untouched: it multiplies price × energy to get a total € cost,
not a per-kWh rate, so €/MWh internally is still correct there (it already
divides by 1000 once, for the MWh→kWh unit conversion in the
multiplication itself). The KPI tile on the main page (`+page.svelte`,
"Average spot price") got the same treatment: `avgSpotPrice` itself is
still computed in €/MWh, divided by 10 only at the point it's formatted
for display (2 decimal places, not 3 — cents don't need euro-scale
precision). Verified live: a year with real data shows a realistic
~3–10 c/kWh curve with the dashed paid line flat at 11 (110 EUR/MWh / 10 =
11, the same flat-rate constant, just in cents now), and the KPI tile
matches (e.g. "5.64 c/kWh").

**A second real bug found during that verification, unrelated to the unit
change**: reproduced by setting a tab's resolution to month/week, enabling
comparison, and picking the *same* year for both Primary and Compare —
`ps.primaryYear === ps.compareYear` gives two chart series the same name,
which is also their Svelte keyed-each key, crashing with
`each_key_duplicate`. This was always latent in `ElectricityChart.svelte`/
`SpotPriceChart.svelte`'s year `<select>`s (both listed every
`availableYears` entry unfiltered), but "default to today" made it far
more likely to actually hit: today's year has no data by design now, so a
user's very first action is often "change Year to the year that's already
the default Compare-with value." Fixed the same way the page-level Year/
Compare-with selectors already avoided this: the compare-year `<select>`
now filters out `ps.primaryYear` from its options
(`availableYears.filter((yr: number) => yr !== ps.primaryYear)`), and the
primary-year `<select>`'s `onchange` clears `ps.compareYear` if it now
equals the new primary year. The equivalent page-level gap — `onYearChange`
in `+page.svelte` set `year` without checking `compareYear` — got the same
`onchange` guard. Verified live: forcing the old collision (global Year
select set to match the existing Compare-with value, and per-tab Primary/
Compare year selects both set to the same year) no longer throws; the
compare option simply isn't offered once it matches the primary selection.

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
