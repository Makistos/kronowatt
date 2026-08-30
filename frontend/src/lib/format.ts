// Locale-aware formatting — pass the active i18n locale ($locale from
// svelte-i18n) so numbers/dates follow the UI language, not just its
// strings. Defaults to 'en' for callers outside a component (e.g. tests).

export const formatCompact = (n: number, locale = 'en'): string =>
	new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(n);

export const formatNumber = (n: number, digits = 0, locale = 'en'): string =>
	new Intl.NumberFormat(locale, { maximumFractionDigits: digits, minimumFractionDigits: digits }).format(n);

export const formatDate = (isoDate: string, locale = 'en'): string =>
	new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }).format(
		new Date(`${isoDate}T00:00:00Z`)
	);
