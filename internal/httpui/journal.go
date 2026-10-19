package httpui

import (
	"fmt"
	"net/http"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// journalView — данные страницы журнала.
type journalView struct {
	Transactions []transactionView
	Empty        bool
}

// transactionView — проводка в журнале.
type transactionView struct {
	Date        time.Time
	OpType      model.OpType
	Description string
	Total       money.Kop
	Lines       []lineView
}

// lineView — строка проводки в журнале.
type lineView struct {
	Account string
	Side    model.Side
	Amount  money.Kop
	Class   string
}

// handleJournal отдаёт журнал последних проводок.
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	paths, err := s.accountPaths(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	transactions, err := s.db.ListTransactions(ctx, store.TransactionFilter{Limit: 200})
	if err != nil {
		s.fail(w, err)
		return
	}

	view := journalView{Empty: len(transactions) == 0}
	for i := range transactions {
		view.Transactions = append(view.Transactions, toTransactionView(&transactions[i], paths))
	}
	s.render(w, r, "journal", pageData{Title: "Журнал", Active: "journal", Data: view})
}

// toTransactionView переводит проводку в вид страницы журнала.
func toTransactionView(tr *model.Transaction, paths map[int64]string) transactionView {
	view := transactionView{
		Date:        tr.Date,
		OpType:      tr.OpType,
		Description: tr.Description,
	}
	for _, line := range tr.Lines {
		class := "credit"
		if line.Side == model.SideDebit {
			class = "debit"
			view.Total = view.Total.Add(line.Amount)
		}
		view.Lines = append(view.Lines, lineView{
			Account: accountName(paths, line.AccountID),
			Side:    line.Side,
			Amount:  line.Amount,
			Class:   class,
		})
	}
	return view
}

// accountName возвращает полный путь счёта или служебную подпись.
func accountName(paths map[int64]string, id int64) string {
	if path, ok := paths[id]; ok {
		return path
	}
	return fmt.Sprintf("счёт #%d", id)
}
