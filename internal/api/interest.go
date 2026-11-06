package api

import (
	"net/http"
	"time"

	accrualsvc "github.com/trefas/asledger/pkg/core/accrual"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// interestRequest — запрос на расчёт или проведение начисления процентов.
type interestRequest struct {
	// UUID — идентификатор ручного начисления; повтор идемпотентен.
	UUID string `json:"uuid,omitempty"`
	// AccountID — счёт кредита или вклада.
	AccountID int64 `json:"account_id"`
	// Kind — вид начисления; пустой означает «по типу счёта».
	Kind model.InterestAccrualKind `json:"kind,omitempty"`
	// Date — дата ручного начисления.
	Date time.Time `json:"date,omitempty"`
	// PeriodFrom и PeriodTo — период периодического начисления.
	PeriodFrom time.Time `json:"period_from,omitempty"`
	PeriodTo   time.Time `json:"period_to,omitempty"`
	// Amount — сумма ручного начисления в копейках.
	Amount money.Kop `json:"amount_kop,omitempty"`
	// IncomeAccountID — статья доходов для капитализации вклада.
	IncomeAccountID int64 `json:"income_account_id,omitempty"`
	// ExpenseAccountID — статья расходов для процентов по кредиту.
	ExpenseAccountID int64 `json:"expense_account_id,omitempty"`
	// Description — описание проводки.
	Description string `json:"description,omitempty"`
}

// interestAccrualDTO — одно начисление в ответе.
type interestAccrualDTO struct {
	UUID          string                      `json:"uuid"`
	Date          time.Time                   `json:"date"`
	Kind          model.InterestAccrualKind   `json:"kind"`
	AmountKop     money.Kop                   `json:"amount_kop"`
	BaseAmountKop money.Kop                   `json:"base_amount_kop,omitempty"`
	RatePPM       int64                       `json:"rate_ppm"`
	PeriodFrom    *time.Time                  `json:"period_from,omitempty"`
	PeriodTo      *time.Time                  `json:"period_to,omitempty"`
	Status        model.InterestAccrualStatus `json:"status"`
	Existing      bool                        `json:"existing,omitempty"`
}

// interestPreviewDTO — результат расчёта начислений.
type interestPreviewDTO struct {
	AccountID      int64                     `json:"account_id"`
	Kind           model.InterestAccrualKind `json:"kind,omitempty"`
	PeriodFrom     *time.Time                `json:"period_from,omitempty"`
	PeriodTo       *time.Time                `json:"period_to,omitempty"`
	Capitalization bool                      `json:"capitalization"`
	TotalKop       money.Kop                 `json:"total_kop"`
	Accruals       []interestAccrualDTO      `json:"accruals"`
}

// interestPostedItemDTO — сохранённое начисление и его проводка.
type interestPostedItemDTO struct {
	Accrual     interestAccrualDTO `json:"accrual"`
	Transaction *transactionDTO    `json:"transaction,omitempty"`
	Existing    bool               `json:"existing,omitempty"`
}

// interestPostedDTO — результат проведения начислений.
type interestPostedDTO struct {
	AccountID      int64                     `json:"account_id"`
	Kind           model.InterestAccrualKind `json:"kind,omitempty"`
	PeriodFrom     *time.Time                `json:"period_from,omitempty"`
	PeriodTo       *time.Time                `json:"period_to,omitempty"`
	Capitalization bool                      `json:"capitalization"`
	TotalKop       money.Kop                 `json:"total_kop"`
	AppliedKop     money.Kop                 `json:"applied_kop"`
	Accruals       []interestPostedItemDTO   `json:"accruals"`
}

// input переводит запрос в данные сервиса начислений.
func (req interestRequest) input(actorID int64, source string) accrualsvc.Input {
	return accrualsvc.Input{
		UUID:             req.UUID,
		AccountID:        req.AccountID,
		Kind:             req.Kind,
		Date:             req.Date,
		PeriodFrom:       req.PeriodFrom,
		PeriodTo:         req.PeriodTo,
		Amount:           req.Amount,
		IncomeAccountID:  req.IncomeAccountID,
		ExpenseAccountID: req.ExpenseAccountID,
		Description:      req.Description,
		ActorID:          actorID,
		Source:           source,
	}
}

// timePtr возвращает указатель на время или nil для нулевого времени.
func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	value := t.UTC()
	return &value
}

