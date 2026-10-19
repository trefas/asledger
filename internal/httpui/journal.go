package httpui

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/report"
	"github.com/trefas/asledger/pkg/core/store"
)

// journalView — данные страницы журнала.
type journalView struct {
	Transactions []transactionView
	Empty        bool
	// Total, Page, Pages — постраничная выдача.
	Total   int
	Page    int
	Pages   int
	PerPage int
	// PrevLink и NextLink — ссылки на соседние страницы (пусто, если нет).
	PrevLink string
	NextLink string
	// Значения фильтров.
	From        string
	To          string
	AccountPath string
	OpType      string
	ActorID     int64
	AmountFrom  string
	AmountTo    string
	HasFilter   bool
	// Справочники для формы фильтров.
	Actors   []model.User
	OpTypes  []model.OpType
	Accounts []filterAccount
	// Negative — счета с отрицательным остатком (подсветка минусов).
	Negative []string
}

// filterAccount — счёт в списке фильтра.
type filterAccount struct {
	Path string
	Name string
}

// transactionView — проводка в журнале.
type transactionView struct {
	UUID        string
	Date        time.Time
	OpType      model.OpType
	Description string
	Total       money.Kop
	Lines       []lineView
	// Duplicate — возможный дубль уже введённой проводки.
	Duplicate bool
	// Reversed — проводка уже отменена сторно.
	Reversed bool
	// Reversal — сама является сторно.
	Reversal bool
}

// lineView — строка проводки в журнале.
type lineView struct {
	Account string
	Side    model.Side
	Amount  money.Kop
	Class   string
}

// handleJournal отдаёт журнал проводок с фильтрами, подсветкой дублей и сторно.
func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	paths, err := s.accountPaths(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	filter, view, err := s.journalFilter(r)
	if err != nil {
		s.mutationError(w, r, "/", err.Error())
		return
	}

	total, err := s.db.CountTransactions(ctx, filter)
	if err != nil {
		s.fail(w, err)
		return
	}
	view.Total = total
	view.Pages = (total + view.PerPage - 1) / view.PerPage
	if view.Pages == 0 {
		view.Pages = 1
	}
	if view.Page > view.Pages {
		view.Page = view.Pages
	}
	filter.Offset = (view.Page - 1) * view.PerPage
	filter.Limit = view.PerPage

	transactions, err := s.db.ListTransactions(ctx, filter)
	if err != nil {
		s.fail(w, err)
		return
	}

	reversed := make(map[int64]bool, len(transactions))
	for i := range transactions {
		if transactions[i].ReversesID != nil {
			reversed[*transactions[i].ReversesID] = true
		}
	}
	duplicates := duplicateIDs(transactions)

	view.Empty = total == 0
	for i := range transactions {
		tr := &transactions[i]
		row := toTransactionView(tr, paths)
		row.UUID = tr.UUID
		row.Duplicate = duplicates[tr.ID]
		row.Reversal = tr.ReversesID != nil
		row.Reversed = reversed[tr.ID]
		view.Transactions = append(view.Transactions, row)
	}
	view.PrevLink = journalLink(view, view.Page-1)
	view.NextLink = journalLink(view, view.Page+1)

	// Подсветка минусов: счета с отрицательным остатком видны сразу в журнале.
	balances, err := report.Balances(ctx, s.db, report.Options{})
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, row := range balances.Negatives() {
		view.Negative = append(view.Negative, row.Path)
	}

	s.render(w, r, "journal", pageData{Title: "Журнал", Active: "journal", Data: view})
}

