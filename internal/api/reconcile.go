package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/money"
	reconcilesvc "github.com/trefas/asledger/pkg/core/reconcile"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/uuid"
)

// reconcileRequest — запрос сверки счёта.
type reconcileRequest struct {
	// AccountID — сверяемый счёт.
	AccountID int64 `json:"account_id"`
	// FactKop — фактический остаток в копейках.
	FactKop money.Kop `json:"fact_kop"`
	// Date — дата корректировки (ГГГГ-ММ-ДД); по умолчанию — текущая.
	Date string `json:"date,omitempty"`
	// Generate — сгенерировать корректировку на расхождение.
	Generate bool `json:"generate"`
	// UUID — идентификатор корректировки (клиент может задать свой).
	UUID string `json:"uuid,omitempty"`
}

// reconcileResultDTO — результат сверки.
type reconcileResultDTO struct {
	// AccountID и Path — сверенный счёт.
	AccountID  int64     `json:"account_id"`
	Path       string    `json:"path"`
	FactKop    money.Kop `json:"fact_kop"`
	BalanceKop money.Kop `json:"balance_kop"`
	DiffKop    money.Kop `json:"difference_kop"`
	Fact       string    `json:"fact"`
	Balance    string    `json:"balance"`
	Difference string    `json:"difference"`
	CheckedAt  string    `json:"checked_at"`
	// Transaction — сгенерированная корректировка (если запрошена).
	Transaction *transactionDTO `json:"transaction,omitempty"`
}

// handleReconcile выполняет сверку счёта и при необходимости создаёт корректировку.
func (s *Server) handleReconcile(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req reconcileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	if req.AccountID <= 0 {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не задан счёт сверки")
		return
	}

	date := time.Now().UTC()
	if text := strings.TrimSpace(req.Date); text != "" {
		parsed, err := time.Parse(time.DateOnly, text)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "неверная дата сверки")
			return
		}
		date = parsed.UTC()
	}

	adjustmentUUID := req.UUID
	if req.Generate && adjustmentUUID == "" {
		generated, err := uuid.New()
		if err != nil {
			s.logger.Error("не удалось создать uuid корректировки", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		adjustmentUUID = generated
	}

	result, err := s.reconcile.Check(r.Context(), reconcilesvc.Request{
		AccountID: req.AccountID,
		Fact:      req.FactKop,
		Date:      date,
		ActorID:   device.UserID,
		Generate:  req.Generate,
		UUID:      adjustmentUUID,
		Source:    "api:" + device.ClientID,
	})
	if errors.Is(err, reconcilesvc.ErrNotAdjustable) {
		writeError(w, http.StatusUnprocessableEntity, "ERR_RECONCILE_NOT_ADJUSTABLE",
			"счёт этого типа нельзя скорректировать сверкой")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ERR_ACCOUNT_NOT_FOUND", "счёт не найден")
		return
	}
	if err != nil {
		if writeEngineError(w, err) {
			return
		}
		s.logger.Error("не удалось выполнить сверку", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	account, err := s.db.Account(r.Context(), req.AccountID)
	if err != nil {
		s.logger.Error("не удалось прочитать счёт сверки", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	dto := reconcileResultDTO{
		AccountID:  account.ID,
		Path:       account.FullPath,
		FactKop:    result.Reconciliation.Fact,
		BalanceKop: result.Reconciliation.Expected,
		DiffKop:    result.Difference,
		Fact:       result.Reconciliation.Fact.String(),
		Balance:    result.Reconciliation.Expected.String(),
		Difference: result.Difference.String(),
		CheckedAt:  result.Reconciliation.CheckedAt.UTC().Format(time.RFC3339),
	}
	if result.Transaction != nil {
		transaction := toTransactionDTO(result.Transaction)
		dto.Transaction = &transaction
		// Текущий расчётный остаток после корректировки совпадает с фактом.
		dto.BalanceKop = result.Reconciliation.Fact
		dto.Balance = result.Reconciliation.Fact.String()
	}
	writeJSON(w, http.StatusOK, dto)
}
