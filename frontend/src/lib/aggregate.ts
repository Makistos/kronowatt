// Language-neutral lookup keys (index = UTC month), not display labels —
// resolve to text via i18n ($t(`months.${MONTH_KEYS[i]}`)), never hardcode
// English month names in a component.
export const MONTH_KEYS = [
	'jan',
	'feb',
	'mar',
	'apr',
	'may',
	'jun',
	'jul',
	'aug',
	'sep',
	'oct',
	'nov',
	'dec'
];

/** Sums valueFn(row) into 12 monthly buckets (UTC month of row.time). */
export function monthlySums<T extends { time: string }>(rows: T[], valueFn: (r: T) => number): number[] {
	const out = new Array(12).fill(0);
	for (const r of rows) {
		out[new Date(r.time).getUTCMonth()] += valueFn(r);
	}
	return out;
}

export type DailyPoint = { dayOfYear: number; date: string; value: number };

/** Aggregates hourly rows into one point per UTC calendar day. */
export function dailySeries<T extends { time: string }>(
	rows: T[],
	valueFn: (r: T) => number,
	agg: 'sum' | 'avg' = 'sum'
): DailyPoint[] {
	const sums = new Map<string, number>();
	const counts = new Map<string, number>();
	for (const r of rows) {
		const d = r.time.slice(0, 10);
		sums.set(d, (sums.get(d) ?? 0) + valueFn(r));
		counts.set(d, (counts.get(d) ?? 0) + 1);
	}
	const dates = [...sums.keys()].sort();
	return dates.map((date, i) => ({
		dayOfYear: i,
		date,
		value: agg === 'avg' ? sums.get(date)! / counts.get(date)! : sums.get(date)!
	}));
}

/** Rounds up to a "nice" axis maximum (1/2/5 * 10^n) so gridlines land on clean numbers. */
export function niceMax(value: number): number {
	if (value <= 0) return 1;
	const exponent = Math.floor(Math.log10(value));
	const magnitude = 10 ** exponent;
	const residual = value / magnitude;
	let niceResidual: number;
	if (residual > 5) niceResidual = 10;
	else if (residual > 2) niceResidual = 5;
	else if (residual > 1) niceResidual = 2;
	else niceResidual = 1;
	return niceResidual * magnitude;
}
