<script lang="ts">
	import { formatNumber, formatDate } from '$lib/format';
	import { locale } from 'svelte-i18n';
	import Tooltip from './Tooltip.svelte';

	type Point = { x: number; y: number; date?: string };
	type Series = { name: string; color: string; points: Point[] };

	let { title, series, xLabel, yLabel } = $props<{
		title: string;
		series: Series[];
		xLabel: string;
		yLabel: string;
	}>();

	const W = 720;
	const H = 320;
	const margin = { top: 16, right: 16, bottom: 40, left: 56 };
	const plotW = W - margin.left - margin.right;
	const plotH = H - margin.top - margin.bottom;

	const allPoints = $derived(series.flatMap((s: Series) => s.points));
	const xMin = $derived(Math.min(...allPoints.map((p: Point) => p.x)));
	const xMax = $derived(Math.max(...allPoints.map((p: Point) => p.x)));
	const yMin = $derived(Math.min(0, ...allPoints.map((p: Point) => p.y)));
	const yMax = $derived(Math.max(...allPoints.map((p: Point) => p.y)));

	function pad(min: number, max: number) {
		const span = max - min || 1;
		return { min: min - span * 0.08, max: max + span * 0.08 };
	}
	const xDomain = $derived(pad(xMin, xMax));
	const yDomain = $derived(pad(yMin, yMax));

	function ticks(min: number, max: number, n = 5) {
		return Array.from({ length: n }, (_, i) => min + (i / (n - 1)) * (max - min));
	}
	const xTicks = $derived(ticks(xDomain.min, xDomain.max));
	const yTicks = $derived(ticks(yDomain.min, yDomain.max));

	function x(v: number) {
		return margin.left + ((v - xDomain.min) / (xDomain.max - xDomain.min)) * plotW;
	}
	function y(v: number) {
		return margin.top + plotH - ((v - yDomain.min) / (yDomain.max - yDomain.min)) * plotH;
	}

	let hovered = $state<{ point: Point; seriesName: string; color: string } | null>(null);
	let pointerPos = $state({ x: 0, y: 0 });
	let containerEl: HTMLDivElement | undefined = $state();

	function onEnter(e: PointerEvent, point: Point, seriesName: string, color: string) {
		hovered = { point, seriesName, color };
		if (!containerEl) return;
		const rect = containerEl.getBoundingClientRect();
		pointerPos = { x: e.clientX - rect.left, y: e.clientY - rect.top };
	}
</script>

<figure class="chart-card">
	<figcaption>
		<span class="title">{title}</span>
	</figcaption>

	{#if series.length > 1}
		<div class="legend">
			{#each series as s (s.name)}
				<span class="legend-item"><span class="swatch" style="background:{s.color}"></span>{s.name}</span>
			{/each}
		</div>
	{/if}

	<div class="svg-wrap" bind:this={containerEl}>
		<svg viewBox="0 0 {W} {H}" role="img" aria-label={title}>
			{#each yTicks as t (t)}
				<line x1={margin.left} x2={W - margin.right} y1={y(t)} y2={y(t)} stroke="var(--gridline)" stroke-width="1" />
				<text x={margin.left - 8} y={y(t)} text-anchor="end" dominant-baseline="middle" class="axis-label"
					>{formatNumber(t, 1, $locale ?? 'en')}</text
				>
			{/each}
			{#each xTicks as t (t)}
				<text x={x(t)} y={H - margin.bottom + 18} text-anchor="middle" class="axis-label"
					>{formatNumber(t, 1, $locale ?? 'en')}</text
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
			<text x={margin.left + plotW / 2} y={H - 6} text-anchor="middle" class="axis-title">{xLabel}</text>
			<text
				x={-(margin.top + plotH / 2)}
				y={14}
				text-anchor="middle"
				transform="rotate(-90)"
				class="axis-title">{yLabel}</text
			>

			{#each series as s (s.name)}
				{#each s.points as p, i (i)}
					<circle cx={x(p.x)} cy={y(p.y)} r="4" fill={s.color} opacity="0.75" />
					<circle
						cx={x(p.x)}
						cy={y(p.y)}
						r="12"
						fill="transparent"
						onpointerenter={(e) => onEnter(e, p, s.name, s.color)}
						onpointerleave={() => (hovered = null)}
						role="presentation"
					/>
				{/each}
			{/each}
		</svg>

		<Tooltip x={pointerPos.x} y={pointerPos.y} visible={hovered !== null}>
			{#snippet children()}
				{#if hovered}
					<strong>{hovered.point.date ? formatDate(hovered.point.date, $locale ?? 'en') : ''}</strong>
					{#if series.length > 1}
						<div>{hovered.seriesName}</div>
					{/if}
					<div>{xLabel}: <strong>{formatNumber(hovered.point.x, 1, $locale ?? 'en')}</strong></div>
					<div>{yLabel}: <strong>{formatNumber(hovered.point.y, 1, $locale ?? 'en')}</strong></div>
				{/if}
			{/snippet}
		</Tooltip>
	</div>
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
		margin-bottom: 8px;
	}
	.title {
		font-size: 14px;
		font-weight: 600;
		color: var(--text-primary);
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
		border-radius: 50%;
		display: inline-block;
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
	.axis-title {
		font-size: 12px;
		fill: var(--text-secondary);
	}
</style>
