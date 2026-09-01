import type { Contract, ElectricityRow, SpotPriceRow } from './api';
import { daysInMonthUTC } from './electricityBuckets';

/** The contract effective at `at` — the one with the latest valid_from
 * that's still <= at (spec §2.4: "effective price determined by
 * timestamp"). `contracts` must be sorted oldest-first, which is what
 * loadContracts() already returns. Returns null before any contract's
 * valid_from, e.g. no contract configured yet. */
export function effectiveContractAt(contracts: Contract[], at: Date): Contract | null {
	let result: Contract | null = null;
	for (const c of contracts) {
		if (new Date(c.valid_from).getTime() <= at.getTime()) result = c;
		else break;
	}
	return result;
}

/** The per-kWh rate (c/kWh) actually paid under `contract` at the moment
 * its matching electricity sample was taken — energy (fixed price, or spot
 * price + margin) plus the flat transfer rate. `spotPriceEurPerMwh` is the
 * exact-timestamp spot price row for a spot contract; ignored for a fixed
 * one. Returns null if there's no contract yet, or a spot contract is
 * effective but no matching spot price sample exists for that timestamp. */
export function paidRateCPerKWh(contract: Contract | null, spotPriceEurPerMwh: number | null): number | null {
	if (!contract) return null;
	let energy: number | null;
	if (contract.pricing_model === 'fixed') {
		energy = contract.energy_price_c_per_kwh;
	} else {
		// EUR/MWh -> c/kWh is divide by 10 (see SpotPriceChart.svelte's
		// toPoints for the same conversion, spelled out in full there).
		energy =
			spotPriceEurPerMwh !== null && contract.spot_margin_c_per_kwh !== null
				? spotPriceEurPerMwh / 10 + contract.spot_margin_c_per_kwh
				: null;
	}
	if (energy === null) return null;
	return energy + contract.transfer_price_c_per_kwh;
}

/** monthly_fee + electricity_tax + transfer_tax — the flat EUR/month
 * charges that don't scale with consumption. */
function monthlyFlatEur(contract: Contract): number {
	return contract.monthly_fee_eur + contract.electricity_tax_eur + contract.transfer_tax_eur;
}

/** Sums the flat monthly charges (fee + both taxes) actually owed across
 * every distinct calendar day present in `sampleTimes`, prorating each
 * contract's monthly total by days-in-that-month. This is a day-level
 * proration, not a sample-level one — a monthly fee is owed once per day
 * regardless of how many 15-minute samples land on it, and a contract that
 * changes mid-window is handled naturally since each day resolves its own
 * effective contract. Returns 0 if no contract covers any of these days. */
export function proratedFlatChargesEur(contracts: Contract[], sampleTimes: string[]): number {
	const days = new Set<string>();
	for (const t of sampleTimes) days.add(t.slice(0, 10));

	let total = 0;
	for (const day of days) {
		const at = new Date(`${day}T00:00:00Z`);
		const contract = effectiveContractAt(contracts, at);
		if (!contract) continue;
		const daysInMonth = daysInMonthUTC(at.getUTCFullYear(), at.getUTCMonth());
		total += monthlyFlatEur(contract) / daysInMonth;
	}
	return total;
}

/** Sums what the household actually paid across `elRows`, matching each
 * sample to its exact-timestamp spot price (for spot contracts) and its
 * effective contract (for either pricing model), plus each day's prorated
 * share of the flat monthly charges. Samples with no effective contract
 * yet, or a spot contract with no matching spot price row, are skipped —
 * same "can't compute what we don't have" behavior as the existing
 * spot-cost comparison. Returns null (not 0) if the window has electricity
 * data but no sample in it matched any contract — "no contract configured
 * yet" must never render as "this cost 0 €", which would read as free
 * electricity rather than "unknown". */
export function computePaidCostEur(
	contracts: Contract[],
	elRows: ElectricityRow[],
	spotRows: SpotPriceRow[]
): number | null {
	const spotByTime = new Map<string, number>();
	for (const p of spotRows) spotByTime.set(p.time, p.price_eur_mwh);

	let energyCostEur = 0;
	const matchedTimes: string[] = [];
	for (const r of elRows) {
		const contract = effectiveContractAt(contracts, new Date(r.time));
		const rate = paidRateCPerKWh(contract, spotByTime.get(r.time) ?? null);
		if (rate === null) continue;
		energyCostEur += (r.energy_kwh * rate) / 100; // c/kWh -> EUR/kWh
		matchedTimes.push(r.time);
	}

	if (elRows.length > 0 && matchedTimes.length === 0) return null;
	return energyCostEur + proratedFlatChargesEur(contracts, matchedTimes);
}
