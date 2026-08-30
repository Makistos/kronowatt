# Kronowatt — Full Implementation Specification

Version: 1.1 (revised)
Status: Implementation-ready specification
Target: AI coding agent (Claude Code)
Supersedes: v1.0 — changes are marked **[v1.1]**

---

## 1. Purpose

Build a local-first home energy monitoring and analysis system.

The system continuously collects electricity measurements from a Cozify HAN
interface, weather observations and forecasts from the Finnish Meteorological
Institute (FMI), Finnish electricity spot prices, the user's electricity
contract data, and — as a best-effort exception to local-first — EV charging
data from a Defa Power charger via Defa's unofficial cloud API.

The primary purpose is to build a reliable long-term personal dataset and
provide useful analysis of:

- household electricity consumption
- instantaneous power
- electricity cost
- fixed-contract vs spot-price economics
- EV charging consumption and cost
- weather vs electricity consumption
- consumption patterns
- historical contract simulations
- later, electricity-consumption and cost forecasting

**[v1.1]** The system runs on a Fujitsu Esprimo Q510 with a 30GB SSD (22GB
free at deployment), Debian 13, dedicated to this service only. All external
API access (FMI, spot price provider, Defa cloud) requires outbound internet;
"LAN only" means no inbound remote access / no exposed UI beyond the LAN, not
an offline box.

The implementation should prioritize data integrity, extensibility,
observability, and simple operation over premature optimization — and, given
the disk budget, over storing more than is useful.

---

# 2. Core principles

## 2.1 Local first
All collected data is stored locally. External services are data sources,
not the primary database. The system continues to work when an external API
is temporarily unavailable.

**[v1.1] Exception:** the EV collector depends entirely on Defa's cloud API
(no local interface exists — see §7). It must never block or degrade any
other part of the system when unavailable.

## 2.2 Raw data preservation
Never discard useful raw information merely because it is not currently used
by the UI. Collectors preserve source-specific information while also
normalizing it into domain measurements.

**[v1.1]** This is now *storage-budget-bounded*, not unconditional — see §9a.
Raw payload retention is time-limited by default, not permanent.

## 2.3 Time-series first
Timestamps and intervals are first-class concepts across electricity, EV,
weather observations/forecasts, spot prices, contract prices, and derived
costs.

## 2.4 Historical correctness
Historical calculations use the contract and price rules valid at the
relevant time. Adding or modifying a current contract must never silently
alter historical results.

## 2.5 External-service independence
External providers are hidden behind collector/provider interfaces:

```
Electricity: Cozify HAN -> ElectricityProvider -> internal model
Weather:     FMI -> WeatherProvider -> internal model
Spot prices: provider -> SpotPriceProvider -> internal model
EV:          Defa cloud -> EVProvider -> internal model
```

Do not spread provider-specific API structures throughout the application.

## 2.6 Idempotent collection
Running a collector twice for the same source data must not create duplicate
logical measurements. Use source identifiers and/or deterministic uniqueness
constraints.

## 2.7 Explainable calculations
Cost and analysis calculations are deterministic and explainable. Avoid
machine learning in the initial implementation.

---

# 3. Electricity data source — Cozify HAN

**[v1.1] Verified against the published OpenAPI spec** (cozify.github.io/han-firmware/han-1.0.html). No protocol-discovery spike is needed before implementation; field-name confirmation against a real device response is still worthwhile before finalizing the DB schema.

## 3.1 Interface

The HAN reader exposes REST + WebSocket (`/ws`) + SSE (`/sse`) on its local
IP. No authentication is required for reading meter/state/history data
(bearer JWT is only needed for firmware/OTA write operations).

**Primary collection path: WebSocket (`/ws`).**
On connect, the reader sends an initial state snapshot, then streams live
updates as `HAN_METER_MESSAGE` events. Live meter push appears to correspond
to the device's internal ~10-second sampling period.

**Gap recovery: `/history/*` endpoints.**
The reader keeps its own rolling on-device history — this was missing from
v1.0 and is a significant advantage:

| Endpoint | Resolution | Approx window |
|---|---|---|
| `/history/hourly` | 10s | ~1 hour |
| `/history/weekly` | 15min | ~1 week |
| `/history/monthly` | daily | ~1 month |
| `/history/yearly` | monthly | ~1 month |

On collector startup or reconnect after downtime, backfill the gap from
these endpoints (in the appropriate resolution for the gap length) rather
than leaving a hole in the timeseries. This directly serves §2.6 idempotency
and §42 offline-behavior requirements.

