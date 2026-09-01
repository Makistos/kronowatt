import type { ElectricityRow } from './api';

export type Resolution = 'year' | 'month' | 'week' | 'day' | 'hour' | 'quarter';

// Drill-down window each resolution operates within — matches the UX:
// year has no window (shows everything), month/week show one year,
// day shows one month, hour/quarter (15-min) show one day. Exported so the
// component knows what kind of date picker (none/year/month/day) to show.
export const WINDOW_OF: Record<Resolution, 'none' | 'year' | 'month' | 'day'> = {
	year: 'none',
	month: 'year',
	week: 'year',
	day: 'month',
	hour: 'day',
	quarter: 'day'
};

export type Bucket = { total: number; phases: number[] };

// Buckets are positional (index = position within the period: month 0-11,
// day-of-month 0-based, hour 0-23, quarter-hour 0-95) rather than keyed by
// absolute date. That's deliberate: comparing e.g. March 2025 vs March 2024
// means aligning "day 5" to "day 5", not to a shared calendar date — two
// periods being compared almost never share actual dates.

function emptyBucket(): Bucket {
	return { total: 0, phases: [0, 0, 0] };
}

function add(b: Bucket, row: ElectricityRow) {
	b.total += row.energy_kwh;
	row.phase_energy_kwh?.forEach((v, i) => {
		b.phases[i] = (b.phases[i] ?? 0) + v;
	});
}

/** One bucket per entry in `years`, positional (index into `years`). */
export function bucketByYear(rows: ElectricityRow[], years: number[]): Bucket[] {
	const buckets = years.map(() => emptyBucket());
	const indexOf = new Map(years.map((y, i) => [y, i]));
	for (const row of rows) {
		const i = indexOf.get(new Date(row.time).getUTCFullYear());
		if (i !== undefined) add(buckets[i], row);
	}
	return buckets;
}

/** Always 12 buckets, index = UTC month (0=Jan). */
export function bucketByMonth(rows: ElectricityRow[]): Bucket[] {
	const buckets = Array.from({ length: 12 }, emptyBucket);
	for (const row of rows) {
		add(buckets[new Date(row.time).getUTCMonth()], row);
	}
	return buckets;
}

export function isoWeekNumber(d: Date): number {
	const thursday = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()));
	const dayNum = thursday.getUTCDay() || 7; // Mon=1 .. Sun=7
	thursday.setUTCDate(thursday.getUTCDate() + 4 - dayNum); // -> Thursday of this ISO week
	const yearStart = new Date(Date.UTC(thursday.getUTCFullYear(), 0, 1));
	return Math.ceil(((thursday.getTime() - yearStart.getTime()) / 86_400_000 + 1) / 7);
}

/** Number of ISO-8601 weeks in `year` (52 or 53) — Dec 28 always falls in
 * the year's last ISO week. */
export function isoWeeksInYear(year: number): number {
	return isoWeekNumber(new Date(Date.UTC(year, 11, 28)));
}

/** `weekCount` buckets, index = ISO week number - 1. Caller decides the
 * count (e.g. max across two compared years, so a 53-week year doesn't
 * lose data when compared against a 52-week one). */
export function bucketByWeek(rows: ElectricityRow[], weekCount: number): Bucket[] {
	const buckets = Array.from({ length: weekCount }, emptyBucket);
	for (const row of rows) {
		const w = isoWeekNumber(new Date(row.time));
		if (w >= 1 && w <= weekCount) add(buckets[w - 1], row);
	}
	return buckets;
}

/** Days in `month` (0-based) of `year`. */
export function daysInMonthUTC(year: number, month: number): number {
	return new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
}

/** `dayCount` buckets, index = day-of-month - 1. Caller decides the count
 * (e.g. max across two compared months of different lengths). */
export function bucketByDay(rows: ElectricityRow[], dayCount: number): Bucket[] {
	const buckets = Array.from({ length: dayCount }, emptyBucket);
	for (const row of rows) {
		const day = new Date(row.time).getUTCDate() - 1;
		if (day < dayCount) add(buckets[day], row);
	}
	return buckets;
}

/** Always 24 buckets, index = UTC hour. */
export function bucketByHour(rows: ElectricityRow[]): Bucket[] {
	const buckets = Array.from({ length: 24 }, emptyBucket);
	for (const row of rows) {
		add(buckets[new Date(row.time).getUTCHours()], row);
	}
	return buckets;
}

/** Always 96 buckets (24h * 4) — effectively a pass-through, since raw
 * samples already are 15-minute. */
export function bucketByQuarterHour(rows: ElectricityRow[]): Bucket[] {
	const buckets = Array.from({ length: 96 }, emptyBucket);
	for (const row of rows) {
		const d = new Date(row.time);
		add(buckets[d.getUTCHours() * 4 + Math.floor(d.getUTCMinutes() / 15)], row);
	}
	return buckets;
}
