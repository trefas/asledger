package httpui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	depositsvc "github.com/trefas/asledger/pkg/core/deposit"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// depositAccountOption — счёт вклада в выпадающем списке.
type depositAccountOption struct {
	// ID — идентификатор счёта.
	ID int64
	// FullPath — полный путь счёта.
	FullPath string
	// HasSettings — у счёта заданы настройки вклада.
	HasSettings bool
}

// depositView — данные страницы снятия с вклада.
type depositView struct {
	// UUID — базовый идентификатор операции.
	UUID string
	// Date — дата снятия в формате формы.
	Date string
	// Description — описание проводок.
	Description string
	// ActorID — выбранный автор проводок.
	ActorID int64
	// Users — доступные авторы.
	Users []model.User
	// Accounts — счета вкладов (листовые активы).
	Accounts []depositAccountOption
	// Destinations — счета-получатели (листовые активы).
	Destinations []model.Account
	// Incomes — статьи доходов (листовые доходы).
	Incomes []model.Account
	// AccountID, DestinationAccountID, IncomeAccountID — выбранные счета.
	AccountID            int64
	DestinationAccountID int64
	IncomeAccountID      int64
	// Amount — сумма снятия так, как её ввёл пользователь.
	Amount string
	// Error — сообщение об ошибке над формой.
	Error string
	// Preview — разбивка после расчёта; nil до предпросмотра.
	Preview *depositPreviewView
}

// depositPreviewView — разбивка снятия для показа пользователю.
type depositPreviewView struct {
	// Date — дата снятия в формате «ДД.ММ.ГГГГ».
	Date string
	// Requested — запрошенная сумма.
	Requested money.Kop
	// Principal и Interest — части снятия.
	Principal money.Kop
	Interest  money.Kop
	// Total — фактически распределённая сумма.
	Total money.Kop
	// Balance и Accrued — остатки до снятия.
	Balance money.Kop
	Accrued money.Kop
	// RemainingBalance и RemainingInterest — остатки после снятия.
	RemainingBalance  money.Kop
	RemainingInterest money.Kop
	// Overpayment — часть суммы сверх остатка и процентов.
	Overpayment money.Kop
}

// depositForm — значения формы снятия с вклада.
type depositForm struct {
	UUID                 string
	Date                 string
	Description          string
	AccountID            int64
	DestinationAccountID int64
	IncomeAccountID      int64
	ActorID              int64
	Amount               string
}

// handleDeposits показывает форму снятия с вклада.
func (s *Server) handleDeposits(w http.ResponseWriter, r *http.Request) {
	view, err := s.depositView(r.Context(), depositForm{
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
	if view.IncomeAccountID == 0 && len(view.Incomes) > 0 {
		view.IncomeAccountID = view.Incomes[0].ID
	}
	s.render(w, r, "deposits", pageData{Title: "Вклады", Active: "deposits", Data: view})
}

// handleDepositPreview рассчитывает разбивку снятия без записи.
func (s *Server) handleDepositPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	form := depositFormFromRequest(r)
	view, err := s.depositView(r.Context(), form)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	form = form.resolve(view)

	input, err := s.depositInput(form)
	if err != nil {
		s.depositFailure(w, r, view, err.Error())
		return
	}
	preview, err := s.deposit.Preview(r.Context(), input)
	if err != nil {
		s.depositFailure(w, r, view, statusMessage(err))
		return
	}

	view.UUID = preview.UUID
	view.Preview = depositPreviewFrom(preview)
	s.render(w, r, "deposits", pageData{Title: "Вклады", Active: "deposits", Data: view})
}

// handleDepositPost проводит снятие двумя проводками.
func (s *Server) handleDepositPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	form := depositFormFromRequest(r)
	view, err := s.depositView(ctx, form)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	form = form.resolve(view)

	input, err := s.depositInput(form)
	if err != nil {
		s.depositFailure(w, r, view, err.Error())
		return
	}

	existing, err := s.depositExists(ctx, form.UUID)
	if err != nil {
		s.fail(w, err)
		return
	}
	posted, err := s.deposit.Post(ctx, input)
	if err != nil {
		s.depositFailure(w, r, view, statusMessage(err))
		return
	}
	if existing {
		s.redirectOK(w, r, "/deposits", "Это снятие уже проведено: повтор не создал дубль")
		return
	}
	s.redirectOK(w, r, "/deposits", fmt.Sprintf(
		"Снятие проведено: тело вклада %s, проценты %s, получено %s",
		posted.Principal.String(), posted.Interest.String(), posted.Total.String()))
}

// depositView собирает данные страницы и переносит в неё значения формы.
func (s *Server) depositView(ctx context.Context, form depositForm) (depositView, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return depositView{}, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return depositView{}, err
	}

	view := depositView{
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
		case model.TypeAsset:
			hasSettings := true
			if _, err := s.db.DepositSettings(ctx, account.ID); err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return depositView{}, err
				}
				hasSettings = false
			}
			view.Accounts = append(view.Accounts, depositAccountOption{
				ID: account.ID, FullPath: account.FullPath, HasSettings: hasSettings,
			})
			view.Destinations = append(view.Destinations, account)
		case model.TypeIncome:
			view.Incomes = append(view.Incomes, account)
		}
	}

	view.AccountID = form.AccountID
	if !view.hasDeposit(view.AccountID) {
		view.AccountID = view.defaultDepositAccount()
	}
	view.DestinationAccountID = form.DestinationAccountID
	if !view.hasDestination(view.DestinationAccountID) || view.DestinationAccountID == view.AccountID {
		view.DestinationAccountID = view.defaultDestination()
	}
	view.IncomeAccountID = form.IncomeAccountID
	if !view.hasIncome(view.IncomeAccountID) {
		view.IncomeAccountID = 0
	}
	return view, nil
}

