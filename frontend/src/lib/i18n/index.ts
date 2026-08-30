// i18n setup. English is the only registered locale for now — adding a
// second one is: drop a new locales/<code>.json file, register() it below,
// and (once there's actually a choice to make) add a language switcher.
// initialLocale is hardcoded to 'en' rather than detected from the browser
// (getLocaleFromNavigator()) because there is nothing else to fall back to
// yet — switch this once a second locale exists.
import { register, init } from 'svelte-i18n';

register('en', () => import('./locales/en.json'));

init({
	fallbackLocale: 'en',
	initialLocale: 'en'
});
