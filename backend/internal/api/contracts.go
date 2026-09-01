package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"kronowatt/backend/internal/domain"
	"kronowatt/backend/internal/storage"
)

// contractDTO's field names spell out the unit in every price field
// (c_per_kwh vs eur) since, unlike spot_price's €/MWh market unit, this
// data has no external wire format to defer to — the settings UI collects
// and displays these exact units, so the API shouldn't force a conversion
// at the boundary.
type contractDTO struct {
	ID                   int64    `json:"id"`
	PricingModel         string   `json:"pricing_model"`
	ValidFrom            string   `json:"valid_from"`
	EnergyPriceCPerKWh   *float64 `json:"energy_price_c_per_kwh"`
	SpotMarginCPerKWh    *float64 `json:"spot_margin_c_per_kwh"`
	TransferPriceCPerKWh float64  `json:"transfer_price_c_per_kwh"`
	MonthlyFeeEUR        float64  `json:"monthly_fee_eur"`
	ElectricityTaxEUR    float64  `json:"electricity_tax_eur"`
	TransferTaxEUR       float64  `json:"transfer_tax_eur"`
}

func toContractDTO(c domain.Contract) contractDTO {
	return contractDTO{
		ID:                   c.ID,
		PricingModel:         string(c.PricingModel),
		ValidFrom:            c.ValidFrom.UTC().Format(time.RFC3339),
		EnergyPriceCPerKWh:   c.Period.EnergyPrice,
		SpotMarginCPerKWh:    c.Period.SpotMargin,
		TransferPriceCPerKWh: c.Period.TransferPrice,
		MonthlyFeeEUR:        c.Period.MonthlyFee,
		ElectricityTaxEUR:    c.Period.ElectricityTax,
		TransferTaxEUR:       c.Period.TransferTax,
	}
}

// contractsHandler serves /api/contracts (list + create); contractHandler
// (below) serves /api/contracts/{id} (update) — split because Go's
// net/http mux dispatches on the literal path, and "view and change an
// existing contract" needs to address one by id while "add a new one"
// doesn't.
func contractsHandler(repo *storage.ContractRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listContracts(w, r, repo)
		case http.MethodPost:
			createContract(w, r, repo)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func contractHandler(repo *storage.ContractRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			updateContract(w, r, repo)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listContracts(w http.ResponseWriter, r *http.Request, repo *storage.ContractRepository) {
	rows, err := repo.List(r.Context())
	if err != nil {
		http.Error(w, "failed to load contracts", http.StatusInternalServerError)
		return
	}

	dtos := make([]contractDTO, len(rows))
	for i, c := range rows {
		dtos[i] = toContractDTO(c)
	}
	writeJSON(w, http.StatusOK, dtos)
}

type contractRequest struct {
	ValidFrom            string   `json:"valid_from"`
	PricingModel         string   `json:"pricing_model"`
	EnergyPriceCPerKWh   *float64 `json:"energy_price_c_per_kwh"`
	SpotMarginCPerKWh    *float64 `json:"spot_margin_c_per_kwh"`
	TransferPriceCPerKWh float64  `json:"transfer_price_c_per_kwh"`
	MonthlyFeeEUR        float64  `json:"monthly_fee_eur"`
	ElectricityTaxEUR    float64  `json:"electricity_tax_eur"`
	TransferTaxEUR       float64  `json:"transfer_tax_eur"`
}

// parseContract decodes and validates a create/update request body into a
// domain.Contract (ID left zero — callers fill it in for an update). On
// error it has already written the HTTP response; callers must return
// immediately without writing anything else.
func parseContract(w http.ResponseWriter, r *http.Request) (domain.Contract, bool) {
	var req contractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return domain.Contract{}, false
	}

	validFrom, err := time.Parse(time.RFC3339, req.ValidFrom)
	if err != nil {
		// Also accept a bare date ("2026-09-01"), what <input type="date">
		// sends — treated as UTC midnight, matching how the rest of the API
		// parses ?start=/?end= dates in daterange.go.
		validFrom, err = time.Parse("2006-01-02", req.ValidFrom)
		if err != nil {
			http.Error(w, "invalid valid_from", http.StatusBadRequest)
			return domain.Contract{}, false
		}
	}

	model := domain.PricingModel(req.PricingModel)
	switch model {
	case domain.PricingModelFixed:
		if req.EnergyPriceCPerKWh == nil {
			http.Error(w, "energy_price_c_per_kwh is required for a fixed contract", http.StatusBadRequest)
			return domain.Contract{}, false
		}
	case domain.PricingModelSpot:
		if req.SpotMarginCPerKWh == nil {
			http.Error(w, "spot_margin_c_per_kwh is required for a spot contract", http.StatusBadRequest)
			return domain.Contract{}, false
		}
	default:
		http.Error(w, `pricing_model must be "fixed" or "spot"`, http.StatusBadRequest)
		return domain.Contract{}, false
	}

	return domain.Contract{
		PricingModel: model,
		ValidFrom:    validFrom,
		Period: domain.ContractPeriod{
			EnergyPrice:    req.EnergyPriceCPerKWh,
			SpotMargin:     req.SpotMarginCPerKWh,
			TransferPrice:  req.TransferPriceCPerKWh,
			MonthlyFee:     req.MonthlyFeeEUR,
			ElectricityTax: req.ElectricityTaxEUR,
			TransferTax:    req.TransferTaxEUR,
		},
	}, true
}

func createContract(w http.ResponseWriter, r *http.Request, repo *storage.ContractRepository) {
	contract, ok := parseContract(w, r)
	if !ok {
		return
	}

	created, err := repo.Create(r.Context(), contract)
	if err != nil {
		http.Error(w, "failed to create contract", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, toContractDTO(created))
}

func updateContract(w http.ResponseWriter, r *http.Request, repo *storage.ContractRepository) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid contract id", http.StatusBadRequest)
		return
	}

	contract, ok := parseContract(w, r)
	if !ok {
		return
	}
	contract.ID = id

	updated, err := repo.Update(r.Context(), contract)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "contract not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update contract", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, toContractDTO(updated))
}
