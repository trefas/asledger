package api

import (
	"net/http"
	"time"

	depositsvc "github.com/trefas/asledger/pkg/core/deposit"
	"github.com/trefas/asledger/pkg/core/money"
)

// depositWithdrawalRequest — запрос на снятие с вклада.
//
// Клиент передаёт счёт вклада, получателя, статью доходов и сумму: разбивку
// на тело и проценты считает сервер.
type depositWithdrawalRequest struct {
	// UUID — базовый идентификатор операции; повтор идемпотентен.
	UUID string `json:"uuid,omitempty"`
	// Date — дата снятия.
	Date time.Time `json:"date"`
	// AccountID — счёт вклада (листовой актив).
	AccountID int64 `json:"account_id"`
	// DestinationAccountID — счёт-получатель (листовой актив).
	DestinationAccountID int64 `json:"destination_account_id"`
	// IncomeAccountID — статья доходов по процентам (листовой доход).
	IncomeAccountID int64 `json:"income_account_id,omitempty"`
	// Amount — сумма снятия в копейках.
	Amount money.Kop `json:"amount_kop"`
	// Description — описание проводок.
	Description string `json:"description,omitempty"`
}

// depositPreviewDTO — разбивка снятия до подтверждения.
type depositPreviewDTO struct {
	UUID                 string    `json:"uuid"`
	Date                 time.Time `json:"date"`
	AccountID            int64     `json:"account_id"`
	DestinationAccountID int64     `json:"destination_account_id"`
	IncomeAccountID      int64     `json:"income_account_id,omitempty"`
	RequestedKop         money.Kop `json:"amount_kop"`
	PrincipalKop         money.Kop `json:"principal_kop"`
	InterestKop          money.Kop `json:"interest_kop"`
	TotalKop             money.Kop `json:"total_kop"`
	BalanceKop           money.Kop `json:"balance_kop"`
	AccruedInterestKop   money.Kop `json:"accrued_interest_kop"`
	RemainingBalanceKop  money.Kop `json:"remaining_balance_kop"`
	RemainingInterestKop money.Kop `json:"remaining_interest_kop"`
}

// depositWithdrawalDTO — результат проведённого снятия.
type depositWithdrawalDTO struct {
	UUID                 string          `json:"uuid"`
	PrincipalKop         money.Kop       `json:"principal_kop"`
	InterestKop          money.Kop       `json:"interest_kop"`
	TotalKop             money.Kop       `json:"total_kop"`
	PrincipalTransaction transactionDTO  `json:"principal_transaction"`
	InterestTransaction  *transactionDTO `json:"interest_transaction,omitempty"`
}

// input переводит запрос в данные сервиса снятия.
func (req depositWithdrawalRequest) input(actorID int64, source string) depositsvc.WithdrawalInput {
	return depositsvc.WithdrawalInput{
		UUID:                 req.UUID,
		Date:                 req.Date,
		AccountID:            req.AccountID,
		DestinationAccountID: req.DestinationAccountID,
		IncomeAccountID:      req.IncomeAccountID,
		Amount:               req.Amount,
		Description:          req.Description,
		ActorID:              actorID,
		Source:               source,
	}
}

// toDepositPreviewDTO переводит предпросмотр в формат ответа.
func toDepositPreviewDTO(preview *depositsvc.Preview) depositPreviewDTO {
	return depositPreviewDTO{
		UUID:                 preview.UUID,
		Date:                 preview.Date,
		AccountID:            preview.AccountID,
		DestinationAccountID: preview.DestinationAccountID,
		IncomeAccountID:      preview.IncomeAccountID,
		RequestedKop:         preview.Requested,
		PrincipalKop:         preview.Principal,
		InterestKop:          preview.Interest,
		TotalKop:             preview.Total,
		BalanceKop:           preview.Balance,
		AccruedInterestKop:   preview.AccruedInterest,
		RemainingBalanceKop:  preview.RemainingBalance,
		RemainingInterestKop: preview.RemainingInterest,
	}
}

// toDepositWithdrawalDTO переводит результат снятия в формат ответа.
func toDepositWithdrawalDTO(posted *depositsvc.Posted) depositWithdrawalDTO {
	dto := depositWithdrawalDTO{
		UUID:         posted.UUID,
		PrincipalKop: posted.Principal,
		InterestKop:  posted.Interest,
		TotalKop:     posted.Total,
	}
	if posted.PrincipalTransaction != nil {
		dto.PrincipalTransaction = toTransactionDTO(posted.PrincipalTransaction)
	}
	if posted.InterestTransaction != nil {
		interest := toTransactionDTO(posted.InterestTransaction)
		dto.InterestTransaction = &interest
	}
	return dto
}

// handleDepositWithdrawalPreview рассчитывает разбивку снятия без записи.
func (s *Server) handleDepositWithdrawalPreview(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req depositWithdrawalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}

	preview, err := s.deposit.Preview(r.Context(), req.input(device.UserID, "api:"+device.ClientID))
	if err != nil {
		s.writeDepositError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDepositPreviewDTO(preview))
}

// handleDepositWithdrawal проводит снятие двумя проводками.
func (s *Server) handleDepositWithdrawal(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req depositWithdrawalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}

	posted, err := s.deposit.Post(r.Context(), req.input(device.UserID, "api:"+device.ClientID))
	if err != nil {
		s.writeDepositError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDepositWithdrawalDTO(posted))
}

// writeDepositError отдаёт ошибку снятия в едином конверте API.
func (s *Server) writeDepositError(w http.ResponseWriter, err error) {
	if writeEngineError(w, err) {
		return
	}
	s.logger.Error("не удалось выполнить снятие с вклада", "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}
