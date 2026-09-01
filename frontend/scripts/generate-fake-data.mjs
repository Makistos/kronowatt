#!/usr/bin/env node
// Generates synthetic time-series fixtures for frontend development —
// there is no backend API yet (spec §43 Step 8 hasn't happened), so these
// are static JSON files the frontend fetches directly during local dev.
// Not meant to resemble the future real API's response shape; just enough
// structure to build and test charts against. Re-run after editing this
// file; output is gitignored, not committed.
//
// Timestamps are generated with Date.UTC() and are only loosely "Finnish
// local time" (no real Europe/Helsinki DST handling) — fine for fake data,
// not for anything that needs to be precise.

import { writeFile, mkdir } from 'node:fs/promises';
import { parseArgs } from 'node:util';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const { values: args } = parseArgs({
	options: {
		'start-year': { type: 'string' },
		'end-year': { type: 'string' },
		'annual-kwh': { type: 'string' },
		'ev-sessions-per-week': { type: 'string' },
		'ev-kwh-per-session': { type: 'string' },
		seed: { type: 'string' },
		'out-dir': { type: 'string' }
	}
});

const now = new Date();
const START_YEAR = Number(args['start-year'] ?? now.getUTCFullYear() - 2);
const END_YEAR = Number(args['end-year'] ?? now.getUTCFullYear() - 1);
const ANNUAL_KWH = Number(args['annual-kwh'] ?? 8000);
const EV_SESSIONS_PER_WEEK = Number(args['ev-sessions-per-week'] ?? 2);
const EV_KWH_PER_SESSION = Number(args['ev-kwh-per-session'] ?? 40);
const SEED = Number(args.seed ?? 42);

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const OUT_DIR = path.resolve(__dirname, args['out-dir'] ?? '../static/fake-data');

