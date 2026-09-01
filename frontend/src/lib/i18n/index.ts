// i18n setup. English and Finnish are registered; LanguageSwitcher.svelte
// lets the user pick between them and persists the choice to localStorage
// (read back in +layout.svelte's onMount, not here — this module also runs
// during adapter-static's prerender pass, where localStorage doesn't
// exist). initialLocale stays hardcoded to 'en' rather than detected from
// the browser (getLocaleFromNavigator()) — a saved preference should win
// over the browser's language, and defaulting new visitors to English
// (this app's only fully-reviewed locale) is safer than guessing from
// Accept-Language.
import { register, init } from 'svelte-i18n';

register('en', () => import('./locales/en.json'));
register('fi', () => import('./locales/fi.json'));

init({
	fallbackLocale: 'en',
	initialLocale: 'en'
});
