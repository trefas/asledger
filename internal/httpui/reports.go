package httpui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/report"
	"github.com/trefas/asledger/pkg/core/store"
)

// reportsView — данные страницы отчётов.
type reportsView struct {
	From            string
	To              string
	Node            string
	IncludeEmpty    bool
	IncludeArchived bool
	Balances        *report.BalancesReport
	Turnovers       *report.TurnoversReport
	Expenses        *report.ActorReport
	// Negative — счета с отрицательным остатком (подсветка минусов).
	Negative []report.BalanceRow
	// Accounts — счета для выбора узла оборотов.
	Accounts []filterAccount
	// Period — подпись периода отчётов.
	Period string

	// CreditSummary и DepositSummary — сводки по продуктам на текущую дату.
	CreditSummary  *report.CreditSummaryReport
	DepositSummary *report.DepositSummaryReport
	// CreditChart и DepositChart — простые SVG-графики к сводкам.
	CreditChart  barChart
	DepositChart barChart
	// CreditAccounts — кредитные счета с настройками для формы прогноза.
	CreditAccounts []creditAccountOption

	// Forecast — прогноз погашения кредита; nil, если он не запрошен.
	Forecast      *report.CreditForecastReport
	ForecastChart barChart
	// ForecastError — ошибка параметров или расчёта прогноза.
	ForecastError string
	// ForecastAccountID, ForecastAmount, ForecastDate и ForecastHorizon —
	// значения формы прогноза.
	ForecastAccountID int64
	ForecastAmount    string
	ForecastDate      string
	ForecastHorizon   string
}

// handleReports отдаёт отчёты: остатки, обороты и расходы по членам семьи.
//
// Остатки по активам и обязательства показаны с иерархическими итогами,
// отрицательные остатки подсвечены. Обороты считаются по выбранному узлу дерева
// за период, расходы — по членам семьи.
func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	view := reportsView{
		From:            query.Get("from"),
		To:              query.Get("to"),
		Node:            query.Get("node"),
		IncludeEmpty:    queryBool(query, "include_empty"),
		IncludeArchived: queryBool(query, "include_archived"),
	}
	from, to, err := reportPeriod(query)
	if err != nil {
		s.mutationError(w, r, "/reports", err.Error())
		return
	}
	opts := report.Options{
		Node:            view.Node,
		From:            from,
		To:              to,
		IncludeEmpty:    view.IncludeEmpty,
		IncludeArchived: view.IncludeArchived,
	}

	balances, err := report.Balances(ctx, s.db, opts)
	if err != nil {
		s.fail(w, err)
		return
	}
	turnovers, err := report.Turnovers(ctx, s.db, opts)
	if err != nil {
		s.fail(w, err)
		return
	}
	expenses, err := report.ExpensesByActor(ctx, s.db, report.Options{From: from, To: to})
	if err != nil {
		s.fail(w, err)
		return
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{})
	if err != nil {
		s.fail(w, err)
		return
	}
	for i := range accounts {
		view.Accounts = append(view.Accounts, filterAccount{Path: accounts[i].FullPath, Name: accounts[i].Name})
	}

	creditSummary, err := report.CreditSummary(ctx, s.db, report.ProductOptions{})
	if err != nil {
		s.fail(w, err)
		return
	}
	depositSummary, err := report.DepositSummary(ctx, s.db, report.ProductOptions{})
	if err != nil {
		s.fail(w, err)
		return
	}
	view.CreditSummary = creditSummary
	view.DepositSummary = depositSummary
	view.CreditChart = creditChart(creditSummary)
	view.DepositChart = depositChart(depositSummary)
	view.CreditAccounts = s.creditAccountOptions(ctx, accounts)
	s.reportsForecast(ctx, query, &view)

	view.Balances = balances
	view.Turnovers = turnovers
	view.Expenses = expenses
	view.Negative = balances.Negatives()
	view.Period = report.FormatPeriod(from, to)
	s.render(w, r, "reports", pageData{Title: "Отчёты", Active: "reports", Data: view})
}

