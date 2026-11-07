package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/report"
)

// upcomingAccrualDTO — предстоящее начисление процентов или бонуса.
type upcomingAccrualDTO struct {
	Date          string                    `json:"date"`
	Kind          model.InterestAccrualKind `json:"kind"`
	AmountKop     money.Kop                 `json:"amount_kop"`
	BaseAmountKop money.Kop                 `json:"base_amount_kop"`
	RatePPM       int64                     `json:"rate_ppm"`
	Days          int                       `json:"days"`
}

// creditSummaryRowDTO — состояние одного кредитного счёта.
type creditSummaryRowDTO struct {
	AccountID           int64                    `json:"account_id"`
	Path                string                   `json:"path"`
	Name                string                   `json:"name"`
	RatePPM             int64                    `json:"rate_ppm"`
	InterestType        model.CreditInterestType `json:"interest_type"`
	RepaymentOrder      model.RepaymentOrder     `json:"repayment_order"`
	GraceEnabled        bool                     `json:"grace_enabled"`
	GraceEndDate        string                   `json:"grace_end_date,omitempty"`
	InGrace             bool                     `json:"in_grace"`
	DebtKop             money.Kop                `json:"debt_kop"`
	AccruedInterestKop  money.Kop                `json:"accrued_interest_kop"`
	TotalDebtKop        money.Kop                `json:"total_debt_kop"`
	Upcoming            []upcomingAccrualDTO     `json:"upcoming"`
	UpcomingInterestKop money.Kop                `json:"upcoming_interest_kop"`
}

// creditSummaryDTO — отчёт по всем кредитным счетам.
type creditSummaryDTO struct {
	AsOf                string                `json:"as_of"`
	HorizonDays         int                   `json:"horizon_days"`
	Rows                []creditSummaryRowDTO `json:"rows"`
	DebtKop             money.Kop             `json:"debt_kop"`
	AccruedInterestKop  money.Kop             `json:"accrued_interest_kop"`
	TotalDebtKop        money.Kop             `json:"total_debt_kop"`
	UpcomingInterestKop money.Kop             `json:"upcoming_interest_kop"`
}

// depositSummaryRowDTO — состояние одного вклада.
type depositSummaryRowDTO struct {
	AccountID           int64                         `json:"account_id"`
	Path                string                        `json:"path"`
	Name                string                        `json:"name"`
	BaseRatePPM         int64                         `json:"base_rate_ppm"`
	BonusType           model.DepositBonusType        `json:"bonus_type"`
	AccrualFrequency    model.DepositAccrualFrequency `json:"accrual_frequency"`
	Capitalization      bool                          `json:"capitalization"`
	BalanceKop          money.Kop                     `json:"balance_kop"`
	AccruedInterestKop  money.Kop                     `json:"accrued_interest_kop"`
	TotalKop            money.Kop                     `json:"total_kop"`
	Upcoming            []upcomingAccrualDTO          `json:"upcoming"`
	UpcomingInterestKop money.Kop                     `json:"upcoming_interest_kop"`
	UpcomingBonusKop    money.Kop                     `json:"upcoming_bonus_kop"`
	UpcomingTotalKop    money.Kop                     `json:"upcoming_total_kop"`
}

// depositSummaryDTO — отчёт по всем вкладам.
type depositSummaryDTO struct {
	AsOf                string                 `json:"as_of"`
	HorizonDays         int                    `json:"horizon_days"`
	Rows                []depositSummaryRowDTO `json:"rows"`
	BalanceKop          money.Kop              `json:"balance_kop"`
	AccruedInterestKop  money.Kop              `json:"accrued_interest_kop"`
	TotalKop            money.Kop              `json:"total_kop"`
	UpcomingInterestKop money.Kop              `json:"upcoming_interest_kop"`
	UpcomingBonusKop    money.Kop              `json:"upcoming_bonus_kop"`
	UpcomingTotalKop    money.Kop              `json:"upcoming_total_kop"`
}

// creditForecastDTO — прогноз изменения долга и процентов после платежа.
type creditForecastDTO struct {
	AccountID                int64                `json:"account_id"`
	Path                     string               `json:"path"`
	Name                     string               `json:"name"`
	Date                     string               `json:"date"`
	HorizonDays              int                  `json:"horizon_days"`
	RequestedKop             money.Kop            `json:"requested_kop"`
	PrincipalKop             money.Kop            `json:"principal_kop"`
	InterestKop              money.Kop            `json:"interest_kop"`
	TotalKop                 money.Kop            `json:"total_kop"`
	DebtBeforeKop            money.Kop            `json:"debt_before_kop"`
	AccruedInterestBeforeKop money.Kop            `json:"accrued_interest_before_kop"`
	TotalDebtBeforeKop       money.Kop            `json:"total_debt_before_kop"`
	DebtAfterKop             money.Kop            `json:"debt_after_kop"`
	AccruedInterestAfterKop  money.Kop            `json:"accrued_interest_after_kop"`
	TotalDebtAfterKop        money.Kop            `json:"total_debt_after_kop"`
	DebtChangeKop            money.Kop            `json:"debt_change_kop"`
	InterestChangeKop        money.Kop            `json:"interest_change_kop"`
	InGrace                  bool                 `json:"in_grace"`
	Upcoming                 []upcomingAccrualDTO `json:"upcoming"`
	UpcomingInterestKop      money.Kop            `json:"upcoming_interest_kop"`
}

