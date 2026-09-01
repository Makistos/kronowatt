import type { WeatherRow } from './api';
import { isoWeekNumber } from './electricityBuckets';

// Parallels electricityBuckets.ts's positional buckets, but for
// temperature: year/month/week/day resolutions get an *average* (weather
// isn't a quantity that sums meaningfully), while hour/quarter get the
// *actual* reading — weather data is hourly, so at 15-min resolution each
// hour's real reading is stepped across its 4 quarter-buckets rather than
// averaged away, since there's nothing finer to average.

function avg(sums: number[], counts: number[]): (number | null)[] {
	return sums.map((s, i) => (counts[i] > 0 ? s / counts[i] : null));
}

export function avgTempByYear(rows: WeatherRow[], years: number[]): (number | null)[] {
	const sums = years.map(() => 0);
	const counts = years.map(() => 0);
	const indexOf = new Map(years.map((y, i) => [y, i]));
	for (const row of rows) {
		const i = indexOf.get(new Date(row.time).getUTCFullYear());
		if (i !== undefined) {
			sums[i] += row.temperature_c;
			counts[i]++;
		}
	}
	return avg(sums, counts);
}

export function avgTempByMonth(rows: WeatherRow[]): (number | null)[] {
	const sums = Array(12).fill(0);
	const counts = Array(12).fill(0);
	for (const row of rows) {
		const m = new Date(row.time).getUTCMonth();
		sums[m] += row.temperature_c;
		counts[m]++;
	}
	return avg(sums, counts);
}

export function avgTempByWeek(rows: WeatherRow[], weekCount: number): (number | null)[] {
	const sums = Array(weekCount).fill(0);
	const counts = Array(weekCount).fill(0);
	for (const row of rows) {
		const w = isoWeekNumber(new Date(row.time));
		if (w >= 1 && w <= weekCount) {
			sums[w - 1] += row.temperature_c;
			counts[w - 1]++;
		}
	}
	return avg(sums, counts);
}

export function avgTempByDay(rows: WeatherRow[], dayCount: number): (number | null)[] {
	const sums = Array(dayCount).fill(0);
	const counts = Array(dayCount).fill(0);
	for (const row of rows) {
		const day = new Date(row.time).getUTCDate() - 1;
		if (day < dayCount) {
			sums[day] += row.temperature_c;
			counts[day]++;
		}
	}
	return avg(sums, counts);
}

export function actualTempByHour(rows: WeatherRow[]): (number | null)[] {
	const values: (number | null)[] = Array(24).fill(null);
	for (const row of rows) {
		values[new Date(row.time).getUTCHours()] = row.temperature_c;
	}
	return values;
}

export function actualTempByQuarterHour(rows: WeatherRow[]): (number | null)[] {
	const hourly = actualTempByHour(rows);
	const values: (number | null)[] = Array(96).fill(null);
	for (let h = 0; h < 24; h++) {
		for (let q = 0; q < 4; q++) values[h * 4 + q] = hourly[h];
	}
	return values;
}
