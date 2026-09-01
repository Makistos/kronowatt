package storage

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"kronowatt/backend/internal/domain"
)

type ContractRepository struct {
	pool *pgxpool.Pool
}

func NewContractRepository(pool *pgxpool.Pool) *ContractRepository {
	return &ContractRepository{pool: pool}
}

// Create inserts a new contract and its single price period, effective
// from c.ValidFrom. This never edits an existing row — spec §6's
// "historical contract data immutable unless explicitly edited" — it only
// adds a new one, which naturally supersedes the previous contract for any
// timestamp on or after ValidFrom once List/EffectiveAt pick the most
// recent match; anything before ValidFrom keeps resolving to whatever
// contract covered it before.
func (r *ContractRepository) Create(ctx context.Context, c domain.Contract) (domain.Contract, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Contract{}, err
	}
	defer tx.Rollback(ctx)

	// supplier/contract_name are spec §6 fields the settings UI this repo
	// serves doesn't collect (it only asks for pricing, not admin
	// metadata) — filled with a placeholder rather than adding
	// UI-unused-but-required inputs.
	var contractID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO electricity_contract (supplier, contract_name, pricing_model, valid_from)
		VALUES ('', '', $1, $2)
		RETURNING id
	`, string(c.PricingModel), c.ValidFrom).Scan(&contractID); err != nil {
		return domain.Contract{}, err
	}

	p := c.Period
	if _, err := tx.Exec(ctx, `
		INSERT INTO contract_price_period
			(contract_id, period_start, energy_price, spot_margin, transfer_price, monthly_fee, taxes, transfer_tax)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, contractID, c.ValidFrom, p.EnergyPrice, p.SpotMargin, p.TransferPrice, p.MonthlyFee, p.ElectricityTax, p.TransferTax); err != nil {
		return domain.Contract{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Contract{}, err
	}

	c.ID = contractID
	return c, nil
}

// Update overwrites an existing contract's pricing_model/valid_from and its
// price period's fields in place — the explicit-edit exception to spec
// §6's "historical contract data immutable unless explicitly edited".
// Unlike Create, this can change history: editing an old contract changes
// what List/effective-at resolution returns for every timestamp it used to
// cover, which is exactly what "let the user view and change an existing
// contract" (as opposed to only ever adding a new one) means. Returns
// ErrNotFound if c.ID doesn't match an existing contract.
func (r *ContractRepository) Update(ctx context.Context, c domain.Contract) (domain.Contract, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Contract{}, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE electricity_contract SET pricing_model = $1, valid_from = $2, updated_at = now()
		WHERE id = $3
	`, string(c.PricingModel), c.ValidFrom, c.ID)
	if err != nil {
		return domain.Contract{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.Contract{}, ErrNotFound
	}

	p := c.Period
	if _, err := tx.Exec(ctx, `
		UPDATE contract_price_period
		SET period_start = $1, energy_price = $2, spot_margin = $3, transfer_price = $4,
			monthly_fee = $5, taxes = $6, transfer_tax = $7, updated_at = now()
		WHERE contract_id = $8
	`, c.ValidFrom, p.EnergyPrice, p.SpotMargin, p.TransferPrice, p.MonthlyFee, p.ElectricityTax, p.TransferTax, c.ID); err != nil {
		return domain.Contract{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Contract{}, err
	}
	return c, nil
}

// List returns every contract with its price period, ordered by ValidFrom
// ascending — small enough (a handful of rows over years) to fetch in
// full and resolve "which contract applies at timestamp T" client-side,
// the same way the frontend already matches electricity samples to spot
// price rows.
func (r *ContractRepository) List(ctx context.Context) ([]domain.Contract, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.pricing_model, c.valid_from,
			p.energy_price, p.spot_margin, p.transfer_price, p.monthly_fee, p.taxes, p.transfer_tax
		FROM electricity_contract c
		JOIN contract_price_period p ON p.contract_id = c.id AND p.period_start = c.valid_from
		ORDER BY c.valid_from ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Contract
	for rows.Next() {
		var c domain.Contract
		var pricingModel string
		var monthlyFee, taxes, transferTax *float64
		if err := rows.Scan(&c.ID, &pricingModel, &c.ValidFrom,
			&c.Period.EnergyPrice, &c.Period.SpotMargin, &c.Period.TransferPrice, &monthlyFee, &taxes, &transferTax); err != nil {
			return nil, err
		}
		c.PricingModel = domain.PricingModel(pricingModel)
		c.Period.MonthlyFee = deref(monthlyFee)
		c.Period.ElectricityTax = deref(taxes)
		c.Period.TransferTax = deref(transferTax)
		out = append(out, c)
	}
	return out, rows.Err()
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
