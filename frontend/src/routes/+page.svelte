<script lang="ts">
	import { onMount } from 'svelte';
	import {
		loadYears,
		loadDateRange,
		loadElectricity,
		loadWeather,
		loadSpotPrice,
		loadEvSessions,
		type DateRange,
		type ElectricityRow,
		type WeatherRow,
		type SpotPriceRow,
		type EvSession
	} from '$lib/api';
	import { MONTH_KEYS, dailySeries } from '$lib/aggregate';
	import { formatCompact, formatNumber } from '$lib/format';
	import { _ as translate, locale } from 'svelte-i18n';
	import Tabs from '$lib/Tabs.svelte';
	import ElectricityChart from '$lib/charts/ElectricityChart.svelte';
	import SpotPriceChart from '$lib/charts/SpotPriceChart.svelte';
	import MonthlyBarChart from '$lib/charts/MonthlyBarChart.svelte';
	import ScatterChart from '$lib/charts/ScatterChart.svelte';
	import StatTile from '$lib/charts/StatTile.svelte';

	type YearData = {
		electricity: ElectricityRow[];
		weather: WeatherRow[];
		spotPrice: SpotPriceRow[];
		evSessions: EvSession[];
	};

	const SERIES_COLORS = ['var(--series-1)', 'var(--series-2)'];

	let availableYears = $state<number[] | null>(null);
	let dateRange = $state<DateRange>({ min: null, max: null });
	let yearsError = $state<string | null>(null);
	let year = $state<number | null>(null);
	let compareYear = $state<number | null>(null);

	let cache = $state<Map<number, YearData>>(new Map());
	let loadingYears = $state<Set<number>>(new Set());

	let activeTab = $state('electricity');
	const tabs = $derived([
		{ id: 'electricity', label: $translate('tabs.electricity') },
		{ id: 'weather', label: $translate('tabs.weather') },
		{ id: 'spotPrice', label: $translate('tabs.spotPrice') },
		{ id: 'ev', label: $translate('tabs.ev') }
	]);

	onMount(async () => {
		try {
			const [years, range] = await Promise.all([loadYears(), loadDateRange()]);
			// Today's year is always selectable, even before any data exists
			// for it yet — a fresh deployment on day 1 should default to the
			// current (so-far-empty) year, not silently fall back to nothing.
			const todayYear = new Date().getUTCFullYear();
			availableYears = years.includes(todayYear) ? years : [...years, todayYear].sort((a, b) => a - b);
			dateRange = range;
			year = todayYear;
			// Compare against the most recent *other* year that actually has
			// data — usually last year, since this year is only selectable
			// via the line above and may have none yet.
			const otherDataYears = years.filter((y) => y !== todayYear);
			compareYear = otherDataYears.length > 0 ? otherDataYears.at(-1)! : null;
		} catch (e) {
			yearsError = e instanceof Error ? e.message : String(e);
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
	const evTotalKwh = $derived(primary ? primary.evSessions.reduce((s, e) => s + e.energy_kwh, 0) : 0);
	const evSessionCount = $derived(primary ? primary.evSessions.length : 0);
	const avgSpotPrice = $derived(
		primary && primary.spotPrice.length
			? primary.spotPrice.reduce((s, r) => s + r.price_eur_mwh, 0) / primary.spotPrice.length
			: 0
	);

	const monthLabels = $derived(MONTH_KEYS.map((k) => $translate(`months.${k}`)));

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
		// Can't compare a year against itself — the compare year option list
		// already excludes `year`, so if they now match, the underlying state
		// is stale and must be cleared too (otherwise two series share a key).
		if (compareYear === year) compareYear = null;
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

	{#if yearsError}
		<div class="empty-state">
			<p><strong>{$translate('emptyState.heading')}</strong></p>
			<p>{$translate('emptyState.instructions')}</p>
			<p class="error-detail">{yearsError}</p>
		</div>
	{:else if !availableYears}
		<p class="muted">{$translate('dashboard.loading')}</p>
	{:else}
		<div class="filters">
			<label>
				{$translate('dashboard.year')}
				<select value={year} onchange={onYearChange}>
					{#each availableYears as y (y)}
						<option value={y}>{y}</option>
					{/each}
				</select>
			</label>
			{#if availableYears.length > 1}
				<label>
					{$translate('dashboard.compareWith')}
					<select value={compareYear ?? ''} onchange={onCompareChange}>
						<option value="">{$translate('dashboard.none')}</option>
						{#each availableYears.filter((y) => y !== year) as y (y)}
							<option value={y}>{y}</option>
						{/each}
					</select>
				</label>
			{/if}
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
					label={$translate('kpi.averageSpotPrice')}
					value="{formatNumber(avgSpotPrice / 10, 2, $locale ?? 'en')} c/kWh"
					sub={String(year)}
				/>
				<StatTile
					label={$translate('kpi.evCharging')}
					value="{formatCompact(evTotalKwh, $locale ?? 'en')} kWh"
					sub={$translate('kpi.evSessionsSub', { values: { count: evSessionCount, year } })}
				/>
			</div>
		{/if}

		<Tabs {tabs} bind:active={activeTab} />

		<div class="tab-content" class:hidden={activeTab !== 'electricity'}>
			<ElectricityChart {availableYears} {dateRange} />
		</div>

		<div class="tab-content" class:hidden={activeTab !== 'spotPrice'}>
			<SpotPriceChart {availableYears} {dateRange} />
		</div>

		{#if primary}
			<div class="tab-content" class:hidden={activeTab !== 'weather'}>
				<ScatterChart
					title={$translate('charts.consumptionVsTemperature')}
					series={scatterSeries}
					xLabel={$translate('charts.xTemperature')}
					yLabel={$translate('charts.yConsumption')}
				/>
			</div>

			<div class="tab-content" class:hidden={activeTab !== 'ev'}>
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
	.tab-content.hidden {
		display: none;
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