// handleReportsCreditSummary отдаёт состояние кредитных счетов.
func (s *Server) handleReportsCreditSummary(w http.ResponseWriter, r *http.Request) {
	opts, err := productOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	rep, err := report.CreditSummary(r.Context(), s.db, opts)
	if err != nil {
		s.writeProductReportError(w, "сводка по кредитам", err)
		return
	}
	writeJSON(w, http.StatusOK, creditSummary(rep))
}

// handleReportsDepositSummary отдаёт состояние вкладов.
func (s *Server) handleReportsDepositSummary(w http.ResponseWriter, r *http.Request) {
	opts, err := productOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	rep, err := report.DepositSummary(r.Context(), s.db, opts)
	if err != nil {
		s.writeProductReportError(w, "сводка по вкладам", err)
		return
	}
	writeJSON(w, http.StatusOK, depositSummary(rep))
}

// handleReportsCreditForecast отдаёт прогноз погашения кредита.
func (s *Server) handleReportsCreditForecast(w http.ResponseWriter, r *http.Request) {
	opts, err := creditForecastOptions(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	forecast, err := report.CreditForecast(r.Context(), s.db, opts)
	if err != nil {
		s.writeProductReportError(w, "прогноз по кредиту", err)
		return
	}
	writeJSON(w, http.StatusOK, creditForecast(forecast))
}

// creditSummary переводит отчёт по кредитам в DTO.
func creditSummary(rep *report.CreditSummaryReport) creditSummaryDTO {
	dto := creditSummaryDTO{
		AsOf:                reportDate(rep.AsOf),
		HorizonDays:         rep.HorizonDays,
		Rows:                make([]creditSummaryRowDTO, 0, len(rep.Rows)),
		DebtKop:             rep.Debt,
		AccruedInterestKop:  rep.AccruedInterest,
		TotalDebtKop:        rep.TotalDebt,
		UpcomingInterestKop: rep.UpcomingInterest,
	}
	for _, row := range rep.Rows {
		dto.Rows = append(dto.Rows, creditSummaryRowDTO{
			AccountID:           row.AccountID,
			Path:                row.Path,
			Name:                row.Name,
			RatePPM:             row.RatePPM,
			InterestType:        row.InterestType,
			RepaymentOrder:      row.RepaymentOrder,
			GraceEnabled:        row.GraceEnabled,
			GraceEndDate:        reportDate(row.GraceEndDate),
			InGrace:             row.InGrace,
			DebtKop:             row.Debt,
			AccruedInterestKop:  row.AccruedInterest,
			TotalDebtKop:        row.TotalDebt,
			Upcoming:            upcomingAccruals(row.Upcoming),
			UpcomingInterestKop: row.UpcomingInterest,
		})
	}
	return dto
}

// depositSummary переводит отчёт по вкладам в DTO.
func depositSummary(rep *report.DepositSummaryReport) depositSummaryDTO {
	dto := depositSummaryDTO{
		AsOf:                reportDate(rep.AsOf),
		HorizonDays:         rep.HorizonDays,
		Rows:                make([]depositSummaryRowDTO, 0, len(rep.Rows)),
		BalanceKop:          rep.Balance,
		AccruedInterestKop:  rep.AccruedInterest,
		TotalKop:            rep.Total,
		UpcomingInterestKop: rep.UpcomingInterest,
		UpcomingBonusKop:    rep.UpcomingBonus,
		UpcomingTotalKop:    rep.UpcomingTotal,
	}
	for _, row := range rep.Rows {
		dto.Rows = append(dto.Rows, depositSummaryRowDTO{
			AccountID:           row.AccountID,
			Path:                row.Path,
			Name:                row.Name,
			BaseRatePPM:         row.BaseRatePPM,
			BonusType:           row.BonusType,
			AccrualFrequency:    row.AccrualFrequency,
			Capitalization:      row.Capitalization,
			BalanceKop:          row.Balance,
			AccruedInterestKop:  row.AccruedInterest,
			TotalKop:            row.Total,
			Upcoming:            upcomingAccruals(row.Upcoming),
			UpcomingInterestKop: row.UpcomingInterest,
			UpcomingBonusKop:    row.UpcomingBonus,
			UpcomingTotalKop:    row.UpcomingTotal,
		})
	}
	return dto
}

// creditForecast переводит прогноз по кредиту в DTO.
func creditForecast(forecast *report.CreditForecastReport) creditForecastDTO {
	return creditForecastDTO{
		AccountID:                forecast.AccountID,
		Path:                     forecast.Path,
		Name:                     forecast.Name,
		Date:                     reportDate(forecast.Date),
		HorizonDays:              forecast.HorizonDays,
		RequestedKop:             forecast.Requested,
		PrincipalKop:             forecast.Principal,
		InterestKop:              forecast.Interest,
		TotalKop:                 forecast.Total,
		DebtBeforeKop:            forecast.DebtBefore,
		AccruedInterestBeforeKop: forecast.AccruedInterestBefore,
		TotalDebtBeforeKop:       forecast.TotalDebtBefore,
		DebtAfterKop:             forecast.DebtAfter,
		AccruedInterestAfterKop:  forecast.AccruedInterestAfter,
		TotalDebtAfterKop:        forecast.TotalDebtAfter,
		DebtChangeKop:            forecast.DebtChange,
		InterestChangeKop:        forecast.InterestChange,
		InGrace:                  forecast.InGrace,
		Upcoming:                 upcomingAccruals(forecast.Upcoming),
		UpcomingInterestKop:      forecast.UpcomingInterest,
	}
}

// upcomingAccruals переводит предстоящие начисления в DTO.
func upcomingAccruals(items []report.UpcomingAccrual) []upcomingAccrualDTO {
	out := make([]upcomingAccrualDTO, 0, len(items))
	for _, item := range items {
		out = append(out, upcomingAccrualDTO{
			Date:          reportDate(item.Date),
			Kind:          item.Kind,
			AmountKop:     item.Amount,
			BaseAmountKop: item.Base,
			RatePPM:       item.RatePPM,
			Days:          item.Days,
		})
	}
	return out
}

// productOptions читает общие параметры отчёта по продукту.
func productOptions(r *http.Request) (report.ProductOptions, error) {
	query := r.URL.Query()
	opts := report.ProductOptions{}
	if text := query.Get("account_id"); text != "" {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil || id <= 0 {
			return opts, errInvalidProductParam("account_id")
		}
		opts.AccountID = id
	}
	if text := query.Get("as_of"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return opts, errInvalidProductParam("as_of")
		}
		opts.AsOf = date
	}
	horizon, err := horizonParam(query.Get("horizon_days"))
	if err != nil {
		return opts, err
	}
	opts.HorizonDays = horizon
	return opts, nil
}

