import type { SpotPriceRow } from './api';
import { isoWeekNumber } from './electricityBuckets';

// Same positional-bucketing shape as electricityBuckets.ts/weatherBuckets.ts.
// Unlike temperature (hourly source data), spot price is already true
// 15-minute data (see CLAUDE.md "Spot price really is 15-minute now"), so
// every resolution down to "quarter" is a real average of real samples —
// "quarter" needs no stepping/faking, it's a 1:1 pass-through.

function avg(sums: number[], counts: number[]): (number | null)[] {
	return sums.map((s, i) => (counts[i] > 0 ? s / counts[i] : null));
}

export function avgPriceByYear(rows: SpotPriceRow[], years: number[]): (number | null)[] {
	const sums = years.map(() => 0);
	const counts = years.map(() => 0);
	const indexOf = new Map(years.map((y, i) => [y, i]));
	for (const row of rows) {
		const i = indexOf.get(new Date(row.time).getUTCFullYear());
		if (i !== undefined) {
			sums[i] += row.price_eur_mwh;
			counts[i]++;
		}
	}
	return avg(sums, counts);
}

export function avgPriceByMonth(rows: SpotPriceRow[]): (number | null)[] {
	const sums = Array(12).fill(0);
	const counts = Array(12).fill(0);
	for (const row of rows) {
		const m = new Date(row.time).getUTCMonth();
		sums[m] += row.price_eur_mwh;
		counts[m]++;
	}
	return avg(sums, counts);
}

export function avgPriceByWeek(rows: SpotPriceRow[], weekCount: number): (number | null)[] {
	const sums = Array(weekCount).fill(0);
	const counts = Array(weekCount).fill(0);
	for (const row of rows) {
		const w = isoWeekNumber(new Date(row.time));
		if (w >= 1 && w <= weekCount) {
			sums[w - 1] += row.price_eur_mwh;
			counts[w - 1]++;
		}
	}
	return avg(sums, counts);
}

export function avgPriceByDay(rows: SpotPriceRow[], dayCount: number): (number | null)[] {
	const sums = Array(dayCount).fill(0);
	const counts = Array(dayCount).fill(0);
	for (const row of rows) {
		const day = new Date(row.time).getUTCDate() - 1;
		if (day < dayCount) {
			sums[day] += row.price_eur_mwh;
			counts[day]++;
		}
	}
	return avg(sums, counts);
}

export function avgPriceByHour(rows: SpotPriceRow[]): (number | null)[] {
	const sums = Array(24).fill(0);
	const counts = Array(24).fill(0);
	for (const row of rows) {
		const h = new Date(row.time).getUTCHours();
		sums[h] += row.price_eur_mwh;
		counts[h]++;
	}
	return avg(sums, counts);
}

/** Real 15-min samples, one per bucket — not averaged or stepped, unlike
 * temperature's quarter-hour handling, because the source data actually is
 * this fine-grained. */
export function actualPriceByQuarterHour(rows: SpotPriceRow[]): (number | null)[] {
	const values: (number | null)[] = Array(96).fill(null);
	for (const row of rows) {
		const d = new Date(row.time);
		values[d.getUTCHours() * 4 + Math.floor(d.getUTCMinutes() / 15)] = row.price_eur_mwh;
	}
	return values;
}
