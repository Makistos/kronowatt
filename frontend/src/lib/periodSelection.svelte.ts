import { isoWeeksInYear, daysInMonthUTC, WINDOW_OF, type Resolution } from './electricityBuckets';
import { formatDate, formatMonth } from './format';

export type PeriodWindow = { kind: 'all-years' } | { kind: 'range'; start: string; end: string };

/** Resolves a PeriodWindow into actual rows — 'all-years' fans out into
 * one request per available year (there's no single range that means "all
 * of it"), a range is one direct request. Shared by every chart consuming
 * PeriodSelection so the all-years-vs-range branching lives in one place. */
export async function fetchForWindow<T>(
	window: PeriodWindow | null,
	availableYears: number[],
	loadYear: (year: number) => Promise<T[]>,
	loadRange: (start: string, end: string) => Promise<T[]>
): Promise<T[]> {
	if (!window) return [];
	if (window.kind === 'all-years') {
		const perYear = await Promise.all(availableYears.map(loadYear));
		return perYear.flat();
	}
	return loadRange(window.start, window.end);
}

/** Same calendar month as `month` ("YYYY-MM"), but in `targetYear`. */
function sameMonthInYear(month: string, targetYear: number): string {
	return `${targetYear}-${month.slice(5, 7)}`;
}
/** Same calendar day as `date` ("YYYY-MM-DD"), but in `targetYear`, clamped
 * to that month's actual day count (e.g. Feb 29 -> Feb 28). */
