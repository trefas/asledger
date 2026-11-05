package httpui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	creditsvc "github.com/trefas/asledger/pkg/core/credit"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// creditAccountOption — кредитный счёт в выпадающем списке формы.
type creditAccountOption struct {
	// ID — идентификатор счёта.
	ID int64
	// FullPath — полный путь счёта.
	FullPath string
	// HasSettings — у счёта заданы настройки кредита.
	HasSettings bool
}

// creditView — данные страницы погашения кредита.
type creditView struct {
	// UUID — идентификатор проводки, переносится между предпросмотром и записью.
	UUID string
	// Date — дата погашения в формате формы.
	Date string
	// Description — описание проводки.
	Description string
	// ActorID — выбранный автор проводки.
	ActorID int64
	// Users — доступные авторы.
	Users []model.User
	// Accounts — кредитные счета (листовые пассивы).
	Accounts []creditAccountOption
	// Sources — счета-источники (листовые активы).
	Sources []model.Account
	// Interests — статьи процентов (листовые расходы).
	Interests []model.Account
	// AccountID, SourceAccountID, InterestAccountID — выбранные счета.
	AccountID         int64
	SourceAccountID   int64
	InterestAccountID int64
	// Amount — сумма платежа так, как её ввёл пользователь.
	Amount string
	// Error — сообщение об ошибке, показываемое над формой.
	Error string
	// Preview — разбивка платежа после расчёта; nil до предпросмотра.
	Preview *creditPreviewView
}

// creditPreviewView — разбивка платежа для показа пользователю.
type creditPreviewView struct {
	// Date — дата погашения в формате «ДД.ММ.ГГГГ».
	Date string
	// Requested — сумма, которую внёс пользователь.
	Requested money.Kop
	// Principal — часть суммы в погашение тела долга.
	Principal money.Kop
	// Interest — часть суммы в погашение начисленных процентов.
	Interest money.Kop
	// Total — фактически распределённая сумма.
	Total money.Kop
	// Debt и Accrued — остатки до платежа.
	Debt    money.Kop
	Accrued money.Kop
	// RemainingDebt и RemainingInterest — остатки после платежа.
	RemainingDebt     money.Kop
	RemainingInterest money.Kop
	// InGrace — на дату действует беспроцентный период.
	InGrace bool
	// Overpayment — часть суммы сверх остатка долга и процентов.
	Overpayment money.Kop
}

// creditForm — значения формы погашения кредита.
type creditForm struct {
	UUID              string
	Date              string
	Description       string
	AccountID         int64
	SourceAccountID   int64
	InterestAccountID int64
	ActorID           int64
	Amount            string
}

// handleCredits показывает форму погашения кредита.
func (s *Server) handleCredits(w http.ResponseWriter, r *http.Request) {
	view, err := s.creditView(r.Context(), creditForm{
		UUID: newUUID(),
		Date: todayString(),
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if view.InterestAccountID == 0 && len(view.Interests) > 0 {
		view.InterestAccountID = view.Interests[0].ID
	}
	s.render(w, r, "credits", pageData{Title: "Кредиты", Active: "credits", Data: view})
}

// handleCreditPreview рассчитывает разбивку платежа, не записывая проводку.
func (s *Server) handleCreditPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	form := creditFormFromRequest(r)
	view, err := s.creditView(r.Context(), form)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	form = form.resolve(view)

	input, err := s.creditInput(form)
	if err != nil {
		s.creditFailure(w, r, view, err.Error())
		return
	}
	preview, err := s.credit.Preview(r.Context(), input)
	if err != nil {
		s.creditFailure(w, r, view, statusMessage(err))
		return
	}

	view.UUID = preview.UUID
	view.Preview = creditPreviewFrom(preview)
	s.render(w, r, "credits", pageData{Title: "Кредиты", Active: "credits", Data: view})
}

// handleCreditPost проводит погашение кредита одной составной проводкой.
func (s *Server) handleCreditPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	form := creditFormFromRequest(r)
	view, err := s.creditView(ctx, form)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	form = form.resolve(view)

	input, err := s.creditInput(form)
	if err != nil {
		s.creditFailure(w, r, view, err.Error())
		return
	}

	// Повторная отправка того же погашения не должна создавать дубль.
	existing := false
	if _, err := s.db.TransactionByUUID(ctx, form.UUID); err == nil {
		existing = true
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}

	posted, err := s.credit.Post(ctx, input)
	if err != nil {
		s.creditFailure(w, r, view, statusMessage(err))
		return
	}
	if existing {
		s.redirectOK(w, r, "/credits", "Это погашение уже проведено: повтор не создал дубль")
		return
	}
	s.redirectOK(w, r, "/credits", fmt.Sprintf(
		"Погашение проведено: тело долга %s, проценты %s, списано %s",
		posted.Principal.String(), posted.Interest.String(), posted.Total.String()))
}

// creditView собирает данные страницы и переносит в неё значения формы.
func (s *Server) creditView(ctx context.Context, form creditForm) (creditView, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return creditView{}, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return creditView{}, err
	}

	view := creditView{
		UUID:        form.UUID,
		Date:        form.Date,
		Description: form.Description,
		ActorID:     form.ActorID,
		Users:       users,
		Amount:      form.Amount,
	}
	if view.Date == "" {
		view.Date = todayString()
	}
	if view.ActorID == 0 && len(users) > 0 {
		view.ActorID = users[0].ID
	}

	for _, account := range accounts {
		switch account.Type {
		case model.TypeLiability:
			hasSettings := true
			if _, err := s.db.CreditSettings(ctx, account.ID); err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return creditView{}, err
				}
				hasSettings = false
			}
			view.Accounts = append(view.Accounts, creditAccountOption{
				ID: account.ID, FullPath: account.FullPath, HasSettings: hasSettings,
			})
		case model.TypeAsset:
			view.Sources = append(view.Sources, account)
		case model.TypeExpense:
			view.Interests = append(view.Interests, account)
		}
	}

	view.AccountID = form.AccountID
	if !view.hasAccount(view.AccountID) {
		view.AccountID = view.defaultCreditAccount()
	}
	view.SourceAccountID = form.SourceAccountID
	if !view.hasSource(view.SourceAccountID) {
		view.SourceAccountID = firstAccountID(view.Sources)
	}
	view.InterestAccountID = form.InterestAccountID
	if !view.hasInterest(view.InterestAccountID) {
		view.InterestAccountID = 0
	}
	return view, nil
}

