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
