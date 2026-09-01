<script lang="ts">
	import { createContract, updateContract, type Contract, type NewContract } from './api';
	import { contractsStore } from './contractsStore.svelte';
	import { formatDate, formatNumber } from './format';
	import { _ as translate, locale } from 'svelte-i18n';

	let dialogEl: HTMLDialogElement | undefined = $state();

	// null = the form is adding a new contract; otherwise the id of the
	// existing contract currently loaded into the form for editing.
	let editingId = $state<number | null>(null);

	function todayISO(): string {
		return new Date().toISOString().slice(0, 10);
	}

	// <input type="number"> binds a *number*, not a string (Svelte
	// special-cases numeric inputs) — an empty field binds to '' though, so
	// every one of these is a union, never a plain string. Caught this at
	// runtime (a Puppeteer smoke test), not by svelte-check: bind:value's
	// typing didn't flag the earlier plain `$state('')` declarations even
	// though they broke `.trim()` the moment a field was filled in.
	let startDate = $state(todayISO());
	let fixedElectricityCost = $state<number | ''>('');
	let fixedTransferCost = $state<number | ''>('');
	let fixedMonthlyFee = $state<number | ''>('');
	let fixedElectricityTax = $state<number | ''>('');
	let fixedTransferTax = $state<number | ''>('');
	let spotElectricityCost = $state<number | ''>('');
	let spotTransferCost = $state<number | ''>('');
	let spotMonthlyFee = $state<number | ''>('');
	let spotElectricityTax = $state<number | ''>('');
	let spotTransferTax = $state<number | ''>('');

	let error = $state<string | null>(null);
	let saved = $state(false);
	let saving = $state(false);

	function resetForm() {
		editingId = null;
		startDate = todayISO();
		fixedElectricityCost = '';
		fixedTransferCost = '';
		fixedMonthlyFee = '';
		fixedElectricityTax = '';
		fixedTransferTax = '';
		spotElectricityCost = '';
		spotTransferCost = '';
		spotMonthlyFee = '';
		spotElectricityTax = '';
		spotTransferTax = '';
		error = null;
		saved = false;
	}

	// Loads an existing contract into the form for editing — only the
	// section matching its actual pricing_model is filled in, the other
	// stays blank, mirroring how "add new" only ever fills one section too.
	function startEdit(c: Contract) {
		resetForm();
		editingId = c.id;
		startDate = c.valid_from.slice(0, 10);
		if (c.pricing_model === 'fixed') {
			fixedElectricityCost = c.energy_price_c_per_kwh ?? '';
			fixedTransferCost = c.transfer_price_c_per_kwh;
			fixedMonthlyFee = c.monthly_fee_eur;
			fixedElectricityTax = c.electricity_tax_eur;
			fixedTransferTax = c.transfer_tax_eur;
		} else {
			spotElectricityCost = c.spot_margin_c_per_kwh ?? '';
			spotTransferCost = c.transfer_price_c_per_kwh;
			spotMonthlyFee = c.monthly_fee_eur;
			spotElectricityTax = c.electricity_tax_eur;
			spotTransferTax = c.transfer_tax_eur;
		}
	}

	export function open() {
		resetForm();
		dialogEl?.showModal();
	}

	function close() {
		dialogEl?.close();
	}

	function contractSummary(c: Contract, loc: string): string {
		const rate =
			c.pricing_model === 'fixed'
				? `${formatNumber(c.energy_price_c_per_kwh ?? 0, 2, loc)} c/kWh`
				: `${$translate('settings.spotPricing')} +${formatNumber(c.spot_margin_c_per_kwh ?? 0, 2, loc)} c/kWh`;
		return `${rate} + ${formatNumber(c.transfer_price_c_per_kwh, 2, loc)} c/kWh ${$translate('settings.transferCostShort')}`;
	}

	// Empty -> 0 for the "other" fields (a contract with no monthly fee/tax
	// is normal), but the two electricity-cost fields are kept as
	// null-when-empty so "fixed left empty" can be distinguished from "0
	// c/kWh electricity" — see the pricing_model decision below.
	function toNumber(n: number | ''): number {
		return n === '' ? 0 : n;
	}
	function toNumberOrNull(n: number | ''): number | null {
		return n === '' ? null : n;
	}

	async function save() {
		error = null;
		saved = false;

		const fixedCost = toNumberOrNull(fixedElectricityCost);
		const spotMargin = toNumberOrNull(spotElectricityCost);

		let contract: NewContract;
		if (fixedCost !== null) {
			contract = {
				pricing_model: 'fixed',
				valid_from: startDate,
				energy_price_c_per_kwh: fixedCost,
				spot_margin_c_per_kwh: null,
				transfer_price_c_per_kwh: toNumber(fixedTransferCost),
				monthly_fee_eur: toNumber(fixedMonthlyFee),
				electricity_tax_eur: toNumber(fixedElectricityTax),
				transfer_tax_eur: toNumber(fixedTransferTax)
			};
		} else if (spotMargin !== null) {
			contract = {
				pricing_model: 'spot',
				valid_from: startDate,
				energy_price_c_per_kwh: null,
				spot_margin_c_per_kwh: spotMargin,
				transfer_price_c_per_kwh: toNumber(spotTransferCost),
				monthly_fee_eur: toNumber(spotMonthlyFee),
				electricity_tax_eur: toNumber(spotElectricityTax),
				transfer_tax_eur: toNumber(spotTransferTax)
			};
		} else {
			error = $translate('settings.validationError');
			return;
		}

		saving = true;
		try {
			if (editingId !== null) {
				await updateContract(editingId, contract);
			} else {
				await createContract(contract);
			}
			await contractsStore.refresh();
			saved = true;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			saving = false;
		}
	}