// handleJournalReverse создаёт сторно проводки из журнала.
func (s *Server) handleJournalReverse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	uuid := strings.TrimSpace(r.FormValue("uuid"))
	if uuid == "" {
		s.mutationError(w, r, "/", "не удалось определить проводку")
		return
	}
	original, err := s.db.TransactionByUUID(ctx, uuid)
	if err != nil {
		s.mutationError(w, r, "/", "проводка не найдена")
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(users) == 0 {
		s.mutationError(w, r, "/", "в книге нет ни одного пользователя")
		return
	}
	transaction, err := s.engine.Reverse(ctx, original.ID, users[0].ID, newUUID(),
		time.Now().UTC(), "Сторно: "+original.Description)
	if err != nil {
		s.mutationError(w, r, "/", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/", "Проводка отменена сторно "+transaction.UUID)
}

// journalFilter разбирает фильтры журнала и готовит значения для формы.
func (s *Server) journalFilter(r *http.Request) (store.TransactionFilter, journalView, error) {
	query := r.URL.Query()
	view := journalView{
		Page:        1,
		PerPage:     50,
		From:        query.Get("from"),
		To:          query.Get("to"),
		AccountPath: query.Get("account"),
		OpType:      query.Get("op_type"),
		AmountFrom:  query.Get("amount_from"),
		AmountTo:    query.Get("amount_to"),
	}
	if page, err := strconv.Atoi(query.Get("page")); err == nil && page > 0 {
		view.Page = page
	}

	filter := store.TransactionFilter{AccountPrefix: view.AccountPath}
	if view.From != "" {
		date, err := time.Parse(time.DateOnly, view.From)
		if err != nil {
			return filter, view, fmt.Errorf("неверная дата начала периода")
		}
		filter.From = date.UTC().Unix()
	}
	if view.To != "" {
		date, err := time.Parse(time.DateOnly, view.To)
		if err != nil {
			return filter, view, fmt.Errorf("неверная дата конца периода")
		}
		filter.To = date.UTC().AddDate(0, 0, 1).Unix() - 1
	}
	if view.OpType != "" {
		op := model.OpType(view.OpType)
		if !op.Valid() {
			return filter, view, fmt.Errorf("неизвестный тип операции")
		}
		filter.OpType = &op
	}
	if text := query.Get("actor_id"); text != "" {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil || id <= 0 {
			return filter, view, fmt.Errorf("неизвестный автор проводки")
		}
		filter.ActorID = &id
		view.ActorID = id
	}
	if text := strings.TrimSpace(view.AmountFrom); text != "" {
		amount, err := money.Parse(text)
		if err != nil {
			return filter, view, fmt.Errorf("неверная нижняя граница суммы")
		}
		filter.AmountAtLeast = &amount
	}
	if text := strings.TrimSpace(view.AmountTo); text != "" {
		amount, err := money.Parse(text)
		if err != nil {
			return filter, view, fmt.Errorf("неверная верхняя граница суммы")
		}
		filter.AmountAtMost = &amount
	}
	view.HasFilter = view.From != "" || view.To != "" || view.AccountPath != "" ||
		view.OpType != "" || view.ActorID != 0 || view.AmountFrom != "" || view.AmountTo != ""

	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		return filter, view, err
	}
	view.Actors = users
	view.OpTypes = append(view.OpTypes, model.OpTypes()...)
	accounts, err := s.db.ListAccounts(r.Context(), store.AccountFilter{})
	if err != nil {
		return filter, view, err
	}
	for i := range accounts {
		if accounts[i].IsLeaf {
			view.Accounts = append(view.Accounts, filterAccount{Path: accounts[i].FullPath, Name: accounts[i].Name})
		}
	}
	return filter, view, nil
}

// journalLink собирает ссылку на страницу журнала с текущими фильтрами.
func journalLink(view journalView, page int) string {
	if page < 1 || page > view.Pages {
		return ""
	}
	values := url.Values{}
	if view.From != "" {
		values.Set("from", view.From)
	}
	if view.To != "" {
		values.Set("to", view.To)
	}
	if view.AccountPath != "" {
		values.Set("account", view.AccountPath)
	}
	if view.OpType != "" {
		values.Set("op_type", view.OpType)
	}
	if view.ActorID != 0 {
		values.Set("actor_id", strconv.FormatInt(view.ActorID, 10))
	}
	if view.AmountFrom != "" {
		values.Set("amount_from", view.AmountFrom)
	}
	if view.AmountTo != "" {
		values.Set("amount_to", view.AmountTo)
	}
	values.Set("page", strconv.Itoa(page))
	return "/?" + values.Encode()
}

// duplicateIDs помечает проводки, у которых есть совпадение по типу, дате,
// счетам и суммам в пределах дня (эвристика раздела 9.3).
func duplicateIDs(transactions []model.Transaction) map[int64]bool {
	type key struct {
		opType  model.OpType
		day     int64
		content string
	}
	groups := make(map[key][]int64)
	for i := range transactions {
		tr := &transactions[i]
		k := key{opType: tr.OpType, day: tr.Date.UTC().Truncate(24 * time.Hour).Unix(), content: lineSignature(tr)}
		groups[k] = append(groups[k], tr.ID)
	}
	duplicates := make(map[int64]bool)
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		for _, id := range ids {
			duplicates[id] = true
		}
	}
	return duplicates
}

// lineSignature описывает состав проводки: счёт, сторона и сумма строк.
func lineSignature(tr *model.Transaction) string {
	parts := make([]string, 0, len(tr.Lines))
	for _, line := range tr.Lines {
		parts = append(parts, fmt.Sprintf("%d:%s:%d", line.AccountID, line.Side, line.Amount.Kopecks()))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
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
