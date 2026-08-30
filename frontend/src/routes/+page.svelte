<script lang="ts">
	import { onMount } from 'svelte';
	import {
		loadManifest,
		loadElectricity,
		loadWeather,
		loadSpotPrice,
		loadEvSessions,
		type Manifest,
		type ElectricityRow,
		type WeatherRow,
		type SpotPriceRow,
		type EvSession
	} from '$lib/fakeData';
	import { MONTH_KEYS, monthlySums, dailySeries } from '$lib/aggregate';
	import { formatCompact, formatNumber } from '$lib/format';
	import { _ as translate, locale } from 'svelte-i18n';
	import MonthlyBarChart from '$lib/charts/MonthlyBarChart.svelte';
	import DailyLineChart from '$lib/charts/DailyLineChart.svelte';
	import ScatterChart from '$lib/charts/ScatterChart.svelte';
	import StatTile from '$lib/charts/StatTile.svelte';

	type YearData = {
		electricity: ElectricityRow[];
		weather: WeatherRow[];
		spotPrice: SpotPriceRow[];
		evSessions: EvSession[];
	};

	const SERIES_COLORS = ['var(--series-1)', 'var(--series-2)'];

	let manifest = $state<Manifest | null>(null);
	let manifestError = $state<string | null>(null);
	let year = $state<number | null>(null);
	let compareYear = $state<number | null>(null);

	let cache = $state<Map<number, YearData>>(new Map());
	let loadingYears = $state<Set<number>>(new Set());

	onMount(async () => {
		try {
			manifest = await loadManifest();
			const years = manifest.years;
			year = years.at(-1) ?? null;
			compareYear = years.length > 1 ? years.at(-2)! : null;
		} catch (e) {
			manifestError = e instanceof Error ? e.message : String(e);
		}
	});

	async function ensureYear(y: number) {
		if (cache.has(y) || loadingYears.has(y)) return;
		loadingYears.add(y);
		loadingYears = new Set(loadingYears);
		try {
			const [electricity, weather, spotPrice, evSessions] = await Promise.all([
				loadElectricity(y),
				loadWeather(y),
				loadSpotPrice(y),
				loadEvSessions(y)
			]);
			cache.set(y, { electricity, weather, spotPrice, evSessions });
			cache = new Map(cache);
		} finally {
			loadingYears.delete(y);
			loadingYears = new Set(loadingYears);
		}
	}

	$effect(() => {
		if (year !== null) ensureYear(year);
		if (compareYear !== null) ensureYear(compareYear);
	});

	const primary = $derived(year !== null ? cache.get(year) : undefined);
	const secondary = $derived(compareYear !== null ? cache.get(compareYear) : undefined);
	const isLoading = $derived(
		(year !== null && loadingYears.has(year)) || (compareYear !== null && loadingYears.has(compareYear))
	);

	const totalKwh = $derived(primary ? primary.electricity.reduce((s, r) => s + r.energy_kwh, 0) : 0);
	const avgTemp = $derived(
		primary && primary.weather.length
			? primary.weather.reduce((s, r) => s + r.temperature_c, 0) / primary.weather.length
			: 0
	);
	const evTotalKwh = $derived(primary ? primary.evSessions.reduce((s, e) => s + e.energy_kwh, 0) : 0);
	const evSessionCount = $derived(primary ? primary.evSessions.length : 0);
	const avgSpotPrice = $derived(
		primary && primary.spotPrice.length
			? primary.spotPrice.reduce((s, r) => s + r.price_eur_mwh, 0) / primary.spotPrice.length
			: 0
	);

	const consumptionSeries = $derived.by(() => {
		const out: { name: string; color: string; values: number[] }[] = [];
		if (primary) out.push({ name: String(year), color: SERIES_COLORS[0], values: monthlySums(primary.electricity, (r) => r.energy_kwh) });
		if (secondary) out.push({ name: String(compareYear), color: SERIES_COLORS[1], values: monthlySums(secondary.electricity, (r) => r.energy_kwh) });
		return out;
	});

	function toDailyLine(rows: ElectricityRow[]) {
		return dailySeries(rows, (r) => r.energy_kwh, 'sum');
	}
	const electricityDailySeries = $derived.by(() => {
		const out: { name: string; color: string; points: { dayOfYear: number; value: number; date: string }[] }[] = [];
		if (primary) out.push({ name: String(year), color: SERIES_COLORS[0], points: toDailyLine(primary.electricity) });
		if (secondary) out.push({ name: String(compareYear), color: SERIES_COLORS[1], points: toDailyLine(secondary.electricity) });
		return out;
	});

	function toDailyPrice(rows: SpotPriceRow[]) {
		return dailySeries(rows, (r) => r.price_eur_mwh, 'avg');
	}
	const spotPriceDailySeries = $derived.by(() => {
		const out: { name: string; color: string; points: { dayOfYear: number; value: number; date: string }[] }[] = [];
		if (primary) out.push({ name: String(year), color: SERIES_COLORS[0], points: toDailyPrice(primary.spotPrice) });
		if (secondary) out.push({ name: String(compareYear), color: SERIES_COLORS[1], points: toDailyPrice(secondary.spotPrice) });
		return out;
	});

	const monthLabels = $derived(MONTH_KEYS.map((k) => $translate(`months.${k}`)));

	const monthTicks = $derived.by(() => {
		const line = electricityDailySeries[0]?.points ?? [];
		const ticks: { pos: number; label: string }[] = [];
		let lastMonth = -1;
		for (const p of line) {
			const m = new Date(p.date + 'T00:00:00Z').getUTCMonth();
			if (m !== lastMonth) {
				ticks.push({ pos: p.dayOfYear, label: monthLabels[m] });
				lastMonth = m;
			}
		}
		return ticks;
	});

	function toScatter(elRows: ElectricityRow[], wRows: WeatherRow[]) {
		const dailyKwh = new Map(dailySeries(elRows, (r) => r.energy_kwh, 'sum').map((p) => [p.date, p.value]));
		const dailyTemp = dailySeries(wRows, (r) => r.temperature_c, 'avg');
		return dailyTemp
			.filter((p) => dailyKwh.has(p.date))
			.map((p) => ({ x: p.value, y: dailyKwh.get(p.date)!, date: p.date }));
	}
	const scatterSeries = $derived.by(() => {
		const out: { name: string; color: string; points: { x: number; y: number; date: string }[] }[] = [];
		if (primary) out.push({ name: String(year), color: SERIES_COLORS[0], points: toScatter(primary.electricity, primary.weather) });
		if (secondary) out.push({ name: String(compareYear), color: SERIES_COLORS[1], points: toScatter(secondary.electricity, secondary.weather) });
		return out;
	});

	function evMonthly(sessions: EvSession[]) {
		const out = new Array(12).fill(0);
		for (const s of sessions) out[new Date(s.start_time).getUTCMonth()] += s.energy_kwh;
		return out;
	}
	const evMonthlySeries = $derived.by(() => {
		const out: { name: string; color: string; values: number[] }[] = [];
		if (primary) out.push({ name: String(year), color: SERIES_COLORS[0], values: evMonthly(primary.evSessions) });
		if (secondary) out.push({ name: String(compareYear), color: SERIES_COLORS[1], values: evMonthly(secondary.evSessions) });
		return out;
	});

	function onYearChange(e: Event) {
		year = Number((e.target as HTMLSelectElement).value);
	}
	function onCompareChange(e: Event) {
		const v = (e.target as HTMLSelectElement).value;
		compareYear = v === '' ? null : Number(v);
	}
