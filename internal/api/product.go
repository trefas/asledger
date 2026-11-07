package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// creditSettingsPatch — частичное изменение настроек кредита.
//
// Отсутствующие поля сохраняют текущие значения; для нового счёта действуют
// значения по умолчанию из store.
type creditSettingsPatch struct {
	RatePPM        *int64     `json:"rate_ppm"`
	InterestType   *string    `json:"interest_type"`
	RepaymentOrder *string    `json:"repayment_order"`
	GraceEnabled   *bool      `json:"grace_enabled"`
	GraceEndDate   *time.Time `json:"grace_end_date"`
}

// depositStepPatch — ступенчатая ставка вклада в запросе.
type depositStepPatch struct {
	FromKop money.Kop  `json:"from_kop"`
	ToKop   *money.Kop `json:"to_kop"`
	RatePPM int64      `json:"rate_ppm"`
}

// depositSettingsPatch — частичное изменение настроек вклада.
type depositSettingsPatch struct {
	BaseRatePPM      *int64     `json:"base_rate_ppm"`
	MinBalance       *money.Kop `json:"min_balance_kop"`
	BonusType        *string    `json:"bonus_type"`
	BonusRatePPM     *int64     `json:"bonus_rate_ppm"`
	BonusAmount      *money.Kop `json:"bonus_amount_kop"`
	AccrualFrequency *string    `json:"accrual_frequency"`
	Capitalization   *bool      `json:"capitalization"`
	OpenedAt         *time.Time `json:"opened_at"`
	ClosedAt         *time.Time `json:"closed_at"`
	// Steps заменяет все ступени; nil — оставить текущие, [] — удалить.
	Steps *[]depositStepPatch `json:"steps"`
}

// productSettingsDTO — настройки продукта, привязанного к счёту.
type productSettingsDTO struct {
	AccountID int64             `json:"account_id"`
	Path      string            `json:"path"`
	Product   model.ProductType `json:"product"`
	// Settings — настройки кредита или вклада; null, если они ещё не заданы.
	Settings any `json:"settings"`
}

// handleCreditSettings создаёт или частично изменяет настройки кредита.
func (s *Server) handleCreditSettings(w http.ResponseWriter, r *http.Request) {
	accountID, err := pathAccountID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	var patch creditSettingsPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	in, err := s.creditSettingsInput(r, accountID, patch)
	if err != nil {
		s.writeProductError(w, "настройки кредита", err)
		return
	}
	settings, err := s.db.SetCreditSettings(r.Context(), accountID, in)
	if err != nil {
		s.writeProductError(w, "настройки кредита", err)
		return
	}
	s.writeProductSettings(w, r, accountID, model.ProductCredit, settings)
}

// handleDepositSettings создаёт или частично изменяет настройки вклада.
func (s *Server) handleDepositSettings(w http.ResponseWriter, r *http.Request) {
	accountID, err := pathAccountID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	var patch depositSettingsPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	in, err := s.depositSettingsInput(r, accountID, patch)
	if err != nil {
		s.writeProductError(w, "настройки вклада", err)
		return
	}
	settings, err := s.db.SetDepositSettings(r.Context(), accountID, in)
	if err != nil {
		s.writeProductError(w, "настройки вклада", err)
		return
	}
	s.writeProductSettings(w, r, accountID, model.ProductDeposit, settings)
}

// handleProductSettings отдаёт настройки кредита или вклада по счёту.
func (s *Server) handleProductSettings(w http.ResponseWriter, r *http.Request) {
	accountID, err := pathAccountID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	account, err := s.db.Account(r.Context(), accountID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ERR_ACCOUNT_NOT_FOUND", "счёт не найден")
		return
	}
	if err != nil {
		s.writeProductError(w, "настройки продукта", err)
		return
	}
	product, ok := model.ProductForAccountType(account.Type)
	if !ok || !account.IsLeaf || account.IsArchived {
		writeError(w, http.StatusUnprocessableEntity, store.CodeProductAccountInvalid,
			"настройки продукта доступны только для листового счёта типа «пассив» или «актив»: "+
				account.FullPath)
		return
	}

	switch product {
	case model.ProductCredit:
		settings, err := s.db.CreditSettings(r.Context(), accountID)
		if errors.Is(err, store.ErrNotFound) {
			s.writeProductSettings(w, r, accountID, product, nil)
			return
		}
		if err != nil {
			s.writeProductError(w, "настройки кредита", err)
			return
		}
		s.writeProductSettings(w, r, accountID, product, settings)
	case model.ProductDeposit:
		settings, err := s.db.DepositSettings(r.Context(), accountID)
		if errors.Is(err, store.ErrNotFound) {
			s.writeProductSettings(w, r, accountID, product, nil)
			return
		}
		if err != nil {
			s.writeProductError(w, "настройки вклада", err)
			return
		}
		s.writeProductSettings(w, r, accountID, product, settings)
	default:
		writeError(w, http.StatusUnprocessableEntity, store.CodeProductAccountInvalid,
			"для этого счёта нет настроек продукта: "+account.FullPath)
	}
}

