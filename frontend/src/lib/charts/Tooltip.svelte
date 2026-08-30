<script lang="ts">
	let { x, y, visible, children } = $props<{
		x: number;
		y: number;
		visible: boolean;
		children: () => unknown;
	}>();
</script>

{#if visible}
	<div class="tooltip" style="left:{x}px; top:{y}px;">
		{@render children()}
	</div>
{/if}

<style>
	.tooltip {
		position: absolute;
		transform: translate(-50%, -100%) translateY(-10px);
		background: var(--surface-1);
		color: var(--text-primary);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 6px 10px;
		font-size: 12px;
		line-height: 1.5;
		pointer-events: none;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
		white-space: nowrap;
		z-index: 10;
	}
</style>
