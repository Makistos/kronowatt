const compactFormatter = new Intl.NumberFormat('en-US', {
	notation: 'compact',
	maximumFractionDigits: 1
});

const numberFormatter = (digits: number) =>
	new Intl.NumberFormat('en-US', { maximumFractionDigits: digits, minimumFractionDigits: digits });

export const formatCompact = (n: number): string => compactFormatter.format(n);

export const formatNumber = (n: number, digits = 0): string => numberFormatter(digits).format(n);
