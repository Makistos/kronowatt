<script lang="ts">
	let { tabs, active = $bindable() } = $props<{
		tabs: { id: string; label: string }[];
		active: string;
	}>();
</script>

<div class="tabs" role="tablist">
	{#each tabs as tab (tab.id)}
		<button
			role="tab"
			aria-selected={active === tab.id}
			class:active={active === tab.id}
			onclick={() => (active = tab.id)}
		>
			{tab.label}
		</button>
	{/each}
</div>

<style>
	.tabs {
		display: flex;
		gap: 4px;
		border-bottom: 1px solid var(--gridline);
		margin-bottom: 20px;
	}
	button {
		font-size: 13px;
		font-weight: 500;
		color: var(--text-secondary);
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		padding: 10px 4px;
		margin-bottom: -1px;
		cursor: pointer;
	}
	button + button {
		margin-left: 16px;
	}
	button.active {
		color: var(--text-primary);
		border-bottom-color: var(--series-1);
	}
	button:hover:not(.active) {
		color: var(--text-primary);
	}
</style>
