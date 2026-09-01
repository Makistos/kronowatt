<script lang="ts">
	import { locale, _ as translate } from 'svelte-i18n';

	const LOCALE_STORAGE_KEY = 'kronowatt:locale';

	const LANGUAGES = [
		{ code: 'en', flag: '🇬🇧', labelKey: 'dashboard.languageEnglish' },
		{ code: 'fi', flag: '🇫🇮', labelKey: 'dashboard.languageFinnish' }
	];

	function select(code: string) {
		locale.set(code);
		localStorage.setItem(LOCALE_STORAGE_KEY, code);
	}
</script>

<div class="language-switcher" role="group" aria-label={$translate('dashboard.language')}>
	{#each LANGUAGES as lang (lang.code)}
		<button
			type="button"
			class="flag"
			class:active={$locale === lang.code}
			onclick={() => select(lang.code)}
			aria-pressed={$locale === lang.code}
			title={$translate(lang.labelKey)}
		>
			{lang.flag}
		</button>
	{/each}
</div>

<style>
	.language-switcher {
		display: flex;
		gap: 4px;
	}
	.flag {
		font-size: 16px;
		line-height: 1;
		padding: 4px 6px;
		border-radius: 4px;
		border: 1px solid transparent;
		background: none;
		cursor: pointer;
		opacity: 0.5;
	}
	.flag:hover {
		opacity: 0.8;
	}
	.flag.active {
		opacity: 1;
		border-color: var(--border);
		background: var(--surface-1);
	}
</style>