// resolve подставляет в форму значения, выбранные страницей по умолчанию.
//
// Так пустые поля не превращаются в нулевые идентификаторы: сервис получает
// тот же счёт и автора, которые пользователь видит в форме.
func (f creditForm) resolve(view creditView) creditForm {
	if f.AccountID == 0 {
		f.AccountID = view.AccountID
	}
	if f.SourceAccountID == 0 {
		f.SourceAccountID = view.SourceAccountID
	}
	if f.ActorID == 0 {
		f.ActorID = view.ActorID
	}
	if f.Date == "" {
		f.Date = view.Date
	}
	return f
}

// creditFormFromRequest читает значения формы погашения.
func creditFormFromRequest(r *http.Request) creditForm {
	form := creditForm{
		UUID:              strings.TrimSpace(r.FormValue("uuid")),
		Date:              strings.TrimSpace(r.FormValue("date")),
		Description:       strings.TrimSpace(r.FormValue("description")),
		AccountID:         parseID(r.FormValue("account_id"), 0),
		SourceAccountID:   parseID(r.FormValue("source_account_id"), 0),
		InterestAccountID: parseID(r.FormValue("interest_account_id"), 0),
		ActorID:           parseID(r.FormValue("actor_id"), 0),
		Amount:            strings.TrimSpace(r.FormValue("amount")),
	}
	if form.UUID == "" {
		form.UUID = newUUID()
	}
	if form.Date == "" {
		form.Date = todayString()
	}
	return form
}

// creditInput переводит форму в запрос сервиса погашения.
func (s *Server) creditInput(form creditForm) (creditsvc.RepaymentInput, error) {
	amount, err := money.Parse(form.Amount)
	if err != nil {
		return creditsvc.RepaymentInput{}, errors.New("укажите сумму погашения, например 1 250,00")
	}
	date, err := parseDate(form.Date)
	if err != nil {
		return creditsvc.RepaymentInput{}, err
	}
	return creditsvc.RepaymentInput{
		UUID:              form.UUID,
		Date:              date,
		AccountID:         form.AccountID,
		SourceAccountID:   form.SourceAccountID,
		InterestAccountID: form.InterestAccountID,
		Amount:            amount,
		Description:       form.Description,
		ActorID:           form.ActorID,
		Source:            "web",
	}, nil
}

// creditPreviewFrom переводит предпросмотр сервиса в данные шаблона.
func creditPreviewFrom(preview *creditsvc.Preview) *creditPreviewView {
	overpayment := preview.Requested - preview.Total
	if overpayment < 0 {
		overpayment = 0
	}
	return &creditPreviewView{
		Date:              preview.Date.Format("02.01.2006"),
		Requested:         preview.Requested,
		Principal:         preview.Principal,
		Interest:          preview.Interest,
		Total:             preview.Total,
		Debt:              preview.Debt,
		Accrued:           preview.AccruedInterest,
		RemainingDebt:     preview.RemainingDebt,
		RemainingInterest: preview.RemainingInterest,
		InGrace:           preview.InGrace,
		Overpayment:       overpayment,
	}
}

// creditFailure перерисовывает форму с сообщением об ошибке и кодом 422.
func (s *Server) creditFailure(w http.ResponseWriter, r *http.Request, view creditView, message string) {
	view.Error = message
	t, ok := s.pages["credits"]
	if !ok {
		s.fail(w, fmt.Errorf("httpui: нет шаблона страницы %q", "credits"))
		return
	}
	data := pageData{
		Title: "Кредиты", Active: "credits", Version: s.cfg.Version, Data: view,
	}
	if needsSetup, err := s.needsSetup(r.Context()); err == nil {
		data.Setup = needsSetup
		data.HasChart = !needsSetup
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать форму кредита", "err", err)
	}
}

// hasAccount сообщает, есть ли счёт в списке кредитов.
func (v creditView) hasAccount(id int64) bool {
	for _, account := range v.Accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

// hasSource сообщает, есть ли счёт в списке источников.
func (v creditView) hasSource(id int64) bool {
	for _, account := range v.Sources {
		if account.ID == id {
			return true
		}
	}
	return false
}

// hasInterest сообщает, есть ли счёт в списке статей процентов.
func (v creditView) hasInterest(id int64) bool {
	for _, account := range v.Interests {
		if account.ID == id {
			return true
		}
	}
	return false
}

// defaultCreditAccount возвращает первый кредит с настройками.
//
// Если настроек нет ни у одного счёта, выбирается первый доступный: форма
// покажет пояснение, а сервис вернёт понятную ошибку.
func (v creditView) defaultCreditAccount() int64 {
	for _, account := range v.Accounts {
		if account.HasSettings {
			return account.ID
		}
	}
	if len(v.Accounts) == 0 {
		return 0
	}
	return v.Accounts[0].ID
}

// firstAccountID возвращает идентификатор первого счёта списка.
func firstAccountID(accounts []model.Account) int64 {
	if len(accounts) == 0 {
		return 0
	}
	return accounts[0].ID
}