// ---- seeded PRNG (mulberry32) so a given seed always reproduces the same
// fixtures — Math.random() can't be seeded. ----
function makeRng(seed) {
	let a = seed >>> 0;
	return function rng() {
		a |= 0;
		a = (a + 0x6d2b79f5) | 0;
		let t = Math.imul(a ^ (a >>> 15), 1 | a);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

function gaussian(rng, mean = 0, stddev = 1) {
	// Box-Muller
	const u1 = Math.max(rng(), 1e-9);
	const u2 = rng();
	const z0 = Math.sqrt(-2 * Math.log(u1)) * Math.cos(2 * Math.PI * u2);
	return mean + z0 * stddev;
}

function clamp(x, min, max) {
	return Math.min(max, Math.max(min, x));
}

function isLeapYear(year) {
	return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}

function daysInYear(year) {
	return isLeapYear(year) ? 366 : 365;
}

function daysInMonth(year, month) {
	return new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
}

// ---- seasonal shape: forces ~50% of annual consumption into Jan-Mar
// (per household requirement), smooth shoulder-season taper otherwise.
// Sums to 1.0.
const MONTH_WEIGHT = [
	0.19, // Jan
	0.165, // Feb
	0.145, // Mar
	0.075, // Apr
	0.045, // May
	0.025, // Jun
	0.02, // Jul
	0.025, // Aug
	0.045, // Sep
	0.075, // Oct
	0.095, // Nov
	0.095 // Dec
];

// ---- synthetic Oulu-ish temperature model: annual cosine + daily/hourly
// noise. Not calibrated against real FMI normals — just plausible enough
// for testing a weather-vs-consumption chart.
const TEMP_MEAN_C = 1.0;
const TEMP_AMPLITUDE_C = 13.0; // Jan ~ -12C, Jul ~ +14C
const TEMP_PEAK_DAY = 197; // mid-July
const DIURNAL_AMPLITUDE_C = 3.0; // warmest ~14:00, coldest ~02:00

function seasonalMeanTemp(dayOfYear, yearLength) {
	return (
		TEMP_MEAN_C +
		TEMP_AMPLITUDE_C * Math.cos((2 * Math.PI * (dayOfYear - TEMP_PEAK_DAY)) / yearLength)
	);
}

// ---- hourly household load shape: modest morning bump, larger evening
// peak, low overnight. Returns 24 weights summing to 1.
function buildHourlyShape() {
	const bump = (hour, center, width, weight) =>
		weight * Math.exp(-((hour - center) ** 2) / (2 * width * width));
	const raw = Array.from({ length: 24 }, (_, h) => 0.02 + bump(h, 8, 1.5, 0.55) + bump(h, 19, 2.2, 0.85));
	const total = raw.reduce((a, b) => a + b, 0);
	return raw.map((v) => v / total);
}
const HOURLY_SHAPE = buildHourlyShape();

// ---- 3-phase split: a real household load is rarely perfectly balanced
// across phases, and which phase carries which appliance shifts over time
// (not a fixed ratio) — so weights get per-quarter-hour noise, then are
// renormalized to sum back to the total exactly.
const PHASE_BASE_WEIGHTS = [0.36, 0.33, 0.31];

function splitPhases(totalKw, rng) {
	const noisy = PHASE_BASE_WEIGHTS.map((w) => Math.max(0.05, w * (1 + gaussian(rng, 0, 0.15))));
	const sum = noisy.reduce((a, b) => a + b, 0);
	return noisy.map((w) => (totalKw * w) / sum);
}

function generateYear(year, rng) {
	const nDays = daysInYear(year);
	const electricity = [];
	const weather = [];

	let dayOfYear = 0;
	for (let month = 0; month < 12; month++) {
		const nDaysInMonth = daysInMonth(year, month);
		const baseDailyKwh = (ANNUAL_KWH * MONTH_WEIGHT[month]) / nDaysInMonth;

		for (let d = 0; d < nDaysInMonth; d++) {
			const date = new Date(Date.UTC(year, month, d + 1));
			const weekday = date.getUTCDay(); // 0 = Sunday
			const isWeekend = weekday === 0 || weekday === 6;

			// Shared per-day random draw ties "colder than the seasonal
			// average" to "more electricity used that day" — a direct
			// causal-ish link for the weather-vs-consumption chart, not
			// just coincidental month alignment.
			const tempNoise = gaussian(rng, 0, 2.5);
			const meanTemp = seasonalMeanTemp(dayOfYear, nDays) + tempNoise;

			const heatingFactor = clamp(1 - 0.015 * tempNoise, 0.85, 1.2);
			const weekendFactor = isWeekend ? 1.05 : 1.0;
			const dailyNoise = clamp(1 + gaussian(rng, 0, 0.08), 0.75, 1.3);
			const dailyKwh = baseDailyKwh * heatingFactor * weekendFactor * dailyNoise;

			// Electricity: 15-minute resolution (96 samples/day), not
			// hourly — the dashboard's hour/15-min chart views drill into a
			// single real day rather than averaging, so the underlying data
			// needs to actually exist at that resolution.
			for (let h = 0; h < 24; h++) {
				const hourShareKwh = dailyKwh * HOURLY_SHAPE[h];
				for (let q = 0; q < 4; q++) {
					const quarterNoise = clamp(1 + gaussian(rng, 0, 0.06), 0.6, 1.6);
					const quarterKwh = (hourShareKwh / 4) * quarterNoise;
					const totalKw = quarterKwh * 4; // energy(kWh) / 0.25h = power(kW)
					const phasesKw = splitPhases(totalKw, rng);
					const time = new Date(Date.UTC(year, month, d + 1, h, q * 15)).toISOString();

					electricity.push({
						time,
						power_kw: Number(totalKw.toFixed(3)),
						energy_kwh: Number(quarterKwh.toFixed(4)),
						phases_kw: phasesKw.map((p) => Number(p.toFixed(3)))
					});
				}
			}

			// Weather stays hourly — only the electricity chart needs finer
			// resolution.
			for (let h = 0; h < 24; h++) {
				const time = new Date(Date.UTC(year, month, d + 1, h)).toISOString();
				const diurnalOffset =
					-DIURNAL_AMPLITUDE_C * Math.cos((2 * Math.PI * (h - 14)) / 24);
				const hourlyTemp = meanTemp + diurnalOffset + gaussian(rng, 0, 0.4);
				weather.push({
					time,
					temperature_c: Number(hourlyTemp.toFixed(1))
				});
			}

			dayOfYear++;
		}
	}

	return { electricity, weather };
}

// ---- spot price: Nordic-ish shape — higher and more volatile in winter,
// evening peak / cheap or negative summer nights.
const SPOT_MONTH_BASE_EUR_MWH = [95, 90, 75, 55, 40, 30, 25, 30, 40, 55, 70, 90];

function generateSpotPriceYear(year, rng) {
	const nDays = daysInYear(year);
	const prices = [];

	for (let doy = 0; doy < nDays; doy++) {
		const date = new Date(Date.UTC(year, 0, doy + 1));
		const month = date.getUTCMonth();
		const base = SPOT_MONTH_BASE_EUR_MWH[month];

		for (let h = 0; h < 24; h++) {
			let hourMultiplier = 1.0;
			if (h >= 17 && h <= 20) hourMultiplier = 1.3; // evening peak
			else if (h >= 0 && h <= 5) hourMultiplier = 0.7; // night trough

			// Real Nordic spot prices moved to 15-minute settlement — this
			// matches that (and matches electricity's own 15-min
			// resolution, which the cost-comparison calculation depends on
			// lining up exactly). A price-spike event is decided once per
			// hour (a grid-stress event doesn't flicker quarter to
			// quarter), everything else varies per quarter-hour for
			// texture.
			const isSummer = month >= 5 && month <= 7;
			const isNight = h >= 1 && h <= 4;
			const isWinterEveningSpike = (month <= 1 || month === 11) && h >= 17 && h <= 20 && rng() < 0.05;
			const spikeMultiplier = isWinterEveningSpike ? 2 + rng() * 2 : 1;

			for (let q = 0; q < 4; q++) {
				let price =
					base * hourMultiplier * spikeMultiplier * clamp(1 + gaussian(rng, 0, 0.25), 0.2, 2.5);

				// occasional summer-night negative/near-zero prices (high wind, low demand)
				if (isSummer && isNight && rng() < 0.12) {
					price = -rng() * 15;
				}

				prices.push({
					time: new Date(Date.UTC(year, 0, doy + 1, h, q * 15)).toISOString(),
					price_eur_mwh: Number(price.toFixed(2))
				});
			}
		}
	}

	return prices;
}

// ---- EV sessions: ~N per week, ~configured kWh each, evening-start
// charging, duration derived from energy / average power.
function generateEvSessionsYear(year, rng) {
	const nDays = daysInYear(year);
	const sessions = [];
	const intervalDays = 7 / EV_SESSIONS_PER_WEEK;

	let dayCursor = gaussian(rng, intervalDays / 2, intervalDays / 4);
	while (dayCursor < nDays) {
		const day = Math.floor(clamp(dayCursor, 0, nDays - 1));
		const date = new Date(Date.UTC(year, 0, day + 1));

		const energyKwh = clamp(gaussian(rng, EV_KWH_PER_SESSION, EV_KWH_PER_SESSION * 0.15), 10, 80);
		const averagePowerKw = clamp(gaussian(rng, 7, 1), 3, 11);
		const maximumPowerKw = averagePowerKw * (1.05 + rng() * 0.1);
		const durationHours = energyKwh / averagePowerKw;

		const startHour = clamp(gaussian(rng, 19, 2), 6, 23);
		const startTime = new Date(
			Date.UTC(year, date.getUTCMonth(), date.getUTCDate(), Math.floor(startHour), Math.round((startHour % 1) * 60))
		);
		const endTime = new Date(startTime.getTime() + durationHours * 3600_000);

		sessions.push({
			start_time: startTime.toISOString(),
			end_time: endTime.toISOString(),
			energy_kwh: Number(energyKwh.toFixed(2)),
			average_power_kw: Number(averagePowerKw.toFixed(2)),
			maximum_power_kw: Number(maximumPowerKw.toFixed(2)),
			source: 'defa_cloud'
		});

		// advance by the target interval with jitter; occasionally skip
		// (travel, not home) for realism.
		dayCursor += intervalDays * clamp(1 + gaussian(rng, 0, 0.3), 0.4, 2.2);
		if (rng() < 0.05) dayCursor += intervalDays; // skip a cycle
	}

	return sessions;
}

async function main() {
	await mkdir(OUT_DIR, { recursive: true });
	const rng = makeRng(SEED);
	const years = [];

	for (let year = START_YEAR; year <= END_YEAR; year++) {
		years.push(year);
		const { electricity, weather } = generateYear(year, rng);
		const spotPrice = generateSpotPriceYear(year, rng);
		const evSessions = generateEvSessionsYear(year, rng);

		await writeFile(
			path.join(OUT_DIR, `electricity_${year}.json`),
			JSON.stringify(electricity)
		);
		await writeFile(path.join(OUT_DIR, `weather_${year}.json`), JSON.stringify(weather));
		await writeFile(
			path.join(OUT_DIR, `spot_price_${year}.json`),
			JSON.stringify(spotPrice)
		);
		await writeFile(
			path.join(OUT_DIR, `ev_sessions_${year}.json`),
			JSON.stringify(evSessions)
		);

		const totalKwh = electricity.reduce((sum, r) => sum + r.energy_kwh, 0);
		const evKwh = evSessions.reduce((sum, s) => sum + s.energy_kwh, 0);
		console.log(
			`${year}: electricity ${totalKwh.toFixed(0)} kWh (target ${ANNUAL_KWH}), ` +
				`${evSessions.length} EV sessions totalling ${evKwh.toFixed(0)} kWh`
		);
	}

	await writeFile(
		path.join(OUT_DIR, 'manifest.json'),
		JSON.stringify(
			{
				generatedAt: new Date().toISOString(),
				years,
				params: {
					annualKwh: ANNUAL_KWH,
					evSessionsPerWeek: EV_SESSIONS_PER_WEEK,
					evKwhPerSession: EV_KWH_PER_SESSION,
					seed: SEED
				},
				files: {
					electricity: 'electricity_{year}.json',
					weather: 'weather_{year}.json',
					spotPrice: 'spot_price_{year}.json',
					evSessions: 'ev_sessions_{year}.json'
				}
			},
			null,
			2
		)
	);

	console.log(`\nWrote fixtures for ${years.join(', ')} to ${OUT_DIR}`);
}

main();
