<script lang="ts">
	import { onMount } from 'svelte';
	import '../app.css';
	import '$lib/i18n';
	import { isLoading, locale, _ as translate } from 'svelte-i18n';
	import LanguageSwitcher from '$lib/LanguageSwitcher.svelte';
	import SettingsDialog from '$lib/SettingsDialog.svelte';
	import { contractsStore } from '$lib/contractsStore.svelte';

	const LOCALE_STORAGE_KEY = 'kronowatt:locale';

	let { children } = $props();
	let settingsDialog: SettingsDialog | undefined = $state();

	// onMount only runs in the browser, never during adapter-static's
	// prerender pass — safe to touch localStorage here, unlike at module
	// top level in i18n/index.ts. A saved choice briefly re-triggers
	// $isLoading while its locale JSON loads, which the gate below already
	// handles the same way as the initial load.
	onMount(() => {
		const saved = localStorage.getItem(LOCALE_STORAGE_KEY);
		if (saved) locale.set(saved);
		contractsStore.refresh();
	});
</script>

<div class="top-bar">
	<button
		type="button"
		class="settings-button"
		onclick={() => settingsDialog?.open()}
		aria-label={$translate('settings.title')}
		title={$translate('settings.title')}
	>
		⚙️
	</button>
	<LanguageSwitcher />
</div>

<SettingsDialog bind:this={settingsDialog} />

{#if $isLoading}
	<!-- avoids a flash of untranslated keys before the locale JSON loads -->
{:else}
	{@render children()}
{/if}

<style>
	.top-bar {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 12px;
		max-width: 780px;
		margin: 0 auto;
		padding: 12px 16px 0;
	}
	.settings-button {
		font-size: 16px;
		line-height: 1;
		padding: 4px 6px;
		border-radius: 4px;
		border: 1px solid transparent;
		background: none;
		cursor: pointer;
		opacity: 0.6;
	}
	.settings-button:hover {
		opacity: 1;
		border-color: var(--border);
		background: var(--surface-1);
	}
</style>