// creditAccountOptions выбирает кредитные счета с настройками для прогноза.
func (s *Server) creditAccountOptions(ctx context.Context, accounts []model.Account) []creditAccountOption {
	var options []creditAccountOption
	for _, account := range accounts {
		if account.Type != model.TypeLiability || !account.IsLeaf || account.IsArchived {
			continue
		}
		if _, err := s.db.CreditSettings(ctx, account.ID); err != nil {
			continue
		}
		options = append(options, creditAccountOption{
			ID: account.ID, FullPath: account.FullPath, HasSettings: true,
		})
	}
	return options
}

// reportsForecast читает параметры прогноза и добавляет результат в страницу.
func (s *Server) reportsForecast(ctx context.Context, query url.Values, view *reportsView) {
	accountText := strings.TrimSpace(query.Get("forecast_account"))
	amountText := strings.TrimSpace(query.Get("forecast_amount"))
	view.ForecastAccountID = parseID(accountText, 0)
	view.ForecastAmount = amountText
	view.ForecastDate = strings.TrimSpace(query.Get("forecast_date"))
	view.ForecastHorizon = strings.TrimSpace(query.Get("forecast_horizon"))

	if accountText == "" && amountText == "" {
		return
	}
	if accountText == "" || amountText == "" {
		view.ForecastError = "Укажите счёт и сумму прогноза."
		return
	}
	amount, err := money.Parse(amountText)
	if err != nil {
		view.ForecastError = "Сумма прогноза должна быть числом, например 20 000,00."
		return
	}
	date, err := parseOptionalDate(view.ForecastDate)
	if err != nil {
		view.ForecastError = err.Error()
		return
	}
	horizon := 0
	if view.ForecastHorizon != "" {
		value, err := strconv.Atoi(view.ForecastHorizon)
		if err != nil || value < 1 || value > 366 {
			view.ForecastError = "Горизонт прогноза — целое число от 1 до 366 дней."
			return
		}
		horizon = value
	}

	forecast, err := report.CreditForecast(ctx, s.db, report.CreditForecastOptions{
		AccountID:   view.ForecastAccountID,
		Amount:      amount,
		Date:        date,
		HorizonDays: horizon,
	})
	if err != nil {
		view.ForecastError = statusMessage(err)
		return
	}
	view.Forecast = forecast
	view.ForecastChart = newBarChart(
		"Долг до и после платежа",
		[]string{"до платежа", "после платежа", "проценты после"},
		[]money.Kop{forecast.TotalDebtBefore, forecast.TotalDebtAfter, forecast.UpcomingInterest},
		true,
	)
}

// creditChart строит график долга с процентами по кредитам.
func creditChart(rep *report.CreditSummaryReport) barChart {
	labels := make([]string, 0, len(rep.Rows))
	values := make([]money.Kop, 0, len(rep.Rows))
	for _, row := range rep.Rows {
		labels = append(labels, row.Name)
		values = append(values, row.TotalDebt)
	}
	return newBarChart("Долг с процентами по кредитам", labels, values, false)
}

// depositChart строит график остатка с процентами по вкладам.
func depositChart(rep *report.DepositSummaryReport) barChart {
	labels := make([]string, 0, len(rep.Rows))
	values := make([]money.Kop, 0, len(rep.Rows))
	for _, row := range rep.Rows {
		labels = append(labels, row.Name)
		values = append(values, row.Total)
	}
	return newBarChart("Остаток с процентами по вкладам", labels, values, false)
}

// reportPeriod разбирает период отчёта (ГГГГ-ММ-ДД, границы включают день).
func reportPeriod(query url.Values) (int64, int64, error) {
	var from, to int64
	if text := query.Get("from"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return 0, 0, fmt.Errorf("неверная дата начала периода")
		}
		from = date.UTC().Unix()
	}
	if text := query.Get("to"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return 0, 0, fmt.Errorf("неверная дата конца периода")
		}
		to = date.UTC().AddDate(0, 0, 1).Unix() - 1
	}
	if from != 0 && to != 0 && from > to {
		return 0, 0, fmt.Errorf("начало периода позже конца")
	}
	return from, to, nil
}

// queryBool читает флаг из строки запроса.
func queryBool(query url.Values, name string) bool {
	switch query.Get(name) {
	case "1", "true", "да", "yes", "on":
		return true
	default:
		return false
	}
}