function sameDateInYear(date: string, targetYear: number): string {
	const month = Number(date.slice(5, 7));
	const day = Math.min(Number(date.slice(8, 10)), daysInMonthUTC(targetYear, month - 1));
	return `${targetYear}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

/**
 * Shared resolution/date-picker/comparison state — the drill-down control
 * system originally built for the Electricity tab (year -> month/week ->
 * day -> hour/15-min), extracted so the Spot price tab can offer the exact
 * same choices and comparison behavior rather than a second, divergent
 * implementation. See CLAUDE.md "Electricity tab" for the windowing
 * rationale (WINDOW_OF, positional bucketing, production-readiness
 * defaults) — none of that changed, it just moved here.
 *
 * Constructed once, after `availableYears`/`dateRange` are already loaded
 * (both tabs only mount once `availableYears` is non-null) — the min/max/
 * canCompare fields are computed once at construction, not reactively
 * re-derived from props, since this data doesn't change after initial load.
 */
export class PeriodSelection {
	availableYears: number[];
	minYear: number;
	maxYear: number;
	minDate: string;
	maxDate: string;
	minMonth: string;
	maxMonth: string;
	canCompare: boolean;

	resolution = $state<Resolution>('month');
	comparisonEnabled = $state(false);

	primaryYear = $state<number | null>(null);
	compareYear = $state<number | null>(null);
	primaryMonth = $state<string | null>(null); // "YYYY-MM"
	compareMonth = $state<string | null>(null);
	primaryDate = $state<string | null>(null); // "YYYY-MM-DD"
	compareDate = $state<string | null>(null);

	constructor(availableYears: number[], dateRange: { min: string | null; max: string | null }) {
		// "Today" (UTC, matching every other date computation in this app)
		// drives every default — a monitoring dashboard should default to
		// "now", not "the last date that happened to have data". Bounds are
		// extended (not just defaults) so today is always a valid, pickable
		// value even before any data exists for it yet — a fresh deployment
		// on day 1 should default to today's (empty-so-far) view, not
		// silently fall back to nothing.
		const todayISO = new Date().toISOString();
		const todayDate = todayISO.slice(0, 10);
		const todayMonth = todayDate.slice(0, 7);
		const todayYear = Number(todayDate.slice(0, 4));

		this.availableYears = availableYears.includes(todayYear)
			? availableYears
			: [...availableYears, todayYear].sort((a, b) => a - b);
		this.minYear = Math.min(...this.availableYears);
		this.maxYear = Math.max(...this.availableYears);

		const dataMinDate = dateRange.min ? dateRange.min.slice(0, 10) : `${this.minYear}-01-01`;
		const dataMaxDate = dateRange.max ? dateRange.max.slice(0, 10) : `${this.maxYear}-12-31`;
		this.minDate = dataMinDate < todayDate ? dataMinDate : todayDate;
		this.maxDate = dataMaxDate > todayDate ? dataMaxDate : todayDate;
		this.minMonth = this.minDate.slice(0, 7);
		this.maxMonth = this.maxDate.slice(0, 7);
		this.canCompare = this.availableYears.length > 1;

		this.primaryYear = todayYear;
		this.primaryMonth = todayMonth;
		this.primaryDate = todayDate;

		// Compare against the most recent *other* selectable year — usually
		// the latest year that actually has data, since today's year is
		// only in the list at all because we just added it above when it
		// had none yet.
		const otherYears = this.availableYears.filter((y) => y !== this.primaryYear);
		this.compareYear = this.canCompare && otherYears.length > 0 ? otherYears.at(-1)! : null;
		this.compareMonth = this.compareYear !== null ? sameMonthInYear(todayMonth, this.compareYear) : null;
		this.compareDate = this.compareYear !== null ? sameDateInYear(todayDate, this.compareYear) : null;
	}

	get pickerKind() {
		return WINDOW_OF[this.resolution];
	}

	dateValueFor(isCompare: boolean): string | null {
		const kind = this.pickerKind;
		if (kind === 'year') return String(isCompare ? this.compareYear : this.primaryYear);
		if (kind === 'month') return isCompare ? this.compareMonth : this.primaryMonth;
		if (kind === 'day') return isCompare ? this.compareDate : this.primaryDate;
		return null;
	}

	periodLabel(isCompare: boolean, locale: string): string {
		const dateValue = this.dateValueFor(isCompare);
		if (!dateValue) return '';
		const kind = this.pickerKind;
		if (kind === 'year') return dateValue;
		if (kind === 'month') return formatMonth(dateValue, locale);
		return formatDate(dateValue, locale);
	}

	get weekCount() {
		if (this.resolution !== 'week') return 0;
		return Math.max(
			isoWeeksInYear(this.primaryYear ?? this.maxYear),
			this.comparisonEnabled ? isoWeeksInYear(this.compareYear ?? this.maxYear) : 0
		);
	}

	get dayCount() {
		if (this.resolution !== 'day' || !this.primaryMonth) return 0;
		const monthDayCount = (ym: string) => {
			const [y, m] = ym.split('-').map(Number);
			return daysInMonthUTC(y, m - 1);
		};
		return Math.max(
			monthDayCount(this.primaryMonth),
			this.comparisonEnabled && this.compareMonth ? monthDayCount(this.compareMonth) : 0
		);
	}

	/** The fetch window for the primary or compare period — 'all-years' at
	 * year resolution (there's no single range to fetch), otherwise an
	 * explicit [start, end) ISO range regardless of picker kind (a
	 * month/week-resolution "year" window is just as much a range as a
	 * day-resolution "month" window). */
	windowFor(isCompare: boolean): PeriodWindow | null {
		if (this.resolution === 'year') return { kind: 'all-years' };
		const dateValue = this.dateValueFor(isCompare);
		if (!dateValue) return null;

		const kind = this.pickerKind;
		if (kind === 'year') {
			const y = Number(dateValue);
			return { kind: 'range', start: `${y}-01-01T00:00:00.000Z`, end: `${y + 1}-01-01T00:00:00.000Z` };
		}
		if (kind === 'month') {
			const [y, m] = dateValue.split('-').map(Number);
			return {
				kind: 'range',
				start: new Date(Date.UTC(y, m - 1, 1)).toISOString(),
				end: new Date(Date.UTC(y, m, 1)).toISOString()
			};
		}
		const start = new Date(`${dateValue}T00:00:00.000Z`);
		return { kind: 'range', start: start.toISOString(), end: new Date(start.getTime() + 86_400_000).toISOString() };
	}
}
