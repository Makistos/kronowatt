<script lang="ts">
	import {
		loadElectricity,
		loadElectricityRange,
		loadSpotPrice,
		loadSpotPriceRange,
		type ElectricityRow,
		type SpotPriceRow
	} from '$lib/api';
	import { PeriodSelection, fetchForWindow } from '$lib/periodSelection.svelte';
	import { avgPriceByYear, avgPriceByMonth, avgPriceByWeek, avgPriceByDay, avgPriceByHour, actualPriceByQuarterHour } from '$lib/priceBuckets';
	import { contractsStore } from '$lib/contractsStore.svelte';
	import { effectiveContractAt, paidRateCPerKWh, computePaidCostEur } from '$lib/contractPricing';
	import { MONTH_KEYS } from '$lib/aggregate';
	import { formatNumber } from '$lib/format';
	import { _ as translate, locale } from 'svelte-i18n';
	import DailyLineChart from './DailyLineChart.svelte';
	import StatTile from './StatTile.svelte';

	let { availableYears, dateRange } = $props<{
		availableYears: number[];
		dateRange: { min: string | null; max: string | null };
	}>();

	const SERIES_COLORS = ['var(--series-1)', 'var(--series-2)'];

	// Own PeriodSelection instance, independent of the Electricity tab's —
	// same controls and behavior, but each tab remembers its own resolution
	// and dates (see periodSelection.svelte.ts).
	// svelte-ignore state_referenced_locally
	const ps = new PeriodSelection(availableYears, dateRange);

	let primaryElectricity = $state<ElectricityRow[]>([]);
	let compareElectricity = $state<ElectricityRow[]>([]);
	let primarySpotPrice = $state<SpotPriceRow[]>([]);
	let compareSpotPrice = $state<SpotPriceRow[]>([]);
	let loading = $state(false);
	let loadError = $state<string | null>(null);

	async function fetchElectricity(isCompare: boolean): Promise<ElectricityRow[]> {
		return fetchForWindow(ps.windowFor(isCompare), availableYears, loadElectricity, loadElectricityRange);
	}
	async function fetchSpotPrice(isCompare: boolean): Promise<SpotPriceRow[]> {
		return fetchForWindow(ps.windowFor(isCompare), availableYears, loadSpotPrice, loadSpotPriceRange);
	}

	$effect(() => {
		const comparing = ps.comparisonEnabled;
		loading = true;
		loadError = null;
		Promise.all([
			fetchElectricity(false),
			fetchSpotPrice(false),
			comparing ? fetchElectricity(true) : Promise.resolve([]),
			comparing ? fetchSpotPrice(true) : Promise.resolve([])
		])
			.then(([pe, ps_, ce, cs]) => {
				primaryElectricity = pe;
				primarySpotPrice = ps_;
				compareElectricity = ce;
				compareSpotPrice = cs;
			})
			.catch((e) => {
				loadError = e instanceof Error ? e.message : String(e);
			})
			.finally(() => {
				loading = false;
			});
	});

	// Cost comparison always covers the *entire* selected window (whatever
	// resolution picked it) as one total, matching each electricity sample
	// to its exact-timestamp spot price — both are true 15-min data (see
	// CLAUDE.md "Spot price really is 15-minute now"). "Paid" cost now comes
	// from whatever contract was actually effective across the window
	// (contractsStore, see CLAUDE.md "Contracts") instead of a hardcoded
	// flat rate.
	function computeCostComparison(elRows: ElectricityRow[], spotRows: SpotPriceRow[]) {
		const priceByTime = new Map<string, number>();
		for (const p of spotRows) priceByTime.set(p.time, p.price_eur_mwh);

		let spotCostEur = 0;
		for (const r of elRows) {
			const price = priceByTime.get(r.time);
			if (price === undefined) continue;
			spotCostEur += (r.energy_kwh * price) / 1000; // EUR/MWh -> EUR/kWh
		}
		const paidCostEur = computePaidCostEur(contractsStore.contracts, elRows, spotRows);
		const differenceEur = paidCostEur === null ? null : paidCostEur - spotCostEur;
		return { spotCostEur, paidCostEur, differenceEur };
	}

	// "No contract configured yet" must read as unknown, not "this is free"
	// — an em dash rather than a fabricated 0 €.
	function formatEurOrDash(v: number | null): string {
		return v === null ? '—' : `${formatNumber(v, 0, $locale ?? 'en')} €`;
	}
	function formatSignedEurOrDash(v: number | null): string {
		if (v === null) return '—';
		return `${v >= 0 ? '+' : ''}${formatNumber(v, 0, $locale ?? 'en')} €`;
	}

	const primaryCost = $derived(computeCostComparison(primaryElectricity, primarySpotPrice));
	const compareCost = $derived(
		ps.comparisonEnabled ? computeCostComparison(compareElectricity, compareSpotPrice) : null
	);

	function priceBucket(rows: SpotPriceRow[]): (number | null)[] {
		switch (ps.resolution) {
			case 'year':
				return avgPriceByYear(rows, availableYears);
			case 'month':
				return avgPriceByMonth(rows);
			case 'week':
				return avgPriceByWeek(rows, ps.weekCount);
			case 'day':
				return avgPriceByDay(rows, ps.dayCount);
			case 'hour':
				return avgPriceByHour(rows);
			case 'quarter':
				return actualPriceByQuarterHour(rows);
		}
	}

	// The chart displays cents/kWh (the unit a Finnish household actually
	// reads their price in, e.g. "5.6 snt/kWh"), not the raw EUR/MWh the
	// market/API deals in — EUR/MWh -> c/kWh is divide by 10 (divide by 1000
	// for EUR/kWh, then multiply by 100 for cents). Display-only conversion;
	// computeCostComparison above still works in the raw EUR/MWh the API
	// returns, unaffected by this.
	function toPoints(buckets: (number | null)[]) {
		return buckets
			.map((v, i) => (v === null ? null : { dayOfYear: i, value: v / 10 }))
			.filter((p): p is { dayOfYear: number; value: number } => p !== null);
	}

	const groups = $derived.by(() => {
		const names = [ps.periodLabel(false, $locale ?? 'en')];
		if (ps.comparisonEnabled) names.push(ps.periodLabel(true, $locale ?? 'en'));
		return names;
	});

	// The paid-price line's rate depends only on the timestamp (which
	// contract was effective, and — for a spot contract — that timestamp's
	// own spot price), not on consumption, so it's built straight from the
	// spot price rows' own timestamps rather than the electricity rows.
	// Emits a synthetic SpotPriceRow (rate re-expressed as EUR/MWh, ×10) so
	// it can flow through the same priceBucket()/toPoints() pipeline as the
	// real price series, unit math included. Rows with no effective
	// contract yet (or a spot contract with no matching spot price) are
	// dropped, same "can't show what we don't have" as everywhere else.
	function paidPriceRows(spotRows: SpotPriceRow[]): SpotPriceRow[] {
		const out: SpotPriceRow[] = [];
		for (const row of spotRows) {
			const contract = effectiveContractAt(contractsStore.contracts, new Date(row.time));
			const rate = paidRateCPerKWh(contract, row.price_eur_mwh);
			if (rate === null) continue;
			out.push({ time: row.time, price_eur_mwh: rate * 10 });
		}
		return out;
	}

	const priceSeries = $derived.by(() => {
		const out: { name: string; color: string; points: { dayOfYear: number; value: number }[]; dashed?: boolean }[] =
			[];
		const primaryPoints = toPoints(priceBucket(primarySpotPrice));
		out.push({ name: groups[0] ?? '', color: SERIES_COLORS[0], points: primaryPoints });
		out.push({
			name: $translate('charts.pricePaidFor', { values: { period: groups[0] ?? '' } }),
			color: SERIES_COLORS[0],
			points: toPoints(priceBucket(paidPriceRows(primarySpotPrice))),
			dashed: true
		});
		if (ps.comparisonEnabled) {
			const comparePoints = toPoints(priceBucket(compareSpotPrice));
			out.push({ name: groups[1] ?? '', color: SERIES_COLORS[1], points: comparePoints });
			out.push({
				name: $translate('charts.pricePaidFor', { values: { period: groups[1] ?? '' } }),
				color: SERIES_COLORS[1],
				points: toPoints(priceBucket(paidPriceRows(compareSpotPrice))),
				dashed: true
			});
		}
		return out;
	});

	const categories = $derived.by((): string[] => {
		switch (ps.resolution) {
			case 'year':
				return availableYears.map(String);
			case 'month':
				return MONTH_KEYS.map((k) => $translate(`months.${k}`));
			case 'week':
				return Array.from({ length: ps.weekCount }, (_, i) =>
					$translate('charts.weekLabel', { values: { n: i + 1 } })
				);
			case 'day':
				return Array.from({ length: ps.dayCount }, (_, i) => String(i + 1));
			case 'hour':
				return Array.from({ length: 24 }, (_, i) => `${String(i).padStart(2, '0')}:00`);
			case 'quarter':
				return Array.from({ length: 96 }, (_, i) => {
					const h = Math.floor(i / 4);
					const m = (i % 4) * 15;
					return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`;
				});
		}
	});
	const xTicks = $derived(categories.map((label, i) => ({ pos: i, label })));
</script>

<div class="controls">
	<label>
		{$translate('charts.resolutionLabel')}
		<select bind:value={ps.resolution}>
			<option value="year">{$translate('charts.resolutionYear')}</option>
			<option value="month">{$translate('charts.resolutionMonth')}</option>
			<option value="week">{$translate('charts.resolutionWeek')}</option>
			<option value="day">{$translate('charts.resolutionDay')}</option>
			<option value="hour">{$translate('charts.resolutionHour')}</option>
			<option value="quarter">{$translate('charts.resolutionQuarter')}</option>
		</select>
	</label>

	{#if ps.pickerKind === 'year'}
		<label>
			{$translate('dashboard.year')}
			<select
				bind:value={ps.primaryYear}
				onchange={() => {
					if (ps.compareYear === ps.primaryYear) ps.compareYear = null;
				}}
			>
				{#each availableYears as y (y)}
					<option value={y}>{y}</option>
				{/each}
			</select>
		</label>
	{:else if ps.pickerKind === 'month'}
		<label>
			{$translate('charts.selectMonth')}
			<input type="month" bind:value={ps.primaryMonth} min={ps.minMonth} max={ps.maxMonth} />
		</label>
	{:else if ps.pickerKind === 'day'}
		<label>
			{$translate('charts.selectDate')}
			<input type="date" bind:value={ps.primaryDate} min={ps.minDate} max={ps.maxDate} />
		</label>
	{/if}

	{#if ps.resolution !== 'year' && ps.canCompare}
		<label class="checkbox">
			<input type="checkbox" bind:checked={ps.comparisonEnabled} />
			{$translate('charts.showComparison')}
		</label>
	{/if}

	{#if ps.comparisonEnabled && ps.resolution !== 'year' && ps.canCompare}
		{#if ps.pickerKind === 'year'}
			<label>
				{$translate('dashboard.compareWith')}
				<select bind:value={ps.compareYear}>
					{#each availableYears.filter((yr: number) => yr !== ps.primaryYear) as y (y)}
						<option value={y}>{y}</option>
					{/each}
				</select>
			</label>
		{:else if ps.pickerKind === 'month'}
			<label>
				{$translate('dashboard.compareWith')}
				<input type="month" bind:value={ps.compareMonth} min={ps.minMonth} max={ps.maxMonth} />
			</label>
		{:else if ps.pickerKind === 'day'}
			<label>
				{$translate('dashboard.compareWith')}
				<input type="date" bind:value={ps.compareDate} min={ps.minDate} max={ps.maxDate} />
			</label>
		{/if}
	{/if}

	{#if loading}<span class="muted">{$translate('dashboard.loading')}</span>{/if}
</div>

{#if loadError}
	<div class="empty-state">
		<p><strong>{$translate('emptyState.heading')}</strong></p>
		<p class="error-detail">{loadError}</p>
	</div>
{:else}
	<div class="cost-comparison">
		<StatTile
			label={$translate('charts.costAtSpotPrice')}
			value="{formatNumber(primaryCost.spotCostEur, 0, $locale ?? 'en')} €"
			sub={groups[0] ?? ''}
		/>
		<StatTile
			label={$translate('charts.costAtPaidPrice')}
			value={formatEurOrDash(primaryCost.paidCostEur)}
			sub={groups[0] ?? ''}
		/>
		<StatTile
			label={$translate('charts.costDifference')}
			value={formatSignedEurOrDash(primaryCost.differenceEur)}
			sub={groups[0] ?? ''}
		/>
		{#if compareCost}
			<StatTile
				label={$translate('charts.costAtSpotPrice')}
				value="{formatNumber(compareCost.spotCostEur, 0, $locale ?? 'en')} €"
				sub={groups[1] ?? ''}
			/>
			<StatTile
				label={$translate('charts.costAtPaidPrice')}
				value={formatEurOrDash(compareCost.paidCostEur)}
				sub={groups[1] ?? ''}
			/>
			<StatTile
				label={$translate('charts.costDifference')}
				value={formatSignedEurOrDash(compareCost.differenceEur)}
				sub={groups[1] ?? ''}
			/>
		{/if}
	</div>

	<DailyLineChart title={$translate('charts.dailySpotPrice')} series={priceSeries} unit="c/kWh" {xTicks} />
{/if}

<style>
	.controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 16px;
		margin-bottom: 16px;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.controls label {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.controls label.checkbox {
		gap: 4px;
	}
	.controls select,
	.controls input {
		font-size: 13px;
		padding: 4px 6px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--surface-1);
		color: var(--text-primary);
	}
	.muted {
		color: var(--text-secondary);
	}
	.cost-comparison {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin-bottom: 16px;
	}
	.empty-state {
		background: var(--surface-1);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 20px;
	}
	.error-detail {
		font-size: 12px;
		color: var(--text-muted);
	}
</style>
