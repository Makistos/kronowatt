<script lang="ts">
	import { loadElectricity, loadElectricityRange, loadWeather, loadWeatherRange, type ElectricityRow, type WeatherRow } from '$lib/api';
	import { PeriodSelection, fetchForWindow } from '$lib/periodSelection.svelte';
	import {
		bucketByYear,
		bucketByMonth,
		bucketByWeek,
		bucketByDay,
		bucketByHour,
		bucketByQuarterHour,
		type Bucket
	} from '$lib/electricityBuckets';
	import {
		avgTempByYear,
		avgTempByMonth,
		avgTempByWeek,
		avgTempByDay,
		actualTempByHour,
		actualTempByQuarterHour
	} from '$lib/weatherBuckets';
	import { MONTH_KEYS } from '$lib/aggregate';
	import { _ as translate, locale } from 'svelte-i18n';
	import ElectricityBarChart from './ElectricityBarChart.svelte';
	import DailyLineChart from './DailyLineChart.svelte';

	let { availableYears, dateRange } = $props<{
		availableYears: number[];
		dateRange: { min: string | null; max: string | null };
	}>();

	const SERIES_COLORS = ['var(--series-1)', 'var(--series-2)'];
	const EMPTY_BUCKET: Bucket = { total: 0, phases: [0, 0, 0] };

	// Resolution/date-picker/comparison state — shared with the Spot price
	// tab (SpotPriceChart.svelte) via periodSelection.svelte.ts, so both
	// offer identical controls rather than two diverging implementations.
	// Deliberately captures availableYears/dateRange once, not reactively —
	// both are loaded exactly once before this component ever mounts (see
	// the class doc comment).
	// svelte-ignore state_referenced_locally
	const ps = new PeriodSelection(availableYears, dateRange);

	let phaseBreakdown = $state(false);
	let showTemperature = $state(false);

	async function fetchRows(isCompare: boolean): Promise<ElectricityRow[]> {
		return fetchForWindow(ps.windowFor(isCompare), availableYears, loadElectricity, loadElectricityRange);
	}

	let primaryRows = $state<ElectricityRow[]>([]);
	let compareRows = $state<ElectricityRow[]>([]);
	let loading = $state(false);
	let loadError = $state<string | null>(null);

	$effect(() => {
		const comparing = ps.comparisonEnabled;
		loading = true;
		loadError = null;
		Promise.all([fetchRows(false), comparing ? fetchRows(true) : Promise.resolve([])])
			.then(([p, c]) => {
				primaryRows = p;
				compareRows = c;
			})
			.catch((e) => {
				loadError = e instanceof Error ? e.message : String(e);
			})
			.finally(() => {
				loading = false;
			});
	});

	let temperaturePrimaryRows = $state<WeatherRow[]>([]);
	let temperatureCompareRows = $state<WeatherRow[]>([]);

	async function fetchWeatherRows(isCompare: boolean): Promise<WeatherRow[]> {
		return fetchForWindow(ps.windowFor(isCompare), availableYears, loadWeather, loadWeatherRange);
	}

	$effect(() => {
		if (!showTemperature) return;
		const comparing = ps.comparisonEnabled;
		Promise.all([fetchWeatherRows(false), comparing ? fetchWeatherRows(true) : Promise.resolve([])]).then(
			([p, c]) => {
				temperaturePrimaryRows = p;
				temperatureCompareRows = c;
			}
		);
		// Errors here aren't surfaced separately — if the backend is down,
		// the electricity fetch above already shows the empty-state error;
		// the temperature overlay just stays empty rather than doubling up
		// on error UI for the same underlying failure.
	});

	function bucket(rows: ElectricityRow[]): Bucket[] {
		switch (ps.resolution) {
			case 'year':
				return bucketByYear(rows, availableYears);
			case 'month':
				return bucketByMonth(rows);
			case 'week':
				return bucketByWeek(rows, ps.weekCount);
			case 'day':
				return bucketByDay(rows, ps.dayCount);
			case 'hour':
				return bucketByHour(rows);
			case 'quarter':
				return bucketByQuarterHour(rows);
		}
	}

	const primaryBuckets = $derived(bucket(primaryRows));
	const compareBuckets = $derived(ps.comparisonEnabled ? bucket(compareRows) : []);

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

	const groups = $derived.by(() => {
		const names = [ps.periodLabel(false, $locale ?? 'en')];
		if (ps.comparisonEnabled) names.push(ps.periodLabel(true, $locale ?? 'en'));
		return names.map((name, i) => ({ name, color: SERIES_COLORS[i] }));
	});

	const stackLabels = $derived(
		phaseBreakdown
			? [$translate('charts.phase1'), $translate('charts.phase2'), $translate('charts.phase3')]
			: [$translate('charts.total')]
	);

	function stackValues(b: Bucket): number[] {
		return phaseBreakdown ? b.phases : [b.total];
	}

	const values = $derived(
		categories.map((_, ci) => {
			const row = [stackValues(primaryBuckets[ci] ?? EMPTY_BUCKET)];
			if (ps.comparisonEnabled) row.push(stackValues(compareBuckets[ci] ?? EMPTY_BUCKET));
			return row;
		})
	);

	// Year/month/week/day resolutions average temperature over the bucket
	// period; hour/quarter show the actual reading (see weatherBuckets.ts —
	// weather is hourly data, so there's nothing finer to average away for
	// those two anyway).
	function temperatureBucket(rows: WeatherRow[]): (number | null)[] {
		switch (ps.resolution) {
			case 'year':
				return avgTempByYear(rows, availableYears);
			case 'month':
				return avgTempByMonth(rows);
			case 'week':
				return avgTempByWeek(rows, ps.weekCount);
			case 'day':
				return avgTempByDay(rows, ps.dayCount);
			case 'hour':
				return actualTempByHour(rows);
			case 'quarter':
				return actualTempByQuarterHour(rows);
		}
	}

	function toTemperaturePoints(buckets: (number | null)[]) {
		return buckets
			.map((v, i) => (v === null ? null : { dayOfYear: i, value: v }))
			.filter((p): p is { dayOfYear: number; value: number } => p !== null);
	}

	const temperatureSeries = $derived.by(() => {
		if (!showTemperature) return [];
		const out: { name: string; color: string; points: { dayOfYear: number; value: number }[] }[] = [
			{ name: groups[0]?.name ?? '', color: SERIES_COLORS[0], points: toTemperaturePoints(temperatureBucket(temperaturePrimaryRows)) }
		];
		if (ps.comparisonEnabled) {
			out.push({
				name: groups[1]?.name ?? '',
				color: SERIES_COLORS[1],
				points: toTemperaturePoints(temperatureBucket(temperatureCompareRows))
			});
		}
		return out;
	});

	const temperatureXTicks = $derived(categories.map((label, i) => ({ pos: i, label })));
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

	<label class="checkbox">
		<input type="checkbox" bind:checked={phaseBreakdown} />
		{$translate('charts.showPhases')}
	</label>

	<label class="checkbox">
		<input type="checkbox" bind:checked={showTemperature} />
		{$translate('charts.showTemperature')}
	</label>

	{#if loading}<span class="muted">{$translate('dashboard.loading')}</span>{/if}
</div>

{#if loadError}
	<div class="empty-state">
		<p><strong>{$translate('emptyState.heading')}</strong></p>
		<p class="error-detail">{loadError}</p>
	</div>
{:else}
	<ElectricityBarChart
		title={$translate('charts.electricityConsumption')}
		{categories}
		{groups}
		{stackLabels}
		{values}
		unit="kWh"
	/>

	{#if showTemperature && temperatureSeries.some((s) => s.points.length > 0)}
		<div class="temperature-chart">
			<DailyLineChart
				title={$translate('charts.temperature')}
				series={temperatureSeries}
				unit="°C"
				xTicks={temperatureXTicks}
			/>
		</div>
	{/if}
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
	.temperature-chart {
		margin-top: 16px;
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
