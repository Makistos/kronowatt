<script lang="ts">
	import { niceMax } from '$lib/aggregate';
	import { formatNumber } from '$lib/format';
	import { _ as translate, locale } from 'svelte-i18n';
	import Tooltip from './Tooltip.svelte';

	type Group = { name: string; color: string };

	// values[categoryIndex][groupIndex][stackIndex]
	let { title, categories, groups, stackLabels, values, unit } = $props<{
		title: string;
		categories: string[];
		groups: Group[];
		stackLabels: string[];
		values: number[][][];
		unit: string;
	}>();

	const STACK_OPACITY = [1, 0.6, 0.35];

	const W = 720;
	const H = 300;
	const margin = { top: 16, right: 16, bottom: 32, left: 52 };
	const plotW = W - margin.left - margin.right;
	const plotH = H - margin.top - margin.bottom;

	// Totals per category+group (sum across stacks), for the y-scale and
	// bar heights.
	const categoryGroupTotals = $derived(
		values.map((cat: number[][]) => cat.map((group) => group.reduce((s, v) => s + v, 0)))
	);
	const maxVal = $derived(niceMax(Math.max(0, ...categoryGroupTotals.flat())));
	const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => f * maxVal));
	// Small-magnitude data (e.g. 15-min electricity buckets, often < 1 kWh)
	// needs decimals on the axis or every tick rounds to the same integer.
	const yAxisDigits = $derived(maxVal < 2 ? 2 : maxVal < 20 ? 1 : 0);

	const bandWidth = $derived(plotW / Math.max(1, categories.length));
	const maxBarWidth = 24;
	const barGap = 2;
	const barWidth = $derived(
		Math.min(maxBarWidth, (bandWidth * 0.7 - (groups.length - 1) * barGap) / groups.length)
	);
	const groupWidth = $derived(groups.length * barWidth + (groups.length - 1) * barGap);

	// Thin x-axis labels so they don't collide when there are many bars
	// (up to 96 at 15-min resolution) — every bar still renders, just not
	// every label.
	const labelStride = $derived(Math.max(1, Math.ceil(categories.length / 14)));

	function y(v: number) {
		return margin.top + plotH - (v / maxVal) * plotH;
	}

	let hoveredIndex = $state<number | null>(null);
	let pointerPos = $state({ x: 0, y: 0 });
	let containerEl: HTMLDivElement | undefined = $state();

	function onMove(e: PointerEvent, index: number) {
		hoveredIndex = index;
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

	{#if groups.length > 1 || stackLabels.length > 1}
		<div class="legend">
			{#if groups.length > 1}
				<span class="legend-group">
					{#each groups as g (g.name)}
						<span class="legend-item"><span class="swatch" style="background:{g.color}"></span>{g.name}</span>
					{/each}
				</span>
			{/if}
			{#if stackLabels.length > 1}
				<span class="legend-group">
					{#each stackLabels as label, i (label)}
						<span class="legend-item"
							><span
								class="swatch"
								style="background:{groups[0].color}; opacity:{STACK_OPACITY[i] ?? 1}"
							></span>{label}</span
						>
					{/each}
				</span>
			{/if}
		</div>
	{/if}

	{#if showTable}
		<div class="table-wrap">
			<table class="data-table">
				<thead>
					<tr>
						<th>{$translate('charts.tableCategoryHeader')}</th>
						{#each groups as g (g.name)}
							{#each stackLabels as label (label)}
								<th>{groups.length > 1 ? `${g.name} ` : ''}{label} ({unit})</th>
							{/each}
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each categories as c, ci (c)}
						<tr>
							<td>{c}</td>
							{#each groups as g, gi (g.name)}
								{#each stackLabels as label, si (label)}
									<td>{formatNumber(values[ci]?.[gi]?.[si] ?? 0, 2, $locale ?? 'en')}</td>
								{/each}
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
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
						>{formatNumber(t, yAxisDigits, $locale ?? 'en')}</text
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

				{#each categories as c, ci (ci)}
					{@const bandStart = margin.left + ci * bandWidth}
					{@const groupStart = bandStart + (bandWidth - groupWidth) / 2}
					{#if ci % labelStride === 0}
						<text x={bandStart + bandWidth / 2} y={H - 8} text-anchor="middle" class="axis-label">{c}</text>
					{/if}
					{#each groups as g, gi (g.name)}
						{@const bx = groupStart + gi * (barWidth + barGap)}
						{#each stackLabels as label, si (label)}
							{@const below = (values[ci]?.[gi] ?? []).slice(0, si).reduce((s: number, v: number) => s + v, 0)}
							{@const v = values[ci]?.[gi]?.[si] ?? 0}
							{@const yTop = y(below + v)}
							{@const yBottom = y(below)}
							<rect
								x={bx}
								y={yTop}
								width={barWidth}
								height={Math.max(0, yBottom - yTop)}
								rx={si === stackLabels.length - 1 ? 4 : 0}
								fill={g.color}
								opacity={(STACK_OPACITY[si] ?? 1) * (hoveredIndex === null || hoveredIndex === ci ? 1 : 0.45)}
							/>
						{/each}
					{/each}
					<rect
						x={bandStart}
						y={margin.top}
						width={bandWidth}
						height={plotH}
						fill="transparent"
						onpointermove={(e) => onMove(e, ci)}
						onpointerleave={() => (hoveredIndex = null)}
						role="presentation"
					/>
				{/each}
			</svg>

			<Tooltip x={pointerPos.x} y={pointerPos.y} visible={hoveredIndex !== null}>
				{#snippet children()}
					{#if hoveredIndex !== null}
						<strong>{categories[hoveredIndex]}</strong>
						{#each groups as g, gi (g.name)}
							{#each stackLabels as label, si (label)}
								<div>
									<span class="key" style="background:{g.color}; opacity:{STACK_OPACITY[si] ?? 1}"></span>
									{groups.length > 1 ? `${g.name} ` : ''}{stackLabels.length > 1 ? label : ''}: <strong
										>{formatNumber(values[hoveredIndex]?.[gi]?.[si] ?? 0, 2, $locale ?? 'en')} {unit}</strong
									>
								</div>
							{/each}
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
		flex-wrap: wrap;
		gap: 20px;
		margin-bottom: 8px;
		font-size: 12px;
		color: var(--text-secondary);
	}
	.legend-group {
		display: flex;
		gap: 16px;
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
	.table-wrap {
		overflow-x: auto;
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
		white-space: nowrap;
	}
	.data-table th:first-child,
	.data-table td:first-child {
		text-align: left;
	}
</style>