## 3.2 Meter message fields (from `HAN_METER_MESSAGE` / `/meter`)

Confirmed fields (exact units — W vs kW, Wh vs kWh — must be verified
against a live device; the spec below assumes W/Wh pending confirmation):

```
ts    timestamp (ms)
ic    cumulative imported energy
ec    cumulative exported energy
ric   cumulative reactive imported energy
rec   cumulative reactive exported energy
p[]   instantaneous active power (array: total + up to 3 phases)
pi[]  instantaneous active import power per element
pe[]  instantaneous active export power per element
r[]   instantaneous reactive power
ri[]  reactive import power
re[]  reactive export power
u[]   phase voltages (array of up to 3)
i[]   phase currents (array of up to 3)
```

`/meter/id` additionally provides low-churn meter identity (manufacturer,
device type, meter ID, meter model) — collect once and on change, not per
sample.

`/data` provides a compact snapshot including device-computed rolling energy
totals (`ep.h/d/w/m` = hour/day/week/month) and the device's own idea of
current price — useful as a cross-check against the independently collected
spot-price series, not a replacement for it.

Do not invent fields the device doesn't send. Store whatever the live
response actually contains; treat the field list above as provisional until
confirmed.

## 3.3 Configuration

```yaml
cozify:
  enabled: true
  host: <local IP or hostname of HAN reader>
  ws_path: /ws
  history_backfill_on_reconnect: true
  reconnect_backoff: exponential, max 60s
```

No credentials are required for read access with current firmware; do not
hard-code the device's local IP/hostname — make it configurable, and prefer
mDNS/static-DHCP-reservation discovery over a hard IP if practical.

---

# 4. Weather source — FMI

*(Unchanged from v1.0.)*

## 4.1 Observation station
- Name: Oulu lentoasema, FMISID: 101786 (~64.9350N, 25.3392E)
- User location: 64.943604N, 25.367475E — used for forecasts

## 4.2 Weather observations
Collect when available: air temperature, relative humidity, dew point, air
pressure, wind speed/direction/gust, precipitation, cloud cover, visibility.
Missing one optional parameter must not fail the whole observation. Store
source timestamp separately from local retrieval timestamp.

## 4.3 Weather forecast
FMI point forecast at 64.943604, 25.367475. Store forecast generation time,
target time, provider, model. Never overwrite old forecast versions — needed
later for forecast-accuracy evaluation. No historical backfill required.

## 4.4 Frequency
Observations every ~10–15 min; forecasts several times/day, respecting FMI
rate limits.

---

# 5. Electricity spot prices

*(Unchanged from v1.0.)*

Model as 15-minute intervals, not points: `interval_start, interval_end,
price, currency, unit, source, retrieved_at`. Store future and historical
prices permanently (this series is tiny — see §9a). Distinguish "when the
price applies" from "when the application learned the price."

---

# 6. Electricity contracts

*(Unchanged from v1.0.)*

Manual entry. Fields: supplier, contract name, validity start/end (nullable),
pricing model (`fixed`/`spot`/`hybrid`), energy price, spot margin, monthly
fee, transfer price, taxes, VAT, other fees, notes. Support multiple price
periods per contract. Effective price determined by timestamp. Historical
contract data immutable unless explicitly edited.

---

# 7. EV charger — Defa Power

**[v1.1] Rewritten. No local interface exists for this charger.**

## 7.1 Reality of the interface

- Defa has confirmed there is **no public API**, with no committed timeline.
- The charger supports OCPP 2.0 outbound only, and Defa's consumer app gives
  no way to point it at a custom local CSMS (a workaround via a separate
  installer-only "Defa Setup" app is rumored but unconfirmed and not a
  reliable engineering foundation).
- The only working integration path is the community-maintained
  `ha-defa-power` project, which talks to Defa's **unofficial CloudCharge
  backend API** using phone-number/SMS login or a manually supplied token.
  This API enforces per-token rate limiting and can change or break without
  notice — it is explicitly a reverse-engineered, unsupported interface.

## 7.2 Implications for this system

- The EV collector is **cloud-dependent by necessity**, in direct tension
  with §2.1 (local-first). This is accepted as a scoped exception, not
  extended to any other collector.
- It requires internet access from the Fujitsu box specifically for this
  collector (already true for FMI and spot price, so no new network
  exposure — just note it explicitly rather than assuming "LAN only" means
  no outbound calls).
