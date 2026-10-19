package httpui

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/money"
	reconcilesvc "github.com/trefas/asledger/pkg/core/reconcile"
	"github.com/trefas/asledger/pkg/core/store"
)

// reconcileView — данные экрана сверки счёта.
type reconcileView struct {
	Accounts      []reconcileAccount
	Discrepancies []reconcileRow
	Empty         bool
	// AccountID и Amount — предзаполнение формы.
	AccountID int64
	Amount    string
}

// reconcileAccount — счёт, доступный для сверки.
type reconcileAccount struct {
	ID   int64
	Path string
}

// reconcileRow — расхождение по счёту в списке.
type reconcileRow struct {
	AccountID  int64
	Path       string
	Fact       money.Kop
	Balance    money.Kop
	Difference money.Kop
	CheckedAt  time.Time
	Adjustable bool
}

// handleReconcileForm показывает экран сверки: расхождения и форму.
func (s *Server) handleReconcileForm(w http.ResponseWriter, r *http.Request) {
	view, err := s.reconcileView(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	if text := r.URL.Query().Get("account"); text != "" {
		if id, err := strconv.ParseInt(text, 10, 64); err == nil {
			view.AccountID = id
		}
	}
	s.render(w, r, "reconcile", pageData{Title: "Сверка", Active: "reconcile", Data: view})
}

// handleReconcileSubmit выполняет сверку и, если запрошено, создаёт корректировку.
func (s *Server) handleReconcileSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	accountID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("account_id")), 10, 64)
	if err != nil || accountID <= 0 {
		s.mutationError(w, r, "/reconcile", "выберите счёт для сверки")
		return
	}
	fact, err := money.Parse(r.FormValue("amount"))
	if err != nil {
		s.mutationError(w, r, "/reconcile", "не удалось разобрать сумму: "+err.Error())
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(users) == 0 {
		s.mutationError(w, r, "/reconcile", "в книге нет ни одного пользователя")
		return
	}
	generate := r.FormValue("generate") != ""

	result, err := s.recon.Check(ctx, reconcilesvc.Request{
		AccountID: accountID,
		Fact:      fact,
		Date:      time.Now().UTC(),
		ActorID:   users[0].ID,
		Generate:  generate,
		UUID:      newUUID(),
		Source:    "web",
	})
	if errors.Is(err, reconcilesvc.ErrNotAdjustable) {
		s.mutationError(w, r, "/reconcile", "счёт этого типа нельзя скорректировать сверкой")
		return
	}
	if err != nil {
		s.mutationError(w, r, "/reconcile", statusMessage(err))
		return
	}

	account, err := s.db.Account(ctx, accountID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if result.Transaction != nil {
		s.redirectOK(w, r, "/reconcile",
			"Корректировка создана: расхождение "+result.Difference.String()+" по счёту "+account.FullPath)
		return
	}
	message := "Расхождение по счёту " + account.FullPath + ": " + result.Difference.String()
	if result.Difference == 0 {
		message = "Счёт " + account.FullPath + ": расхождения нет"
	}
	s.redirectOK(w, r, "/reconcile", message)
}

// reconcileView собирает расхождения и список счетов для сверки.
func (s *Server) reconcileView(r *http.Request) (reconcileView, error) {
	ctx := r.Context()

	discrepancies, err := s.recon.Discrepancies(ctx)
	if err != nil {
		return reconcileView{}, err
	}
	view := reconcileView{Empty: true}
	for _, d := range discrepancies {
		if d.Difference == 0 {
			continue
		}
		view.Empty = false
		view.Discrepancies = append(view.Discrepancies, reconcileRow{
			AccountID:  d.AccountID,
			Path:       d.Path,
			Fact:       d.Fact,
			Balance:    d.Balance,
			Difference: d.Difference,
			CheckedAt:  d.CheckedAt,
			Adjustable: d.Adjustable,
		})
	}

	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return reconcileView{}, err
	}
	for i := range accounts {
		if !reconcilesvc.Adjustable(accounts[i].Type) {
			continue
		}
		view.Accounts = append(view.Accounts, reconcileAccount{ID: accounts[i].ID, Path: accounts[i].FullPath})
	}
	return view, nil
}
