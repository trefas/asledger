package httpui

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

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

	view.Balances = balances
	view.Turnovers = turnovers
	view.Expenses = expenses
	view.Negative = balances.Negatives()
	view.Period = report.FormatPeriod(from, to)
	s.render(w, r, "reports", pageData{Title: "Отчёты", Active: "reports", Data: view})
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