- It must be **fully optional and isolated**: if the Defa API is down,
  rate-limited, or the token needs re-authentication, nothing else in the
  system may be affected. Health status must clearly show "EV: degraded /
  needs re-auth" rather than a generic error.
- Read-only monitoring only. No charge control in this version (was already
  true in v1.0 — reinforced here since the only realistic control path is
  OCPP, which isn't reachable anyway).
- Poll interval should be conservative (e.g. once per 1–5 minutes, backing
  off further when idle/not charging) to respect the undocumented rate
  limit — treat it as fragile, not as a robust API.

## 7.3 Data model

Collect what the CloudCharge API exposes (verify against `ha-defa-power`'s
actual entities at implementation time — do not assume parity with the
list below until confirmed):

```
EVChargingSession
    id
    start_time
    end_time
    energy_kwh
    average_power_kw
    maximum_power_kw
    source            -- "defa_cloud"
```

```
ev_measurement
    id
    timestamp
    power_kw
    charging (bool)
    raw_payload         -- never store the auth token here
    created_at
```

## 7.4 Authentication storage

Store the CloudCharge token/credentials the same way as any other secret
(§32/§33) — never in `raw_payload`, never committed to the repo. Expect to
need a manual re-auth flow when the token expires or the community project's
auth method changes.

---

# 8. Optional future external data sources

*(Unchanged from v1.0 — Fingrid, solar/daylight, carbon intensity. Not
required for MVP.)*

---

# 9. Technology stack

## Backend
Go. Standard HTTP stack, PostgreSQL/pgx driver, migration framework,
structured logging, `context.Context`, standard testing tools. Avoid
unnecessary frameworks. Compiles to a single static binary — built on the
dev machine, copied to the Fujitsu box as an artifact.

## Database
**TimescaleDB** (per user decision) on PostgreSQL. **[v1.1] Native
compression is mandatory, not optional** — enable a compression policy on
hypertables (e.g. compress chunks older than 3–7 days) from the very first
migration. Given the 22GB budget, compression is what keeps 5+ years of data
comfortably on-disk (see §9a).

## Frontend
SvelteKit + TypeScript, prebuilt to static assets on the dev machine, served
by a small static file server on the Fujitsu box. Consumes the backend API
only.

## Deployment — **[v1.1] revised: hybrid, not "everything in Docker"**

Given the 30GB disk and the dev-machine-builds/box-just-runs workflow, full
containerization of every component adds image-layer overhead for no real
benefit here — the backend and frontend are already single deployable
artifacts once built elsewhere.

**Recommended split:**

| Component | Runtime | Why |
|---|---|---|
| TimescaleDB | Docker container | Official image simplifies extension version management, upgrades, and gives you a known-good `pg_dump`/`pg_basebackup` toolchain without hand-installing Postgres + extensions on bare Debian |
| Go backend | systemd service (static binary) | Nothing to containerize — it's already a single portable artifact; systemd gives restart-on-failure and journald logging for free |
| Frontend static assets | systemd service (small static file server, or served by the backend itself) | Same reasoning — no build step happens on-target |
| Reverse proxy | Optional, systemd (Caddy or nginx) if you want TLS/single-port access on the LAN | Skip if you're fine hitting backend/frontend ports directly on the LAN |

This keeps Docker's footprint to one image (TimescaleDB, ~300–400MB) instead
of three-plus, while still getting Docker's real benefit (painless DB
upgrades/rollback) where it matters most. If you'd rather keep everything
uniform for operational simplicity, full Docker Compose is still viable
space-wise (see §9a) — the recommendation above is an optimization, not a
hard requirement.

---

# 9a. Storage budget **[v1.1 — new section]**

Fujitsu Esprimo Q510, 30GB SSD, 22GB free at deployment, this service only.

## Fixed overhead (rough)
```
Debian 13 minimal server install     ~1.5–2.5 GB
Docker engine (DB container only)    ~0.5–1 GB
TimescaleDB image                    ~0.3–0.4 GB
Backend binary + frontend assets     ~0.1–0.2 GB
------------------------------------------------
Fixed footprint                      ~2.5–4 GB
```
Leaves roughly **18–19 GB** for data growth, WAL, indexes, and vacuum
overhead.

## Data growth estimate

| Source | Frequency | Est. size/year (uncompressed) |
|---|---|---|
| Electricity (Cozify, ~10s) | ~8,600 samples/day | ~1–1.5 GB |
| Weather observations | ~10–15 min | ~20–30 MB |
| Weather forecasts | several/day, capped retention | ~20–30 MB (bounded by retention policy, §39) |
| Spot prices | 15 min | ~10 MB |
| EV (Defa) | sparse, charging-only | tens of MB |