</script>

<svelte:head>
	<title>{$translate('dashboard.title')}</title>
</svelte:head>

<div class="page">
	<h1>{$translate('dashboard.title')}</h1>

	{#if manifestError}
		<div class="empty-state">
			<p><strong>{$translate('emptyState.heading')}</strong></p>
			<p>{$translate('emptyState.instructions')}</p>
			<pre>npm run generate:fake-data</pre>
			<p class="error-detail">{manifestError}</p>
		</div>
	{:else if !manifest}
		<p class="muted">{$translate('dashboard.loading')}</p>
	{:else}
		<div class="filters">
			<label>
				{$translate('dashboard.year')}
				<select value={year} onchange={onYearChange}>
					{#each manifest.years as y (y)}
						<option value={y}>{y}</option>
					{/each}
				</select>
			</label>
			<label>
				{$translate('dashboard.compareWith')}
				<select value={compareYear ?? ''} onchange={onCompareChange}>
					<option value="">{$translate('dashboard.none')}</option>
					{#each manifest.years.filter((y) => y !== year) as y (y)}
						<option value={y}>{y}</option>
					{/each}
				</select>
			</label>
			{#if isLoading}<span class="muted">{$translate('dashboard.loading')}</span>{/if}
		</div>

		{#if primary}
			<div class="kpis">
				<StatTile
					label={$translate('kpi.annualConsumption')}
					value="{formatCompact(totalKwh, $locale ?? 'en')} kWh"
					sub={String(year)}
				/>
				<StatTile
					label={$translate('kpi.averageTemperature')}
					value="{formatNumber(avgTemp, 1, $locale ?? 'en')}°C"
					sub={String(year)}
				/>
				<StatTile
					label={$translate('kpi.averageSpotPrice')}
					value="{formatNumber(avgSpotPrice, 1, $locale ?? 'en')} €/MWh"
					sub={String(year)}
				/>
				<StatTile
					label={$translate('kpi.evCharging')}
					value="{formatCompact(evTotalKwh, $locale ?? 'en')} kWh"
					sub={$translate('kpi.evSessionsSub', { values: { count: evSessionCount, year } })}
				/>
			</div>

			<div class="charts">
				<MonthlyBarChart
					title={$translate('charts.monthlyConsumption')}
					months={monthLabels}
					series={consumptionSeries}
					unit="kWh"
				/>

				<ScatterChart
					title={$translate('charts.consumptionVsTemperature')}
					series={scatterSeries}
					xLabel={$translate('charts.xTemperature')}
					yLabel={$translate('charts.yConsumption')}
				/>

				<DailyLineChart
					title={$translate('charts.dailyConsumption')}
					series={electricityDailySeries}
					unit="kWh"
					xTicks={monthTicks}
				/>

				<DailyLineChart
					title={$translate('charts.dailySpotPrice')}
					series={spotPriceDailySeries}
					unit="€/MWh"
					xTicks={monthTicks}
				/>

				<MonthlyBarChart
					title={$translate('charts.evMonthly')}
					months={monthLabels}
					series={evMonthlySeries}
					unit="kWh"
				/>
			</div>
		{/if}
	{/if}
</div>

<style>
	.page {
		max-width: 780px;
		margin: 0 auto;
		padding: 24px 16px 64px;
	}
	h1 {
		font-size: 22px;
		margin-bottom: 16px;
	}
	.muted {
		color: var(--text-secondary);
	}
	.filters {
		display: flex;
		align-items: center;
		gap: 20px;
		margin-bottom: 20px;
		font-size: 13px;
		color: var(--text-secondary);
	}
	.filters label {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	select {
		font-size: 13px;
		padding: 4px 6px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--surface-1);
		color: var(--text-primary);
	}
	.kpis {
		display: flex;
		flex-wrap: wrap;
		gap: 12px;
		margin-bottom: 24px;
	}
	.charts {
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.empty-state {
		background: var(--surface-1);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 20px;
	}
	.empty-state pre {
		background: var(--page-plane);
		padding: 10px 12px;
		border-radius: 6px;
		display: inline-block;
	}
	.error-detail {
		font-size: 12px;
		color: var(--text-muted);
	}
</style>
