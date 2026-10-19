package httpui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// accountNode — узел дерева плана счетов для отображения.
type accountNode struct {
	Account  model.Account
	Children []*accountNode
}

// chartView — данные страницы редактора плана счетов.
type chartView struct {
	Tree        []*accountNode
	Accounts    []model.Account
	Types       []model.AccountType
	BalanceMode string
	ModeFree    string
	ModeLimited string
	// Total — всего счетов в дереве.
	Total int
	// SystemCount — число служебных счетов (защищены от правки).
	SystemCount int
}

// accountTypes — типы счетов, доступные при создании.
var accountTypes = []model.AccountType{
	model.TypeAsset, model.TypeLiability, model.TypeCapital,
	model.TypeIncome, model.TypeExpense,
}

// handleChart показывает дерево плана счетов.
func (s *Server) handleChart(w http.ResponseWriter, r *http.Request) {
	view, err := s.chartView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "chart", pageData{Title: "План счетов", Active: "chart", Data: view})
}

// chartView собирает дерево и настройки режима контроля остатка.
func (s *Server) chartView(ctx context.Context) (chartView, error) {
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return chartView{}, err
	}
	mode, ok, err := s.db.Setting(ctx, store.SettingBalanceMode)
	if err != nil {
		return chartView{}, err
	}
	if !ok || mode == "" {
		mode = store.BalanceModeFree
	}

	view := chartView{
		Accounts:    accounts,
		Types:       accountTypes,
		BalanceMode: mode,
		ModeFree:    store.BalanceModeFree,
		ModeLimited: store.BalanceModeLimited,
	}
	for _, acc := range accounts {
		if acc.IsSystem {
			view.SystemCount++
		}
	}
	view.Tree = buildAccountTree(accounts)
	view.Total = treeCount(view.Tree)
	return view, nil
}

// buildAccountTree собирает дерево из плоского списка (порядок по полному пути).
func buildAccountTree(accounts []model.Account) []*accountNode {
	nodes := make(map[int64]*accountNode, len(accounts))
	order := make([]*accountNode, 0, len(accounts))
	for i := range accounts {
		node := &accountNode{Account: accounts[i]}
		nodes[accounts[i].ID] = node
		order = append(order, node)
	}

	var roots []*accountNode
	for _, node := range order {
		parentID := node.Account.ParentID
		if parentID == nil {
			roots = append(roots, node)
			continue
		}
		parent, ok := nodes[*parentID]
		if !ok {
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	return roots
}

// handleChartCreate создаёт счёт.
func (s *Server) handleChartCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	typ := model.AccountType(r.FormValue("type"))
	var parentID *int64
	if raw := strings.TrimSpace(r.FormValue("parent_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			s.mutationError(w, r, "/chart", "не удалось определить родительский счёт")
			return
		}
		parentID = &id
	}

	if _, err := s.db.CreateAccount(ctx, store.AccountInput{Name: name, Type: typ, ParentID: parentID}); err != nil {
		s.mutationError(w, r, "/chart", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/chart", "Счёт «"+name+"» создан")
}

// handleChartRename переименовывает счёт (путь потомков пересчитывается).
func (s *Server) handleChartRename(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	id, ok := s.formID(w, r, "account_id", "/chart")
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if err := s.db.RenameAccount(ctx, id, name); err != nil {
		s.mutationError(w, r, "/chart", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/chart", "Счёт переименован: «"+name+"»")
}

// handleChartMove переносит счёт к другому родителю по пути.
func (s *Server) handleChartMove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	id, ok := s.formID(w, r, "account_id", "/chart")
	if !ok {
		return
	}

	var parentID *int64
	if path := strings.TrimSpace(r.FormValue("parent_path")); path != "" {
		parent, err := s.db.AccountByPath(ctx, path)
		if err != nil {
			s.mutationError(w, r, "/chart", "родительский счёт не найден: «"+path+"»")
			return
		}
		parentID = &parent.ID
	}
	if err := s.db.MoveAccount(ctx, id, parentID); err != nil {
		s.mutationError(w, r, "/chart", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/chart", "Счёт перемещён")
}

// handleChartArchive архивирует счёт без удаления истории.
func (s *Server) handleChartArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	id, ok := s.formID(w, r, "account_id", "/chart")
	if !ok {
		return
	}
	if err := s.db.ArchiveAccount(r.Context(), id); err != nil {
		s.mutationError(w, r, "/chart", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/chart", "Счёт заархивирован: история сохранена")
}

// handleChartMinBalance задаёт минимально допустимый остаток.
func (s *Server) handleChartMinBalance(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	id, ok := s.formID(w, r, "account_id", "/chart")
	if !ok {
		return
	}
	value, err := parseSignedAmount(r.FormValue("min_balance"))
	if err != nil {
		s.mutationError(w, r, "/chart", err.Error())
		return
	}
	if err := s.db.SetMinBalance(r.Context(), id, value); err != nil {
		s.mutationError(w, r, "/chart", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/chart", "Остаток сохранён: "+value.String())
}

// handleChartMode переключает режим контроля остатка.
func (s *Server) handleChartMode(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	mode := r.FormValue("mode")
	if mode != store.BalanceModeFree && mode != store.BalanceModeLimited {
		s.mutationError(w, r, "/chart", "неизвестный режим контроля остатка")
		return
	}
	if err := s.db.SetSetting(r.Context(), store.SettingBalanceMode, mode); err != nil {
		s.mutationError(w, r, "/chart", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/chart", "Режим контроля остатка: "+mode)
}

// formID разбирает идентификатор счёта из формы.
func (s *Server) formID(w http.ResponseWriter, r *http.Request, field, back string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue(field)), 10, 64)
	if err != nil || id <= 0 {
		s.mutationError(w, r, back, "не удалось определить счёт")
		return 0, false
	}
	return id, true
}

// mutationError переадресует на страницу с сообщением об ошибке.
func (s *Server) mutationError(w http.ResponseWriter, r *http.Request, path, message string) {
	http.Redirect(w, r, path+"?flash="+encodeQuery(message)+"&kind=error", http.StatusSeeOther)
}

// minBalanceLabel выводит min_balance понятным текстом.
func minBalanceLabel(value money.Kop) string {
	if value == 0 {
		return "без ограничения"
	}
	return value.String()
}

// indent отбивает иерархию в таблицах отчётов.
func indent(depth int) string {
	if depth <= 0 {
		return ""
	}
	out := make([]byte, 0, depth*2)
	for i := 0; i < depth; i++ {
		out = append(out, '\u00a0', '\u00a0')
	}
	return string(out)
}

// treeCount считает счета дерева.
func treeCount(nodes []*accountNode) int {
	total := 0
	for _, node := range nodes {
		total += 1 + treeCount(node.Children)
	}
	return total
}
