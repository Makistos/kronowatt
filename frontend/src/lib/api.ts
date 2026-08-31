// Talks to the real backend (internal/api) instead of the static fake-data
// fixtures — see CLAUDE.md "Frontend dashboard" for why fakeData.ts existed
// and why this replaces it. The backend DTOs are richer/differently-shaped
// than the old fixtures (e.g. electricity exposes raw ic/p fields); this
// module is where that gets adapted back to the simple {time, value} shapes
// the dashboard's aggregation code already expects, so +page.svelte and
// aggregate.ts didn't need to change.

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080';

export type ElectricityRow = { time: string; power_kw: number; energy_kwh: number };
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

type electricityMeasurementDTO = {
	time: string;
	power_kw?: number;
	energy_kwh?: number;
};

export async function loadElectricity(year: number): Promise<ElectricityRow[]> {
	const rows = await fetchJson<electricityMeasurementDTO[]>(`/api/electricity/measurements?year=${year}`);
	return rows
		.filter((r) => r.power_kw != null && r.energy_kwh != null)
		.map((r) => ({ time: r.time, power_kw: r.power_kw!, energy_kwh: r.energy_kwh! }));
}

type weatherObservationDTO = {
	time: string;
	air_temperature?: number;
};

export async function loadWeather(year: number): Promise<WeatherRow[]> {
	const rows = await fetchJson<weatherObservationDTO[]>(`/api/weather/observations?year=${year}`);
	return rows
		.filter((r) => r.air_temperature != null)
		.map((r) => ({ time: r.time, temperature_c: r.air_temperature! }));
}

type spotPriceDTO = {
	interval_start: string;
	price: number;
};

export async function loadSpotPrice(year: number): Promise<SpotPriceRow[]> {
	const rows = await fetchJson<spotPriceDTO[]>(`/api/spot-prices?year=${year}`);
	return rows.map((r) => ({ time: r.interval_start, price_eur_mwh: r.price }));
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
