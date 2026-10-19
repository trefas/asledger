package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/report"
	"github.com/trefas/asledger/pkg/core/store"
)

// balanceRowDTO — строка отчёта об остатках.
type balanceRowDTO struct {
	AccountID int64             `json:"account_id"`
	Path      string            `json:"path"`
	Name      string            `json:"name"`
	Type      model.AccountType `json:"type"`
	Depth     int               `json:"depth"`
	OwnKop    money.Kop         `json:"own_kop"`
	TotalKop  money.Kop         `json:"total_kop"`
	Own       string            `json:"own"`
	Total     string            `json:"total"`
	Leaf      bool              `json:"is_leaf"`
	Archived  bool              `json:"is_archived"`
	Negative  bool              `json:"negative"`
}

// balancesReportDTO — отчёт об остатках.
type balancesReportDTO struct {
	Rows           []balanceRowDTO `json:"rows"`
	AssetsKop      money.Kop       `json:"assets_kop"`
	LiabilitiesKop money.Kop       `json:"liabilities_kop"`
	NetWorthKop    money.Kop       `json:"net_worth_kop"`
	Negative       []string        `json:"negative,omitempty"`
}

// handleReportsBalances отдаёт отчёт об остатках с иерархическими итогами.
func (s *Server) handleReportsBalances(w http.ResponseWriter, r *http.Request) {
	rep, err := report.Balances(r.Context(), s.db, report.Options{
		IncludeArchived: queryBool(r, "include_archived"),
		IncludeEmpty:    queryBool(r, "include_empty"),
	})
	if err != nil {
		s.reportError(w, "остатки", err)
		return
	}
	writeJSON(w, http.StatusOK, balancesReport(rep))
}

// balancesReport переводит отчёт об остатках в DTO.
func balancesReport(rep *report.BalancesReport) balancesReportDTO {
	dto := balancesReportDTO{
		Rows:           make([]balanceRowDTO, 0, len(rep.Rows)),
		AssetsKop:      rep.Assets,
		LiabilitiesKop: rep.Liabilities,
		NetWorthKop:    rep.NetWorth(),
	}
	for _, row := range rep.Rows {
		dto.Rows = append(dto.Rows, balanceRowDTO{
			AccountID: row.AccountID,
			Path:      row.Path,
			Name:      row.Name,
			Type:      row.Type,
			Depth:     row.Depth,
			OwnKop:    row.Own,
			TotalKop:  row.Total,
			Own:       row.Own.String(),
			Total:     row.Total.String(),
			Leaf:      row.Leaf,
			Archived:  row.Archived,
			Negative:  row.Negative(),
		})
		if row.Negative() {
			dto.Negative = append(dto.Negative, row.Path)
		}
	}
	return dto
}

// turnoverRowDTO — строка отчёта об оборотах.
type turnoverRowDTO struct {
	AccountID  int64             `json:"account_id"`
	Path       string            `json:"path"`
	Name       string            `json:"name"`
	Type       model.AccountType `json:"type"`
	Depth      int               `json:"depth"`
	OpeningKop money.Kop         `json:"opening_kop"`
	DebitKop   money.Kop         `json:"debit_kop"`
	CreditKop  money.Kop         `json:"credit_kop"`
	ClosingKop money.Kop         `json:"closing_kop"`
	Opening    string            `json:"opening"`
	Debit      string            `json:"debit"`
	Credit     string            `json:"credit"`
	Closing    string            `json:"closing"`
}

// turnoversReportDTO — отчёт об оборотах за период.
type turnoversReportDTO struct {
	Node string           `json:"node,omitempty"`
	From string           `json:"from,omitempty"`
	To   string           `json:"to,omitempty"`
	Rows []turnoverRowDTO `json:"rows"`
}

