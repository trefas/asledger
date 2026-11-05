package api

import (
	"net/http"
	"time"

	creditsvc "github.com/trefas/asledger/pkg/core/credit"
	"github.com/trefas/asledger/pkg/core/money"
)

// creditRepaymentRequest — запрос на погашение кредита.
//
// Клиент передаёт только счета и сумму: разбивку на тело и проценты считает
// сервер по настройкам кредита.
type creditRepaymentRequest struct {
	// UUID — идентификатор проводки; повтор с тем же uuid идемпотентен.
	UUID string `json:"uuid,omitempty"`
	// Date — дата погашения.
	Date time.Time `json:"date"`
	// AccountID — кредитный счёт (листовой пассив).
	AccountID int64 `json:"account_id"`
	// SourceAccountID — счёт-источник (листовой актив).
	SourceAccountID int64 `json:"source_account_id"`
	// InterestAccountID — статья расходов на проценты.
	InterestAccountID int64 `json:"interest_account_id,omitempty"`
	// Amount — сумма погашения в копейках.
	Amount money.Kop `json:"amount_kop"`
	// Description — описание проводки.
	Description string `json:"description,omitempty"`
}

// creditPreviewDTO — разбивка платежа до подтверждения.
type creditPreviewDTO struct {
	UUID                 string    `json:"uuid"`
	Date                 time.Time `json:"date"`
	AccountID            int64     `json:"account_id"`
	SourceAccountID      int64     `json:"source_account_id"`
	InterestAccountID    int64     `json:"interest_account_id,omitempty"`
	RequestedKop         money.Kop `json:"amount_kop"`
	PrincipalKop         money.Kop `json:"principal_kop"`
	InterestKop          money.Kop `json:"interest_kop"`
	TotalKop             money.Kop `json:"total_kop"`
	DebtKop              money.Kop `json:"debt_kop"`
	AccruedInterestKop   money.Kop `json:"accrued_interest_kop"`
	RemainingDebtKop     money.Kop `json:"remaining_debt_kop"`
	RemainingInterestKop money.Kop `json:"remaining_interest_kop"`
	InGrace              bool      `json:"in_grace"`
}

// creditRepaymentDTO — результат проведённого погашения.
type creditRepaymentDTO struct {
	UUID         string         `json:"uuid"`
	PrincipalKop money.Kop      `json:"principal_kop"`
	InterestKop  money.Kop      `json:"interest_kop"`
	TotalKop     money.Kop      `json:"total_kop"`
	Transaction  transactionDTO `json:"transaction"`
}

// input переводит запрос в данные сервиса.
func (req creditRepaymentRequest) input(actorID int64, source string) creditsvc.RepaymentInput {
	return creditsvc.RepaymentInput{
		UUID:              req.UUID,
		Date:              req.Date,
		AccountID:         req.AccountID,
		SourceAccountID:   req.SourceAccountID,
		InterestAccountID: req.InterestAccountID,
		Amount:            req.Amount,
		Description:       req.Description,
		ActorID:           actorID,
		Source:            source,
	}
}

// toCreditPreviewDTO переводит предпросмотр в формат ответа.
func toCreditPreviewDTO(preview *creditsvc.Preview) creditPreviewDTO {
	return creditPreviewDTO{
		UUID:                 preview.UUID,
		Date:                 preview.Date,
		AccountID:            preview.AccountID,
		SourceAccountID:      preview.SourceAccountID,
		InterestAccountID:    preview.InterestAccountID,
		RequestedKop:         preview.Requested,
		PrincipalKop:         preview.Principal,
		InterestKop:          preview.Interest,
		TotalKop:             preview.Total,
		DebtKop:              preview.Debt,
		AccruedInterestKop:   preview.AccruedInterest,
		RemainingDebtKop:     preview.RemainingDebt,
		RemainingInterestKop: preview.RemainingInterest,
		InGrace:              preview.InGrace,
	}
}

// toCreditRepaymentDTO переводит результат погашения в формат ответа.
func toCreditRepaymentDTO(posted *creditsvc.Posted) creditRepaymentDTO {
	return creditRepaymentDTO{
		UUID:         posted.UUID,
		PrincipalKop: posted.Principal,
		InterestKop:  posted.Interest,
		TotalKop:     posted.Total,
		Transaction:  toTransactionDTO(posted.Transaction),
	}
}

// handleCreditRepaymentPreview рассчитывает разбивку платежа без записи.
func (s *Server) handleCreditRepaymentPreview(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req creditRepaymentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}

	preview, err := s.credit.Preview(r.Context(), req.input(device.UserID, "api:"+device.ClientID))
	if err != nil {
		s.writeCreditError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCreditPreviewDTO(preview))
}

// handleCreditRepayment проводит погашение одной составной проводкой.
func (s *Server) handleCreditRepayment(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req creditRepaymentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}

	posted, err := s.credit.Post(r.Context(), req.input(device.UserID, "api:"+device.ClientID))
	if err != nil {
		s.writeCreditError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCreditRepaymentDTO(posted))
}

// writeCreditError отдаёт ошибку погашения в едином конверте API.
func (s *Server) writeCreditError(w http.ResponseWriter, err error) {
	if writeEngineError(w, err) {
		return
	}
	s.logger.Error("не удалось выполнить погашение кредита", "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}