Electricity dominates and is the only one worth budgeting carefully. With
TimescaleDB compression enabled on chunks older than a few days, expect
roughly **150–400 MB/year** for electricity instead of 1–1.5GB — numeric
time-series data compresses very well.

## Bottom line

- **1 year:** trivial, well under 5% of available space either compressed
  or not.
- **5 years:** comfortable — roughly 1–3GB with compression on, still under
  10GB even in a pessimistic uncompressed scenario. WAL/vacuum overhead adds
  some margin on top; monitor actual growth after the first month and adjust
  the compression/retention policy rather than guessing further.
- Compression is what turns "should be fine" into "will be fine" — treat
  enabling it as part of MVP, not a later optimization.

## Retention/compression policy (replaces v1.0 §39's unconditional "permanent")

```
Electricity, spot prices, contracts: permanent, compressed after ~7 days
Weather observations:                permanent, compressed after ~7 days
Weather forecasts:                   1–2 years, then dropped (unchanged)
Raw payloads:                        configurable cap (default: keep, but
                                      revisit if disk usage crosses ~70%)
Logs:                                7 days, size-capped
```

Add a simple disk-usage check to the health endpoint (§35) that flags
"degraded" above ~80% disk usage so this is visible before it becomes a
problem, not after.

---

# 10. Repository structure

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
│   ├── docker-compose.yml        -- TimescaleDB only, per §9
│   ├── kronowatt-backend.service
│   ├── kronowatt-frontend.service
│   └── deploy.sh                 -- rsync/scp build artifacts to the box
├── docs/
└── README.md
```

---

# 11. Domain model

*(Unchanged from v1.0 except EV, see §7.3.)*

Includes: `collector`, `electricity_measurement` (with raw_payload JSONB,
only populate fields the source actually provides — see §3.2 for the real
field list), `ev_measurement`, `ev_charging_session`, `weather_observation`,
`weather_forecast`, `spot_price`, `electricity_contract`,
`contract_price_period`. Use explicit validity intervals throughout; use a
uniqueness rule per source to prevent duplicate logical records.

---

# 12. Raw payload handling

JSONB for structured source payloads. Never store API passwords, auth
tokens, or private keys inside raw payloads (this applies with extra force
to the Defa cloud token, §7.4).

---

# 13–38. Time handling, normalization, collection architecture, collector behavior, data quality, missing data, cost engine, contract-vs-spot simulation, consumption-weighted spot price, weather correlation, heating degree calculation, REST API, API date-range handling, frontend MVP, frontend design principles, dashboard examples, forecasting (future), EV optimization (future), Fingrid (future), auth/security, configuration, logging, monitoring, migrations, testing, historical contract behavior test

*(Unchanged from v1.0.)* These sections were not affected by the hardware,
API-verification, or EV-reality findings in this revision. Carry them
forward as-is from the v1.0 document.

One addition to **§35 Monitoring**: include disk usage in the health
response (see §9a) alongside per-collector status.

One addition to **§37 Testing / Collector tests**: use recorded fixtures for
Cozify WebSocket messages and `/history/*` responses (real field names, once
confirmed against a live device), and for Defa CloudCharge responses (via
the `ha-defa-power` project's schema as a starting reference, since there's
no official documentation).

---

# 39. Data retention

**[v1.1] Superseded by §9a.** Use the retention/compression table in §9a
instead of unconditional permanence.

---

# 40. Backups

**[v1.1] Updated — off-box only, matches user's actual setup.**

- Primary: scheduled `pg_dump` (or `pg_basebackup` for full physical backups)
  pushed to the development rig over the LAN.
- Secondary: same backup set synced to the off-site server.
- **No long-term backup retention on the Fujitsu box itself** — the 22GB
  budget is for live data, not a backup archive. A local backup directory is
  fine as a short-lived staging area (e.g. most recent backup only) before
  it's pushed off-box, not as a retention store.
- Document and periodically test the restore procedure on the dev rig, not
  just on the Fujitsu box — a backup that has never been restored elsewhere
  isn't validated.

---

# 41. Data export

*(Unchanged from v1.0.)* CSV minimum, for electricity, EV, weather, prices,
costs. JSON/Parquet optional later.

---

# 42. Offline behavior

*(Unchanged from v1.0, reinforced by §3.1's history-backfill capability.)*
If internet fails: dashboard and stored data remain available, collectors
retry later, nothing is fabricated, collector health shows the failure. If
Cozify is temporarily unreachable, weather and prices keep collecting
independently. On Cozify reconnect, backfill the gap from `/history/*`
before resuming live streaming.

---

# 43. Initial implementation sequence

**[v1.1] Step 0 added; Step 3 and Step 9 updated to reflect verified APIs.**

## Step 0 — Environment & storage policy **[new]**
- Provision Debian 13 on the Fujitsu box, Docker engine, systemd unit
  skeletons.
- Write the actual compression/retention policy (§9a) into the TimescaleDB
  migrations from the start — don't defer it.
- Set up the deploy script (build on dev machine → rsync/scp to box →
  restart systemd services).
- Set up the off-box backup destinations (dev rig + offsite) before any real
  data exists, so backups are exercised from day one.

Deliverable: empty application builds locally, deploys to the box, systemd
services start, TimescaleDB container runs with compression policy applied.

## Step 1 — Repository and build
Monorepo, Go backend, SvelteKit frontend, Docker Compose (DB only per §9),
migrations, CI/test commands.

## Step 2 — Database
Migrations, core tables, hypertables + compression policy, indexes,
uniqueness constraints, repository layer.

## Step 3 — Cozify **[updated]**
Interface is verified (§3) — implement directly against the documented
`/ws` stream and `/history/*` backfill, confirming exact field units against
a live device response before finalizing column types. Connection, parsing,
normalization, storage, reconnect + history backfill, tests.

Deliverable: real electricity measurements collected continuously, gaps
backfilled from device history on reconnect.

## Step 4 — FMI
Observation collector (station 101786), exact-location forecast,
normalization, storage, forecast-version preservation.

## Step 5 — Spot price
15-minute intervals, historical persistence from install date, future
prices, duplicate prevention.

## Step 6 — Contracts
Contract schema, price periods, CRUD API, UI.

## Step 7 — Cost engine
Actual contract cost, spot equivalent, total cost, comparison, weighted
price.

## Step 8 — Frontend MVP
Dashboard, electricity charts, price charts, weather, costs, contracts,
collector health (incl. disk usage, §9a).

## Step 9 — EV **[updated]**
Implement against Defa's CloudCharge cloud API, using `ha-defa-power`'s
reverse-engineered schema as the starting reference (no official docs
exist). Isolate fully per §7.2 — a broken/rate-limited/expired-token EV
collector must never affect anything else. Build the re-auth flow expecting
it will eventually be needed.

Deliverable: EV charging sessions and costs, clearly marked as best-effort.

## Step 10 — Analysis
Temperature correlation, heating degree days, consumption baseline,
contract simulations.

## Step 11 — Fingrid (optional)

## Step 12 — Forecasting/optimization
Only after sufficient real data exists.

---

# 44. MVP acceptance criteria

*(v1.0 criteria 1–15 unchanged, plus:)*

16. **[v1.1]** TimescaleDB compression policy is active and verified (check
    a compressed chunk exists after the policy's age threshold passes).
17. **[v1.1]** Disk usage is visible in the health endpoint and crosses into
    "degraded" status above the configured threshold.
18. **[v1.1]** Off-box backups (dev rig + offsite) have been executed and
    at least one restore has been tested on a machine other than the
    Fujitsu box.
19. **[v1.1]** Cozify reconnect correctly backfills a simulated gap from
    `/history/*` with no duplicate rows.

---

# 45. Non-goals for MVP

*(Unchanged from v1.0)*: cloud hosting, mobile apps, user registration,
multi-household support, ML forecasting, automatic EV control,
appliance-level disaggregation, Ruuvi/Bluetooth sensors, historical weather
backfill, automatic contract discovery/switching, sophisticated carbon
accounting.

---

# 46. Future analytical capabilities

*(Unchanged from v1.0.)*

---

# 47. Most important implementation rule

*(Unchanged from v1.0.)*

> **Create a reliable, timestamped, locally stored history of the user's
> electricity consumption and the external factors that explain its cost
> and variation.**

**[v1.1] addendum:** on a 30GB disk, "reliable and locally stored" also
means *sized responsibly*. Enabling TimescaleDB compression and a sane
retention policy from the first migration is part of data integrity here,
not a tradeoff against it — an application that fills its disk and stops
collecting has failed this rule just as much as one that silently drops
data.
