<script lang="ts">
	import { niceMax } from '$lib/aggregate';
	import { formatNumber } from '$lib/format';
	import { _ as translate, locale } from 'svelte-i18n';
	import Tooltip from './Tooltip.svelte';

	type Series = { name: string; color: string; values: number[] }; // values.length === months.length

	let { title, months, series, unit } = $props<{
		title: string;
		months: string[];
		series: Series[];
		unit: string;
	}>();

	const W = 720;
	const H = 300;
	const margin = { top: 16, right: 16, bottom: 32, left: 52 };
	const plotW = W - margin.left - margin.right;
	const plotH = H - margin.top - margin.bottom;

	const maxVal = $derived(niceMax(Math.max(1, ...series.flatMap((s: Series) => s.values))));
	const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => f * maxVal));

	const bandWidth = $derived(plotW / months.length);
	const maxBarWidth = 24;
	const barGap = 2;
	const barWidth = $derived(
		Math.min(maxBarWidth, (bandWidth * 0.7 - (series.length - 1) * barGap) / series.length)
	);
	const groupWidth = $derived(series.length * barWidth + (series.length - 1) * barGap);

	function y(v: number) {
		return margin.top + plotH - (v / maxVal) * plotH;
	}

	let hoveredMonth = $state<number | null>(null);
	let pointerPos = $state({ x: 0, y: 0 });
	let containerEl: HTMLDivElement | undefined = $state();

	function onMove(e: PointerEvent, monthIndex: number) {
		hoveredMonth = monthIndex;
		if (!containerEl) return;
		const rect = containerEl.getBoundingClientRect();
		pointerPos = { x: e.clientX - rect.left, y: e.clientY - rect.top };
	}

	let showTable = $state(false);
</script>

<figure class="chart-card">
	<figcaption>
		<span class="title">{title}</span>
		<button class="table-toggle" onclick={() => (showTable = !showTable)}>
			{showTable ? $translate('charts.chartView') : $translate('charts.tableView')}
		</button>
	</figcaption>

	{#if series.length > 1}
		<div class="legend">
			{#each series as s (s.name)}
				<span class="legend-item"><span class="swatch" style="background:{s.color}"></span>{s.name}</span>
			{/each}
		</div>
	{/if}

	{#if showTable}
		<table class="data-table">
			<thead>
				<tr>
					<th>{$translate('charts.tableMonthHeader')}</th>
					{#each series as s (s.name)}
						<th>{s.name} ({unit})</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each months as m, i (m)}
					<tr>
						<td>{m}</td>
						{#each series as s (s.name)}
							<td>{formatNumber(s.values[i], 1, $locale ?? 'en')}</td>
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	{:else}
		<div class="svg-wrap" bind:this={containerEl}>
			<svg viewBox="0 0 {W} {H}" role="img" aria-label={title}>
				{#each yTicks as t (t)}
					<line
						x1={margin.left}
						x2={W - margin.right}
						y1={y(t)}
						y2={y(t)}
						stroke="var(--gridline)"
						stroke-width="1"
					/>
					<text x={margin.left - 8} y={y(t)} text-anchor="end" dominant-baseline="middle" class="axis-label"
						>{formatNumber(t, 0, $locale ?? 'en')}</text
					>
				{/each}
				<line
					x1={margin.left}
					x2={W - margin.right}
					y1={margin.top + plotH}
					y2={margin.top + plotH}
					stroke="var(--baseline)"
					stroke-width="1"
				/>

				{#each months as m, i (m)}
					{@const bandStart = margin.left + i * bandWidth}
					{@const groupStart = bandStart + (bandWidth - groupWidth) / 2}
					<text
						x={bandStart + bandWidth / 2}
						y={H - 8}
						text-anchor="middle"
						class="axis-label">{m}</text
					>
					{#each series as s, si (s.name)}
						{@const bx = groupStart + si * (barWidth + barGap)}
						{@const bv = s.values[i]}
						{@const by = y(bv)}
						<rect
							x={bx}
							y={by}
							width={barWidth}
							height={margin.top + plotH - by}
							rx="4"
							fill={s.color}
							opacity={hoveredMonth === null || hoveredMonth === i ? 1 : 0.45}
						/>
					{/each}
					<rect
						x={bandStart}
						y={margin.top}
						width={bandWidth}
						height={plotH}
						fill="transparent"
						onpointermove={(e) => onMove(e, i)}
						onpointerleave={() => (hoveredMonth = null)}
						role="presentation"
					/>
				{/each}
			</svg>

			<Tooltip x={pointerPos.x} y={pointerPos.y} visible={hoveredMonth !== null}>
				{#snippet children()}
					{#if hoveredMonth !== null}
						<strong>{months[hoveredMonth]}</strong>
						{#each series as s (s.name)}
							<div><span class="key" style="background:{s.color}"></span>{s.name}: <strong
									>{formatNumber(s.values[hoveredMonth], 1, $locale ?? 'en')} {unit}</strong
								></div
							>
						{/each}
					{/if}
				{/snippet}
			</Tooltip>
		</div>
	{/if}
</figure>

<style>
	.chart-card {
		background: var(--surface-1);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 16px;
		margin: 0;
	}
	figcaption {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 8px;
	}
	.title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
	}
	.table-toggle {
		font-size: 12px;
		color: var(--text-secondary);
		background: none;
		border: 1px solid var(--border);
		border-radius: 4px;
		padding: 4px 8px;
		cursor: pointer;
	}
	.legend {
		display: flex;
		gap: 16px;
		margin-bottom: 8px;
		font-size: 12px;
		color: var(--text-secondary);
	}
	.legend-item {
		display: flex;
		align-items: center;
		gap: 6px;
	}
	.swatch {
		width: 10px;
		height: 10px;
		border-radius: 2px;
		display: inline-block;
	}
	.key {
		width: 8px;
		height: 2px;
		display: inline-block;
		margin-right: 4px;
	}
	.svg-wrap {
		position: relative;
	}
	svg {
		width: 100%;
		height: auto;
		display: block;
	}
	.axis-label {
		font-size: 11px;
		fill: var(--text-muted);
	}
	.data-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 13px;
	}
	.data-table th,
	.data-table td {
		text-align: right;
		padding: 6px 8px;
		border-bottom: 1px solid var(--gridline);
		font-variant-numeric: tabular-nums;
	}
	.data-table th:first-child,
	.data-table td:first-child {
		text-align: left;
	}
</style>
