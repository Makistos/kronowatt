<script lang="ts">
	let { label, value, sub = '' } = $props<{ label: string; value: string; sub?: string }>();
</script>

<div class="tile">
	<div class="label">{label}</div>
	<div class="value">{value}</div>
	{#if sub}
		<div class="sub">{sub}</div>
	{/if}
</div>

<style>
	.tile {
		background: var(--surface-1);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 16px 18px;
		min-width: 150px;
	}
	.label {
		font-size: 13px;
		color: var(--text-secondary);
	}
	.value {
		font-size: 28px;
		font-weight: 600;
		color: var(--text-primary);
		margin-top: 4px;
	}
	.sub {
		font-size: 12px;
		color: var(--text-muted);
		margin-top: 2px;
	}
</style>
