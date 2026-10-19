package httpui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// initialView — данные страницы начальных остатков.
type initialView struct {
	Assets      []model.Account
	Liabilities []model.Account
	Users       []model.User
	// Applied — стартовые остатки уже внесены (повторный ввод запрещён).
	Applied bool
	// CapitalPath — служебный счёт, строку капитала генерирует движок.
	CapitalPath string
	Date        string
}

// handleInitialForm показывает форму стартовых остатков.
func (s *Server) handleInitialForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	applied, err := s.initialApplied(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		s.fail(w, err)
		return
	}

	view := initialView{
		Users:       users,
		Applied:     applied,
		CapitalPath: engine.InitialCapitalPath,
		Date:        todayString(),
	}
	for _, acc := range accounts {
		if acc.IsSystem {
			continue
		}
		switch acc.Type {
		case model.TypeAsset:
			view.Assets = append(view.Assets, acc)
		case model.TypeLiability:
			view.Liabilities = append(view.Liabilities, acc)
		}
	}
	s.render(w, r, "initial", pageData{Title: "Начальные остатки", Active: "initial", Data: view})
}

// handleInitialSubmit вносит стартовые остатки и запрещает повторный ввод.
func (s *Server) handleInitialSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}

	applied, err := s.initialApplied(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if applied {
		s.mutationError(w, r, "/initial", "начальные остатки уже внесены; повторный ввод запрещён")
		return
	}

	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(users) == 0 {
		s.mutationError(w, r, "/initial", "сначала создайте пользователя семьи")
		return
	}

	assets, err := collectInitialEntries(r.PostForm, "asset", engine.RoleAsset)
	if err != nil {
		s.mutationError(w, r, "/initial", err.Error())
		return
	}
	liabilities, err := collectInitialEntries(r.PostForm, "liability", engine.RoleLiability)
	if err != nil {
		s.mutationError(w, r, "/initial", err.Error())
		return
	}
	if len(assets) == 0 && len(liabilities) == 0 {
		s.mutationError(w, r, "/initial", "введите хотя бы один ненулевой остаток")
		return
	}

	date, err := parseDate(r.FormValue("date"))
	if err != nil {
		s.mutationError(w, r, "/initial", err.Error())
		return
	}

	operation := engine.Operation{
		OpType:      model.OpInitial,
		UUID:        newUUID(),
		Date:        date,
		Description: "Начальные остатки",
		Source:      "web",
		Entries:     append(assets, liabilities...),
	}
	if _, err := s.engine.Post(ctx, operation, users[0].ID); err != nil {
		s.mutationError(w, r, "/initial", statusMessage(err))
		return
	}
	if err := s.engine.MarkInitialApplied(ctx); err != nil {
		s.fail(w, err)
		return
	}
	s.redirectOK(w, r, "/", initialFlash(assets, liabilities))
}

// initialApplied сообщает, внесены ли стартовые остатки.
func (s *Server) initialApplied(ctx context.Context) (bool, error) {
	value, ok, err := s.db.Setting(ctx, store.SettingInitialBalances)
	if err != nil {
		return false, err
	}
	return ok && value == "1", nil
}

// initialFlash описывает итог ввода стартовых остатков.
//
// Строку капитала генерирует движок: ΣА − ΣП = ΣК.
func initialFlash(assets, liabilities []engine.Entry) string {
	var sumAssets, sumLiabilities money.Kop
	for _, entry := range assets {
		sumAssets += entry.Amount
	}
	for _, entry := range liabilities {
		sumLiabilities += entry.Amount
	}
	return "Начальные остатки внесены; строка капитала: " + (sumAssets - sumLiabilities).String()
}

// collectInitialEntries собирает строки стартовых остатков.
//
// Список счетов в форме фиксирован, поэтому пустая сумма означает «остаток не
// указан» и строка пропускается.
func collectInitialEntries(form url.Values, prefix string, role engine.Role) ([]engine.Entry, error) {
	accounts := form[prefix+"_account"]
	amounts := form[prefix+"_amount"]

	var entries []engine.Entry
	for i := range accounts {
		raw := ""
		if i < len(amounts) {
			raw = strings.TrimSpace(amounts[i])
		}
		if raw == "" {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSpace(accounts[i]), 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("не удалось определить счёт остатка")
		}
		amount, err := parseAmount(raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, engine.Entry{AccountID: id, Amount: amount, Role: role})
	}
	return entries, nil
}