// creditForecastOptions читает параметры прогноза погашения кредита.
func creditForecastOptions(r *http.Request) (report.CreditForecastOptions, error) {
	query := r.URL.Query()
	opts := report.CreditForecastOptions{}

	accountText := query.Get("account_id")
	if accountText == "" {
		return opts, errInvalidProductParam("account_id")
	}
	accountID, err := strconv.ParseInt(accountText, 10, 64)
	if err != nil || accountID <= 0 {
		return opts, errInvalidProductParam("account_id")
	}
	opts.AccountID = accountID

	amountText := query.Get("amount")
	if amountText == "" {
		return opts, errInvalidProductParam("amount")
	}
	amount, err := money.ParseKopecks(amountText)
	if err != nil {
		return opts, errInvalidProductParam("amount")
	}
	opts.Amount = amount

	if text := query.Get("date"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return opts, errInvalidProductParam("date")
		}
		opts.Date = date
	}
	horizon, err := horizonParam(query.Get("horizon_days"))
	if err != nil {
		return opts, err
	}
	opts.HorizonDays = horizon
	return opts, nil
}

// horizonParam разбирает горизонт прогноза: 1…366 дней.
func horizonParam(text string) (int, error) {
	if text == "" {
		return 0, nil
	}
	horizon, err := strconv.Atoi(text)
	if err != nil || horizon < 1 || horizon > 366 {
		return 0, errInvalidProductParam("horizon_days")
	}
	return horizon, nil
}

// errInvalidProductParam описывает ошибку параметра отчёта по продукту.
func errInvalidProductParam(name string) error {
	return &productParamError{name: name}
}

// productParamError — ошибка разбора параметра отчёта по продукту.
type productParamError struct {
	name string
}

// Error возвращает причину с подсказкой формата.
func (e *productParamError) Error() string {
	switch e.name {
	case "horizon_days":
		return "неверный параметр horizon_days: ожидается целое число от 1 до 366"
	case "as_of", "date":
		return "неверный параметр " + e.name + ": ожидается ГГГГ-ММ-ДД"
	case "amount":
		return "неверный параметр amount: ожидается сумма в копейках"
	default:
		return "неверный параметр " + e.name + ": ожидается положительное целое число"
	}
}

// reportDate переводит дату в ГГГГ-ММ-ДД; нулевая дата — пустая строка.
func reportDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

// writeProductReportError отдаёт ошибку отчёта по продукту.
func (s *Server) writeProductReportError(w http.ResponseWriter, what string, err error) {
	if writeEngineError(w, err) {
		return
	}
	s.reportError(w, what, err)
}
