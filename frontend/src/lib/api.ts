// Talks to the real backend (internal/api) instead of the static fake-data
// fixtures — see CLAUDE.md "Frontend dashboard" for why fakeData.ts existed
// and why this replaces it. The backend DTOs are richer/differently-shaped
// than the old fixtures (e.g. electricity exposes raw ic/p fields); this
// module is where that gets adapted back to the simple {time, value} shapes
// the dashboard's aggregation code already expects, so +page.svelte and
// aggregate.ts didn't need to change.

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

// energy_kwh/phase_energy_kwh are derived client-side from power_kw
// assuming RAW_SAMPLE_INTERVAL_HOURS — see the constant below for why.
export type ElectricityRow = {
	time: string;
	power_kw: number;
	energy_kwh: number;
	phases_kw?: number[];
	phase_energy_kwh?: number[];
};

// The seeded fake data is 15-minute samples, so power(kW) * 0.25h =
// energy(kWh) for one raw sample. This is NOT a general truth — real
// Cozify samples land every ~10s — but the API deliberately doesn't send
// energy (it can't know the interval; see electricityMeasurementDTO's Go
// comment), so something has to assume it, and this is the one place that
// does. Revisit when real Cozify data exists at a different interval.
const RAW_SAMPLE_INTERVAL_HOURS = 0.25;
export type WeatherRow = { time: string; temperature_c: number };
export type SpotPriceRow = { time: string; price_eur_mwh: number };
export type EvSession = {
	start_time: string;
	end_time: string;
	energy_kwh: number;
	average_power_kw: number;
	maximum_power_kw: number;
	source: string;
};