</script>

<dialog bind:this={dialogEl} class="settings-dialog" onclose={resetForm}>
	<form method="dialog" onsubmit={(e) => e.preventDefault()}>
		<h2>{$translate('settings.title')}</h2>

		<section class="existing">
			<h3>{$translate('settings.existingContracts')}</h3>
			{#if contractsStore.contracts.length === 0}
				<p class="hint">{$translate('settings.noContracts')}</p>
			{:else}
				<ul class="contract-list">
					{#each contractsStore.contracts as c (c.id)}
						<li>
							<span class="contract-date">{formatDate(c.valid_from.slice(0, 10), $locale ?? 'en')}</span>
							<span class="contract-summary">{contractSummary(c, $locale ?? 'en')}</span>
							<button type="button" onclick={() => startEdit(c)}>{$translate('settings.edit')}</button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<div class="form-heading">
			<h3>{editingId !== null ? $translate('settings.editingContract') : $translate('settings.newContract')}</h3>
			{#if editingId !== null}
				<button type="button" class="link" onclick={resetForm}>{$translate('settings.addNew')}</button>
			{/if}
		</div>

		<label class="start-date">
			{$translate('settings.startDate')}
			<input type="date" bind:value={startDate} />
		</label>

		<p class="hint">{$translate('settings.emptyMeansSpot')}</p>

		<div class="sections">
			<fieldset>
				<legend>{$translate('settings.fixedPricing')}</legend>
				<label>
					{$translate('settings.electricityCost')}
					<input type="number" step="0.01" bind:value={fixedElectricityCost} />
				</label>
				<label>
					{$translate('settings.transferCost')}
					<input type="number" step="0.01" bind:value={fixedTransferCost} />
				</label>
				<label>
					{$translate('settings.monthlyFee')}
					<input type="number" step="0.01" bind:value={fixedMonthlyFee} />
				</label>
				<label>
					{$translate('settings.electricityTax')}
					<input type="number" step="0.01" bind:value={fixedElectricityTax} />
				</label>
				<label>
					{$translate('settings.transferTax')}
					<input type="number" step="0.01" bind:value={fixedTransferTax} />
				</label>
			</fieldset>

			<fieldset>
				<legend>{$translate('settings.spotPricing')}</legend>
				<label>
					{$translate('settings.spotElectricityCost')}
					<input type="number" step="0.01" bind:value={spotElectricityCost} />
				</label>
				<p class="hint">{$translate('settings.spotElectricityCostHint')}</p>
				<label>
					{$translate('settings.transferCost')}
					<input type="number" step="0.01" bind:value={spotTransferCost} />
				</label>
				<label>
					{$translate('settings.monthlyFee')}
					<input type="number" step="0.01" bind:value={spotMonthlyFee} />
				</label>
				<label>
					{$translate('settings.electricityTax')}
					<input type="number" step="0.01" bind:value={spotElectricityTax} />
				</label>
				<label>
					{$translate('settings.transferTax')}
					<input type="number" step="0.01" bind:value={spotTransferTax} />
				</label>
			</fieldset>
		</div>

		{#if error}
			<p class="error">{error}</p>
		{/if}
		{#if saved}
			<p class="success">{$translate('settings.saved', { values: { date: startDate } })}</p>
		{/if}

		<div class="actions">
			<button type="button" onclick={close}>{$translate('settings.cancel')}</button>
			<button type="button" class="primary" onclick={save} disabled={saving}>
				{editingId !== null ? $translate('settings.update') : $translate('settings.save')}
			</button>
		</div>
	</form>
</dialog>

<style>
	.settings-dialog {
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 0;
		background: var(--surface-1);
		color: var(--text-primary);
		max-width: 640px;
		width: calc(100% - 32px);
	}
	.settings-dialog::backdrop {
		background: rgba(0, 0, 0, 0.4);
	}
	form {
		padding: 20px 24px 24px;
	}
	h2 {
		font-size: 16px;
		margin: 0 0 16px;
	}
	h3 {
		font-size: 13px;
		margin: 0;
	}
	.existing {
		margin-bottom: 20px;
		padding-bottom: 16px;
		border-bottom: 1px solid var(--border);
	}
	.existing h3 {
		margin-bottom: 8px;
	}
	.contract-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.contract-list li {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 12px;
		padding: 4px 0;
	}
	.contract-date {
		font-weight: 600;
		white-space: nowrap;
	}
	.contract-summary {
		color: var(--text-secondary);
		flex: 1;
	}
	.contract-list button {
		font-size: 12px;
		padding: 2px 10px;
	}
	.form-heading {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 8px;
	}
	.form-heading .link {
		border: none;
		background: none;
		color: var(--series-1);
		padding: 0;
		font-size: 12px;
		text-decoration: underline;
	}
	.start-date {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 13px;
		margin-bottom: 8px;
	}
	.hint {
		font-size: 12px;
		color: var(--text-secondary);
		margin: 0 0 16px;
	}
	.sections {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 20px;
	}
	@media (max-width: 560px) {
		.sections {
			grid-template-columns: 1fr;
		}
	}
	fieldset {
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 12px;
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	legend {
		font-size: 13px;
		font-weight: 600;
		padding: 0 4px;
	}
	fieldset label {
		display: flex;
		flex-direction: column;
		gap: 2px;
		font-size: 12px;
		color: var(--text-secondary);
	}
	input[type='number'],
	input[type='date'] {
		font-size: 13px;
		padding: 4px 6px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--surface-1);
		color: var(--text-primary);
	}
	.error {
		color: var(--text-muted);
		font-weight: 600;
		font-size: 13px;
		margin: 16px 0 0;
	}
	.success {
		color: var(--text-secondary);
		font-size: 13px;
		margin: 16px 0 0;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		margin-top: 20px;
	}
	button {
		font-size: 13px;
		padding: 6px 14px;
		border-radius: 4px;
		border: 1px solid var(--border);
		background: var(--surface-1);
		color: var(--text-primary);
		cursor: pointer;
	}
	button.primary {
		background: var(--series-1);
		color: white;
		border-color: var(--series-1);
	}
</style>