// resolve подставляет в форму значения, выбранные страницей по умолчанию.
func (f depositForm) resolve(view depositView) depositForm {
	if f.AccountID == 0 {
		f.AccountID = view.AccountID
	}
	if f.DestinationAccountID == 0 {
		f.DestinationAccountID = view.DestinationAccountID
	}
	if f.ActorID == 0 {
		f.ActorID = view.ActorID
	}
	if f.Date == "" {
		f.Date = view.Date
	}
	return f
}

// depositFormFromRequest читает значения формы снятия.
func depositFormFromRequest(r *http.Request) depositForm {
	form := depositForm{
		UUID:                 strings.TrimSpace(r.FormValue("uuid")),
		Date:                 strings.TrimSpace(r.FormValue("date")),
		Description:          strings.TrimSpace(r.FormValue("description")),
		AccountID:            parseID(r.FormValue("account_id"), 0),
		DestinationAccountID: parseID(r.FormValue("destination_account_id"), 0),
		IncomeAccountID:      parseID(r.FormValue("income_account_id"), 0),
		ActorID:              parseID(r.FormValue("actor_id"), 0),
		Amount:               strings.TrimSpace(r.FormValue("amount")),
	}
	if form.UUID == "" {
		form.UUID = newUUID()
	}
	if form.Date == "" {
		form.Date = todayString()
	}
	return form
}

// depositInput переводит форму в запрос сервиса снятия.
func (s *Server) depositInput(form depositForm) (depositsvc.WithdrawalInput, error) {
	amount, err := money.Parse(form.Amount)
	if err != nil {
		return depositsvc.WithdrawalInput{}, errors.New("укажите сумму снятия, например 20 000,00")
	}
	date, err := parseDate(form.Date)
	if err != nil {
		return depositsvc.WithdrawalInput{}, err
	}
	return depositsvc.WithdrawalInput{
		UUID:                 form.UUID,
		Date:                 date,
		AccountID:            form.AccountID,
		DestinationAccountID: form.DestinationAccountID,
		IncomeAccountID:      form.IncomeAccountID,
		Amount:               amount,
		Description:          form.Description,
		ActorID:              form.ActorID,
		Source:               "web",
	}, nil
}

// depositPreviewFrom переводит предпросмотр сервиса в данные шаблона.
func depositPreviewFrom(preview *depositsvc.Preview) *depositPreviewView {
	overpayment := preview.Requested - preview.Total
	if overpayment < 0 {
		overpayment = 0
	}
	return &depositPreviewView{
		Date:              preview.Date.Format("02.01.2006"),
		Requested:         preview.Requested,
		Principal:         preview.Principal,
		Interest:          preview.Interest,
		Total:             preview.Total,
		Balance:           preview.Balance,
		Accrued:           preview.AccruedInterest,
		RemainingBalance:  preview.RemainingBalance,
		RemainingInterest: preview.RemainingInterest,
		Overpayment:       overpayment,
	}
}

// depositFailure перерисовывает форму с сообщением об ошибке и кодом 422.
func (s *Server) depositFailure(w http.ResponseWriter, r *http.Request, view depositView, message string) {
	view.Error = message
	t, ok := s.pages["deposits"]
	if !ok {
		s.fail(w, fmt.Errorf("httpui: нет шаблона страницы %q", "deposits"))
		return
	}
	data := pageData{
		Title: "Вклады", Active: "deposits", Version: s.cfg.Version, Data: view,
	}
	if needsSetup, err := s.needsSetup(r.Context()); err == nil {
		data.Setup = needsSetup
		data.HasChart = !needsSetup
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать форму вклада", "err", err)
	}
}

// depositExists сообщает, есть ли уже проводки снятия с таким uuid.
func (s *Server) depositExists(ctx context.Context, uuid string) (bool, error) {
	for _, suffix := range []string{"-principal", "-interest"} {
		_, err := s.db.TransactionByUUID(ctx, uuid+suffix)
		switch {
		case err == nil:
			return true, nil
		case !errors.Is(err, store.ErrNotFound):
			return false, err
		}
	}
	return false, nil
}

// hasDeposit сообщает, есть ли счёт в списке вкладов.
func (v depositView) hasDeposit(id int64) bool {
	for _, account := range v.Accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

// hasDestination сообщает, есть ли счёт в списке получателей.
func (v depositView) hasDestination(id int64) bool {
	for _, account := range v.Destinations {
		if account.ID == id {
			return true
		}
	}
	return false
}

// hasIncome сообщает, есть ли статья в списке доходов.
func (v depositView) hasIncome(id int64) bool {
	for _, account := range v.Incomes {
		if account.ID == id {
			return true
		}
	}
	return false
}

// defaultDepositAccount возвращает первый вклад с настройками.
func (v depositView) defaultDepositAccount() int64 {
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

// defaultDestination возвращает первый счёт-получатель, отличный от вклада.
func (v depositView) defaultDestination() int64 {
	for _, account := range v.Destinations {
		if account.ID != v.AccountID {
			return account.ID
		}
	}
	return 0
}
