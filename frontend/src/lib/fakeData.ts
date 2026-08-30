// Loads the static JSON fixtures from frontend/scripts/generate-fake-data.mjs
// (served as-is from static/fake-data/). Stand-in for the real backend API,
// which doesn't exist yet — see CLAUDE.md.

export type Manifest = {
	generatedAt: string;
	years: number[];
	params: Record<string, number>;
	files: Record<string, string>;
};

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
	const res = await fetch(path);
	if (!res.ok) {
		throw new Error(`fetch ${path} failed: ${res.status}`);
	}
	return (await res.json()) as T;
}

export const loadManifest = (): Promise<Manifest> => fetchJson('/fake-data/manifest.json');

export const loadElectricity = (year: number): Promise<ElectricityRow[]> =>
	fetchJson(`/fake-data/electricity_${year}.json`);

export const loadWeather = (year: number): Promise<WeatherRow[]> =>
	fetchJson(`/fake-data/weather_${year}.json`);

export const loadSpotPrice = (year: number): Promise<SpotPriceRow[]> =>
	fetchJson(`/fake-data/spot_price_${year}.json`);

export const loadEvSessions = (year: number): Promise<EvSession[]> =>
	fetchJson(`/fake-data/ev_sessions_${year}.json`);