// writeProductSettings отправляет настройки продукта в едином формате.
func (s *Server) writeProductSettings(w http.ResponseWriter, r *http.Request,
	accountID int64, product model.ProductType, settings any) {
	account, err := s.db.Account(r.Context(), accountID)
	if err != nil {
		writeJSON(w, http.StatusOK, productSettingsDTO{
			AccountID: accountID,
			Product:   product,
			Settings:  settings,
		})
		return
	}
	writeJSON(w, http.StatusOK, productSettingsDTO{
		AccountID: accountID,
		Path:      account.FullPath,
		Product:   product,
		Settings:  settings,
	})
}

// creditSettingsInput собирает полные настройки из текущих и патча.
func (s *Server) creditSettingsInput(r *http.Request, accountID int64,
	patch creditSettingsPatch) (store.CreditSettingsInput, error) {
	var in store.CreditSettingsInput
	existing, err := s.db.CreditSettings(r.Context(), accountID)
	switch {
	case err == nil:
		in = store.CreditSettingsInput{
			RatePPM:        existing.RatePPM,
			InterestType:   existing.InterestType,
			RepaymentOrder: existing.RepaymentOrder,
			GraceEnabled:   existing.GraceEnabled,
			GraceEndDate:   existing.GraceEndDate,
		}
	case errors.Is(err, store.ErrNotFound):
		// Новый счёт: значения по умолчанию подставит store.
	default:
		return in, err
	}
	if patch.RatePPM != nil {
		in.RatePPM = *patch.RatePPM
	}
	if patch.InterestType != nil {
		in.InterestType = model.CreditInterestType(*patch.InterestType)
	}
	if patch.RepaymentOrder != nil {
		in.RepaymentOrder = model.RepaymentOrder(*patch.RepaymentOrder)
	}
	if patch.GraceEnabled != nil {
		in.GraceEnabled = *patch.GraceEnabled
		if !in.GraceEnabled && patch.GraceEndDate == nil {
			in.GraceEndDate = time.Time{}
		}
	}
	if patch.GraceEndDate != nil {
		in.GraceEndDate = *patch.GraceEndDate
	}
	return in, nil
}

// depositSettingsInput собирает полные настройки из текущих и патча.
func (s *Server) depositSettingsInput(r *http.Request, accountID int64,
	patch depositSettingsPatch) (store.DepositSettingsInput, error) {
	var in store.DepositSettingsInput
	existing, err := s.db.DepositSettings(r.Context(), accountID)
	switch {
	case err == nil:
		in = store.DepositSettingsInput{
			BaseRatePPM:      existing.BaseRatePPM,
			MinBalance:       existing.MinBalance,
			BonusType:        existing.BonusType,
			BonusRatePPM:     existing.BonusRatePPM,
			BonusAmount:      existing.BonusAmount,
			AccrualFrequency: existing.AccrualFrequency,
			Capitalization:   existing.Capitalization,
			OpenedAt:         existing.OpenedAt,
			ClosedAt:         existing.ClosedAt,
			Steps:            depositStepsInput(existing.Steps),
		}
	case errors.Is(err, store.ErrNotFound):
		// Новый счёт: значения по умолчанию подставит store.
	default:
		return in, err
	}
	if patch.BaseRatePPM != nil {
		in.BaseRatePPM = *patch.BaseRatePPM
	}
	if patch.MinBalance != nil {
		in.MinBalance = *patch.MinBalance
	}
	if patch.BonusType != nil {
		in.BonusType = model.DepositBonusType(*patch.BonusType)
	}
	if patch.BonusRatePPM != nil {
		in.BonusRatePPM = *patch.BonusRatePPM
	}
	if patch.BonusAmount != nil {
		in.BonusAmount = *patch.BonusAmount
	}
	if patch.AccrualFrequency != nil {
		in.AccrualFrequency = model.DepositAccrualFrequency(*patch.AccrualFrequency)
	}
	if patch.Capitalization != nil {
		in.Capitalization = *patch.Capitalization
	}
	if patch.OpenedAt != nil {
		in.OpenedAt = *patch.OpenedAt
	}
	if patch.ClosedAt != nil {
		in.ClosedAt = *patch.ClosedAt
	}
	if patch.Steps != nil {
		in.Steps = make([]store.DepositStepRateInput, 0, len(*patch.Steps))
		for _, step := range *patch.Steps {
			in.Steps = append(in.Steps, store.DepositStepRateInput{
				From:    step.FromKop,
				To:      step.ToKop,
				RatePPM: step.RatePPM,
			})
		}
	}
	return in, nil
}

// depositStepsInput переводит сохранённые ступени в данные store.
func depositStepsInput(steps []model.DepositStepRate) []store.DepositStepRateInput {
	out := make([]store.DepositStepRateInput, 0, len(steps))
	for _, step := range steps {
		out = append(out, store.DepositStepRateInput{
			From:    step.From,
			To:      step.To,
			RatePPM: step.RatePPM,
		})
	}
	return out
}

// pathAccountID читает идентификатор счёта из пути.
func pathAccountID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("неверный идентификатор счёта")
	}
	return id, nil
}

// writeProductError отдаёт ошибку настроек продукта в едином конверте API.
func (s *Server) writeProductError(w http.ResponseWriter, what string, err error) {
	if writeEngineError(w, err) {
		return
	}
	s.logger.Error("не удалось сохранить "+what, "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}