// toInterestAccrualDTO переводит рассчитанное начисление в ответ.
func toInterestAccrualDTO(item accrualsvc.Item) interestAccrualDTO {
	return interestAccrualDTO{
		UUID:          item.UUID,
		Date:          item.Date.UTC(),
		Kind:          item.Kind,
		AmountKop:     item.Amount,
		BaseAmountKop: item.Base,
		RatePPM:       item.RatePPM,
		PeriodFrom:    timePtr(item.PeriodFrom),
		PeriodTo:      timePtr(item.PeriodTo),
		Status:        item.Status,
		Existing:      item.Existing,
	}
}

// toInterestAccrualModelDTO переводит сохранённое начисление в ответ.
func toInterestAccrualModelDTO(accrual *model.InterestAccrual) interestAccrualDTO {
	return interestAccrualDTO{
		UUID:          accrual.UUID,
		Date:          accrual.Date.UTC(),
		Kind:          accrual.Kind,
		AmountKop:     accrual.Amount,
		BaseAmountKop: accrual.BaseAmount,
		RatePPM:       accrual.RatePPM,
		PeriodFrom:    timePtr(accrual.PeriodFrom),
		PeriodTo:      timePtr(accrual.PeriodTo),
		Status:        accrual.Status,
	}
}

// toInterestPreviewDTO переводит предпросмотр в формат ответа.
func toInterestPreviewDTO(preview *accrualsvc.Preview) interestPreviewDTO {
	dto := interestPreviewDTO{
		AccountID:      preview.AccountID,
		Kind:           preview.Kind,
		PeriodFrom:     timePtr(preview.PeriodFrom),
		PeriodTo:       timePtr(preview.PeriodTo),
		Capitalization: preview.Capitalization,
		TotalKop:       preview.Total,
		Accruals:       make([]interestAccrualDTO, 0, len(preview.Accruals)),
	}
	for _, item := range preview.Accruals {
		dto.Accruals = append(dto.Accruals, toInterestAccrualDTO(item))
	}
	return dto
}

// toInterestPostedDTO переводит результат проведения в формат ответа.
func toInterestPostedDTO(posted *accrualsvc.Posted) interestPostedDTO {
	dto := interestPostedDTO{
		AccountID:      posted.AccountID,
		Kind:           posted.Kind,
		PeriodFrom:     timePtr(posted.PeriodFrom),
		PeriodTo:       timePtr(posted.PeriodTo),
		Capitalization: posted.Capitalization,
		TotalKop:       posted.Total,
		AppliedKop:     posted.Applied,
		Accruals:       make([]interestPostedItemDTO, 0, len(posted.Accruals)),
	}
	for _, item := range posted.Accruals {
		entry := interestPostedItemDTO{
			Accrual:  toInterestAccrualModelDTO(item.Accrual),
			Existing: item.Existing,
		}
		if item.Transaction != nil {
			transaction := toTransactionDTO(item.Transaction)
			entry.Transaction = &transaction
		}
		dto.Accruals = append(dto.Accruals, entry)
	}
	return dto
}

// handleInterestCalculate рассчитывает начисления без записи.
func (s *Server) handleInterestCalculate(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req interestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}

	preview, err := s.accrual.Calculate(r.Context(), req.input(device.UserID, "api:"+device.ClientID))
	if err != nil {
		s.writeInterestError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toInterestPreviewDTO(preview))
}

// handleInterestApply рассчитывает и записывает начисления.
func (s *Server) handleInterestApply(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req interestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}

	posted, err := s.accrual.Apply(r.Context(), req.input(device.UserID, "api:"+device.ClientID))
	if err != nil {
		s.writeInterestError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toInterestPostedDTO(posted))
}

// writeInterestError отдаёт ошибку начисления в едином конверте API.
func (s *Server) writeInterestError(w http.ResponseWriter, err error) {
	if writeEngineError(w, err) {
		return
	}
	s.logger.Error("не удалось начислить проценты", "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}
