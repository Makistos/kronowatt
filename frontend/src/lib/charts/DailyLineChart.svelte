<script lang="ts">
	import { niceMax } from '$lib/aggregate';
	import { formatNumber, formatDate } from '$lib/format';
	import { _ as translate, locale } from 'svelte-i18n';
	import Tooltip from './Tooltip.svelte';

	type Point = { dayOfYear: number; value: number; date?: string };
	type Series = { name: string; color: string; points: Point[] };

	let { title, series, unit, xTicks = [] } = $props<{
		title: string;
		series: Series[];
		unit: string;
		xTicks?: { pos: number; label: string }[];
	}>();

	const W = 720;
	const H = 260;
	const margin = { top: 16, right: 16, bottom: 28, left: 52 };
	const plotW = W - margin.left - margin.right;
	const plotH = H - margin.top - margin.bottom;

	const maxDay = $derived(
		Math.max(1, ...series.flatMap((s: Series) => s.points.map((p: Point) => p.dayOfYear)))
	);
	const maxVal = $derived(
		niceMax(Math.max(1, ...series.flatMap((s: Series) => s.points.map((p: Point) => p.value))))
	);
	const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => f * maxVal));

	function x(d: number) {
		return margin.left + (d / maxDay) * plotW;
	}
	function y(v: number) {
		return margin.top + plotH - (v / maxVal) * plotH;
	}

	function pathFor(points: Point[]) {
		return points.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.dayOfYear)},${y(p.value)}`).join(' ');
	}

	function nearest(points: Point[], day: number): Point | undefined {
		let best: Point | undefined;
		let bestDist = Infinity;
		for (const p of points) {
			const dist = Math.abs(p.dayOfYear - day);
			if (dist < bestDist) {
				bestDist = dist;
				best = p;
			}
		}
		return best;
	}

	let hoverDay = $state<number | null>(null);
	let pointerPos = $state({ x: 0, y: 0 });
	let containerEl: HTMLDivElement | undefined = $state();

	function onMove(e: PointerEvent) {
		if (!containerEl) return;
		const rect = containerEl.getBoundingClientRect();
		const px = e.clientX - rect.left;
		const frac = (px - margin.left) / plotW;
		hoverDay = Math.round(Math.min(1, Math.max(0, frac)) * maxDay);
		pointerPos = { x: px, y: e.clientY - rect.top };
	}
</script>

<figure class="chart-card">
	<figcaption>
		<span class="title">{title}</span>
	</figcaption>

	{#if series.length > 1}
		<div class="legend">
			{#each series as s (s.name)}
				<span class="legend-item"><span class="key" style="background:{s.color}"></span>{s.name}</span>
			{/each}
		</div>
	{/if}

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
			{#each xTicks as t (t.label + t.pos)}
				<text x={x(t.pos)} y={H - 8} text-anchor="middle" class="axis-label">{t.label}</text>
			{/each}

			{#each series as s (s.name)}
				<path d={pathFor(s.points)} fill="none" stroke={s.color} stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
			{/each}

			{#if hoverDay !== null}
				<line
					x1={x(hoverDay)}
					x2={x(hoverDay)}
					y1={margin.top}
					y2={margin.top + plotH}
					stroke="var(--baseline)"
					stroke-width="1"
				/>
				{#each series as s (s.name)}
					{@const p = nearest(s.points, hoverDay)}
					{#if p}
						<circle cx={x(p.dayOfYear)} cy={y(p.value)} r="5" fill={s.color} stroke="var(--surface-1)" stroke-width="2" />
					{/if}
				{/each}
			{/if}

			<rect
				x={margin.left}
				y={margin.top}
				width={plotW}
				height={plotH}
				fill="transparent"
				onpointermove={onMove}
				onpointerleave={() => (hoverDay = null)}
				role="presentation"
			/>
		</svg>

		<Tooltip x={pointerPos.x} y={pointerPos.y} visible={hoverDay !== null}>
			{#snippet children()}
				{#if hoverDay !== null}
					{@const anyPoint = nearest(series[0]?.points ?? [], hoverDay)}
					<strong
						>{anyPoint?.date
							? formatDate(anyPoint.date, $locale ?? 'en')
							: $translate('charts.dayLabel', { values: { day: hoverDay } })}</strong
					>
					{#each series as s (s.name)}
						{@const p = nearest(s.points, hoverDay)}
						{#if p}
							<div><span class="key" style="background:{s.color}"></span>{s.name}: <strong
									>{formatNumber(p.value, 1, $locale ?? 'en')} {unit}</strong
								></div
							>
						{/if}
					{/each}
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
	.key {
		width: 10px;
		height: 2px;
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
</style>