async function fetchJson<T>(path: string): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`);
	if (!res.ok) {
		throw new Error(`fetch ${path} failed: ${res.status}`);
	}
	return (await res.json()) as T;
}

async function sendJson<T>(method: 'POST' | 'PUT', path: string, body: unknown): Promise<T> {
	const res = await fetch(`${API_BASE}${path}`, {
		method,
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) {
		throw new Error((await res.text()) || `${method} ${path} failed: ${res.status}`);
	}
	return (await res.json()) as T;
}

export async function loadYears(): Promise<number[]> {
	const { years } = await fetchJson<{ years: number[] }>('/api/meta/years');
	return years;
}

export type DateRange = { min: string | null; max: string | null };

/** The actual earliest/latest electricity data — used to default pickers
 * to real data instead of assuming a full calendar year exists (early
 * production will have a partial first year). */
export async function loadDateRange(): Promise<DateRange> {
	return fetchJson<DateRange>('/api/meta/range');
}

type electricityMeasurementDTO = {
	time: string;
	power_kw?: number;
	phases_kw?: number[];
};

function toElectricityRow(r: electricityMeasurementDTO): ElectricityRow {
	const powerKw = r.power_kw!;
	return {
		time: r.time,
		power_kw: powerKw,
		energy_kwh: powerKw * RAW_SAMPLE_INTERVAL_HOURS,
		phases_kw: r.phases_kw,
		phase_energy_kwh: r.phases_kw?.map((p) => p * RAW_SAMPLE_INTERVAL_HOURS)
	};
}

export async function loadElectricity(year: number): Promise<ElectricityRow[]> {
	const rows = await fetchJson<electricityMeasurementDTO[]>(`/api/electricity/measurements?year=${year}`);
	return rows.filter((r) => r.power_kw != null).map(toElectricityRow);
}

/** For windows narrower than a full year (a month, a day) — the drill-down
 * resolutions (day/hour/15-min) fetch only what they need rather than a
 * whole year. start/end are ISO instants, end-exclusive. */
export async function loadElectricityRange(start: string, end: string): Promise<ElectricityRow[]> {
	const rows = await fetchJson<electricityMeasurementDTO[]>(
		`/api/electricity/measurements?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`
	);
	return rows.filter((r) => r.power_kw != null).map(toElectricityRow);
}

type weatherObservationDTO = {
	time: string;
	air_temperature?: number;
};

function toWeatherRow(r: weatherObservationDTO): WeatherRow {
	return { time: r.time, temperature_c: r.air_temperature! };
}

export async function loadWeather(year: number): Promise<WeatherRow[]> {
	const rows = await fetchJson<weatherObservationDTO[]>(`/api/weather/observations?year=${year}`);
	return rows.filter((r) => r.air_temperature != null).map(toWeatherRow);
}

/** Same rationale as loadElectricityRange — narrower-than-a-year windows
 * for the electricity chart's temperature overlay at day/hour/15-min
 * resolution. start/end are ISO instants, end-exclusive. */
export async function loadWeatherRange(start: string, end: string): Promise<WeatherRow[]> {
	const rows = await fetchJson<weatherObservationDTO[]>(
		`/api/weather/observations?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`
	);
	return rows.filter((r) => r.air_temperature != null).map(toWeatherRow);
}

type spotPriceDTO = {
	interval_start: string;
	price: number;
};

function toSpotPriceRow(r: spotPriceDTO): SpotPriceRow {
	return { time: r.interval_start, price_eur_mwh: r.price };
}

export async function loadSpotPrice(year: number): Promise<SpotPriceRow[]> {
	const rows = await fetchJson<spotPriceDTO[]>(`/api/spot-prices?year=${year}`);
	return rows.map(toSpotPriceRow);
}

/** Same rationale as loadElectricityRange/loadWeatherRange — the Spot
 * price tab shares the Electricity tab's resolution/date-picker system, so
 * it needs the same narrower-than-a-year windows. */
export async function loadSpotPriceRange(start: string, end: string): Promise<SpotPriceRow[]> {
	const rows = await fetchJson<spotPriceDTO[]>(
		`/api/spot-prices?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`
	);
	return rows.map(toSpotPriceRow);
}

type evSessionDTO = {
	start_time: string;
	end_time?: string;
	energy_kwh?: number;
	average_power_kw?: number;
	maximum_power_kw?: number;
	source: string;
};

// Every price field's unit is spelled out in its own name (c_per_kwh vs
// eur) — unlike spot_price's raw €/MWh market unit, contracts have no
// external wire format to defer to, so the API returns exactly what the
// settings dialog collects and displays (see CLAUDE.md "Contracts").
export type Contract = {
	id: number;
	pricing_model: 'fixed' | 'spot';
	valid_from: string;
	energy_price_c_per_kwh: number | null;
	spot_margin_c_per_kwh: number | null;
	transfer_price_c_per_kwh: number;
	monthly_fee_eur: number;
	electricity_tax_eur: number;
	transfer_tax_eur: number;
};

export type NewContract = Omit<Contract, 'id'>;

/** All contracts, oldest first — small enough to fetch in full and resolve
 * "which contract applies at timestamp T" client-side (see
 * contractPricing.ts), the same way electricity samples are matched to
 * spot price rows. */
export async function loadContracts(): Promise<Contract[]> {
	return fetchJson<Contract[]>('/api/contracts');
}

/** Adds a new contract effective from `valid_from` — never edits an
 * existing one (spec §6: historical contract data is immutable by
 * default). It supersedes whatever was previously effective for any
 * timestamp on or after `valid_from`. */
export async function createContract(contract: NewContract): Promise<Contract> {
	return sendJson<Contract>('POST', '/api/contracts', contract);
}

/** Overwrites an existing contract in place — the explicit-edit exception
 * to spec §6's immutability rule, for "view and change an existing
 * contract" rather than only ever adding a new one. Can change history:
 * editing an old contract changes what applies for every timestamp it
 * used to cover. */
export async function updateContract(id: number, contract: NewContract): Promise<Contract> {
	return sendJson<Contract>('PUT', `/api/contracts/${id}`, contract);
}

// The household's coordinates and the FMI station chosen for weather
// observations (spec §4.1's "User location... used for forecasts",
// extended to also pick the observation station — see CLAUDE.md "Home
// location and nearest-station search").
export type HomeLocation = {
	latitude: number;
	longitude: number;
	station_fmisid: string;
	station_name: string;
};

/** null if never configured — same "null, not 404" convention as
 * loadDateRange, so the settings dialog can tell "not set yet" apart from
 * a request error. */
export async function loadHomeLocation(): Promise<HomeLocation | null> {
	return fetchJson<HomeLocation | null>('/api/home-location');
}

export async function saveHomeLocation(location: HomeLocation): Promise<HomeLocation> {
	return sendJson<HomeLocation>('PUT', '/api/home-location', location);
}

export type NearestStation = {
	fmisid: string;
	name: string;
	latitude: number;
	longitude: number;
	distance_km: number;
};

/** A live lookup against FMI's own station list + current observations
 * (not stored data — see the backend's nearestStationsHandler), so this
 * can be slow-ish (a couple of real HTTP round trips server-side) and can
 * fail if FMI is unreachable; callers should handle both. */
export async function findNearestStations(lat: number, lon: number): Promise<NearestStation[]> {
	return fetchJson<NearestStation[]>(`/api/weather/stations/nearest?lat=${lat}&lon=${lon}`);
}

export async function loadEvSessions(year: number): Promise<EvSession[]> {
	const rows = await fetchJson<evSessionDTO[]>(`/api/ev/sessions?year=${year}`);
	return rows
		.filter((r) => r.end_time != null && r.energy_kwh != null && r.average_power_kw != null && r.maximum_power_kw != null)
		.map((r) => ({
			start_time: r.start_time,
			end_time: r.end_time!,
			energy_kwh: r.energy_kwh!,
			average_power_kw: r.average_power_kw!,
			maximum_power_kw: r.maximum_power_kw!,
			source: r.source
		}));
}
