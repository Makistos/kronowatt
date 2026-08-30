<script lang="ts">
	import '../app.css';
	import '$lib/i18n';
	import { isLoading } from 'svelte-i18n';

	let { children } = $props();
</script>

{#if $isLoading}
	<!-- avoids a flash of untranslated keys before the locale JSON loads -->
{:else}
	{@render children()}
{/if}