// handleReportsTurnovers отдаёт обороты по узлу дерева за период.
func (s *Server) handleReportsTurnovers(w http.ResponseWriter, r *http.Request) {
	from, to, err := period(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	rep, err := report.Turnovers(r.Context(), s.db, report.Options{
		Node:            r.URL.Query().Get("node"),
		From:            from,
		To:              to,
		IncludeArchived: queryBool(r, "include_archived"),
		IncludeEmpty:    queryBool(r, "include_empty"),
	})
	if err != nil {
		s.reportError(w, "обороты", err)
		return
	}
	dto := turnoversReportDTO{
		Node: rep.Node,
		From: dateText(rep.From),
		To:   dateText(rep.To),
		Rows: make([]turnoverRowDTO, 0, len(rep.Rows)),
	}
	for _, row := range rep.Rows {
		dto.Rows = append(dto.Rows, turnoverRowDTO{
			AccountID:  row.AccountID,
			Path:       row.Path,
			Name:       row.Name,
			Type:       row.Type,
			Depth:      row.Depth,
			OpeningKop: row.Opening,
			DebitKop:   row.Debit,
			CreditKop:  row.Credit,
			ClosingKop: row.Closing,
			Opening:    row.Opening.String(),
			Debit:      row.Debit.String(),
			Credit:     row.Credit.String(),
			Closing:    row.Closing.String(),
		})
	}
	writeJSON(w, http.StatusOK, dto)
}

// articleExpenseDTO — расход по статье.
type articleExpenseDTO struct {
	AccountID int64     `json:"account_id"`
	Path      string    `json:"path"`
	AmountKop money.Kop `json:"amount_kop"`
	Amount    string    `json:"amount"`
}

// actorExpenseDTO — расходы одного члена семьи.
type actorExpenseDTO struct {
	ActorID  int64               `json:"actor_id"`
	Name     string              `json:"name"`
	TotalKop money.Kop           `json:"total_kop"`
	Total    string              `json:"total"`
	Articles []articleExpenseDTO `json:"articles"`
}

// expensesReportDTO — расходы по членам семьи за период.
type expensesReportDTO struct {
	From     string            `json:"from,omitempty"`
	To       string            `json:"to,omitempty"`
	Actors   []actorExpenseDTO `json:"actors"`
	TotalKop money.Kop         `json:"total_kop"`
	Total    string            `json:"total"`
}

// handleReportsExpenses отдаёт расходы по членам семьи за период.
func (s *Server) handleReportsExpenses(w http.ResponseWriter, r *http.Request) {
	from, to, err := period(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	rep, err := report.ExpensesByActor(r.Context(), s.db, report.Options{From: from, To: to})
	if err != nil {
		s.reportError(w, "расходы по членам семьи", err)
		return
	}
	dto := expensesReportDTO{
		From:     dateText(rep.From),
		To:       dateText(rep.To),
		TotalKop: rep.Total,
		Total:    rep.Total.String(),
		Actors:   make([]actorExpenseDTO, 0, len(rep.Actors)),
	}
	for _, actor := range rep.Actors {
		item := actorExpenseDTO{
			ActorID:  actor.ActorID,
			Name:     actor.Name,
			TotalKop: actor.Total,
			Total:    actor.Total.String(),
			Articles: make([]articleExpenseDTO, 0, len(actor.Articles)),
		}
		for _, article := range actor.Articles {
			item.Articles = append(item.Articles, articleExpenseDTO{
				AccountID: article.AccountID,
				Path:      article.Path,
				AmountKop: article.Amount,
				Amount:    article.Amount.String(),
			})
		}
		dto.Actors = append(dto.Actors, item)
	}
	writeJSON(w, http.StatusOK, dto)
}

// journalLineDTO — строка проводки журнала.
type journalLineDTO struct {
	AccountID int64      `json:"account_id"`
	Path      string     `json:"path"`
	Side      model.Side `json:"side"`
	AmountKop money.Kop  `json:"amount_kop"`
	Amount    string     `json:"amount"`
}

// journalRowDTO — проводка журнала.
type journalRowDTO struct {
	ID          int64            `json:"id"`
	UUID        string           `json:"uuid"`
	Date        string           `json:"date"`
	OpType      model.OpType     `json:"op_type"`
	Description string           `json:"description,omitempty"`
	ActorID     int64            `json:"actor_id"`
	ActorName   string           `json:"actor_name"`
	ReversesID  *int64           `json:"reverses_id,omitempty"`
	AmountKop   money.Kop        `json:"amount_kop"`
	Amount      string           `json:"amount"`
	Lines       []journalLineDTO `json:"lines"`
}

// journalReportDTO — журнал проводок с постраничной выдачей.
type journalReportDTO struct {
	Total   int             `json:"total"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
	Rows    []journalRowDTO `json:"rows"`
}

// handleReportsJournal отдаёт журнал проводок с фильтрами и постраничной выдачей.
func (s *Server) handleReportsJournal(w http.ResponseWriter, r *http.Request) {
	filter, err := journalFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}
	page, perPage := pageParams(r)
	total, err := s.db.CountTransactions(r.Context(), filter)
	if err != nil {
		s.reportError(w, "журнал", err)
		return
	}
	filter.Offset = (page - 1) * perPage
	filter.Limit = perPage
	rows, err := report.Journal(r.Context(), s.db, filter)
	if err != nil {
		s.reportError(w, "журнал", err)
		return
	}
	dto := journalReportDTO{Total: total, Page: page, PerPage: perPage, Rows: make([]journalRowDTO, 0, len(rows))}
	for _, row := range rows {
		item := journalRowDTO{
			ID:          row.ID,
			UUID:        row.UUID,
			Date:        row.Date.UTC().Format(time.DateOnly),
			OpType:      row.OpType,
			Description: row.Description,
			ActorID:     row.ActorID,
			ActorName:   row.ActorName,
			ReversesID:  row.ReversesID,
			AmountKop:   row.Amount,
			Amount:      row.Amount.String(),
			Lines:       make([]journalLineDTO, 0, len(row.Lines)),
		}
		for _, line := range row.Lines {
			item.Lines = append(item.Lines, journalLineDTO{
				AccountID: line.AccountID,
				Path:      line.Path,
				Side:      line.Side,
				AmountKop: line.Amount,
				Amount:    line.Amount.String(),
			})
		}
		dto.Rows = append(dto.Rows, item)
	}
	writeJSON(w, http.StatusOK, dto)
}

// handleReportsDiscrepancies отдаёт расхождения по счетам.
func (s *Server) handleReportsDiscrepancies(w http.ResponseWriter, r *http.Request) {
	discrepancies, err := s.reconcile.Discrepancies(r.Context())
	if err != nil {
		s.reportError(w, "расхождения по счетам", err)
		return
	}
	rows := make([]discrepancyDTO, 0, len(discrepancies))
	for _, d := range discrepancies {
		rows = append(rows, discrepancyDTO{
			AccountID:  d.AccountID,
			Path:       d.Path,
			Type:       d.Type,
			FactKop:    d.Fact,
			BalanceKop: d.Balance,
			DiffKop:    d.Difference,
			Fact:       d.Fact.String(),
			Balance:    d.Balance.String(),
			Difference: d.Difference.String(),
			CheckedAt:  d.CheckedAt.UTC().Format(time.RFC3339),
			Adjustable: d.Adjustable,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// discrepancyDTO — расхождение по счёту.
type discrepancyDTO struct {
	AccountID  int64             `json:"account_id"`
	Path       string            `json:"path"`
	Type       model.AccountType `json:"type"`
	FactKop    money.Kop         `json:"fact_kop"`
	BalanceKop money.Kop         `json:"balance_kop"`
	DiffKop    money.Kop         `json:"difference_kop"`
	Fact       string            `json:"fact"`
	Balance    string            `json:"balance"`
	Difference string            `json:"difference"`
	CheckedAt  string            `json:"checked_at"`
	Adjustable bool              `json:"adjustable"`
}

// journalFilter собирает фильтр журнала из строки запроса.
func journalFilter(r *http.Request) (store.TransactionFilter, error) {
	query := r.URL.Query()
	filter := store.TransactionFilter{
		AccountPrefix: query.Get("account"),
		Source:        query.Get("source"),
	}
	from, to, err := period(query)
	if err != nil {
		return filter, err
	}
	filter.From, filter.To = from, to

	if text := query.Get("account_id"); text != "" {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return filter, errInvalidPeriod("account_id")
		}
		filter.AccountID = &id
	}
	if text := query.Get("actor_id"); text != "" {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return filter, errInvalidPeriod("actor_id")
		}
		filter.ActorID = &id
	}
	if text := query.Get("op_type"); text != "" {
		op := model.OpType(text)
		if !op.Valid() {
			return filter, errInvalidPeriod("op_type")
		}
		filter.OpType = &op
	}
	if text := query.Get("amount_from"); text != "" {
		amount, err := money.ParseKopecks(text)
		if err != nil {
			return filter, errInvalidPeriod("amount_from")
		}
		filter.AmountAtLeast = &amount
	}
	if text := query.Get("amount_to"); text != "" {
		amount, err := money.ParseKopecks(text)
		if err != nil {
			return filter, errInvalidPeriod("amount_to")
		}
		filter.AmountAtMost = &amount
	}
	return filter, nil
}

// pageParams читает номер страницы и размер страницы.
func pageParams(r *http.Request) (int, int) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	perPage, err := strconv.Atoi(r.URL.Query().Get("per_page"))
	if err != nil || perPage < 1 {
		perPage = 50
	}
	if perPage > 500 {
		perPage = 500
	}
	return page, perPage
}

// queryBool читает флаг из строки запроса («1», «true», «да»).
func queryBool(r *http.Request, name string) bool {
	switch r.URL.Query().Get(name) {
	case "1", "true", "да", "yes":
		return true
	default:
		return false
	}
}

// dateText переводит unix-секунды в дату ГГГГ-ММ-ДД (пусто для нуля).
func dateText(unix int64) string {
	if unix == 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format(time.DateOnly)
}

// reportError отвечает внутренней ошибкой построения отчёта.
func (s *Server) reportError(w http.ResponseWriter, what string, err error) {
	s.logger.Error("не удалось построить отчёт", "report", what, "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}
