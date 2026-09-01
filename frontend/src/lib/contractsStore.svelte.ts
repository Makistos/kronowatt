import { loadContracts, type Contract } from './api';

/** Shared, app-wide contract list — loaded once and re-shared by every
 * consumer (currently just SpotPriceChart's cost/paid-price calculations)
 * rather than each component fetching its own copy of what's a small,
 * rarely-changing list. `+layout.svelte` loads it once on mount;
 * SettingsDialog calls `refresh()` again after successfully adding a new
 * contract so open charts pick up the change without a full page reload. */
class ContractsStore {
	contracts = $state<Contract[]>([]);

	async refresh() {
		this.contracts = await loadContracts();
	}
}

export const contractsStore = new ContractsStore();
