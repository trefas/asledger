package httpui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	accrualsvc "github.com/trefas/asledger/pkg/core/accrual"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// accrualAccountOption — счёт кредита или вклада в выпадающем списке.
type accrualAccountOption struct {
	// ID — идентификатор счёта.
	ID int64
	// FullPath — полный путь счёта.
	FullPath string
	// HasSettings — у счёта заданы настройки продукта.
	HasSettings bool
	// Capitalization — проценты вклада добавляются к остатку.
	Capitalization bool
}

// accrualItemView — одно начисление для показа пользователю.
type accrualItemView struct {
	// Date — дата начисления в формате «ДД.ММ.ГГГГ».
	Date string
	// Kind — вид начисления.
	Kind model.InterestAccrualKind
	// Amount — сумма начисления.
	Amount money.Kop
	// Base — база расчёта.
	Base money.Kop
	// RatePPM — ставка расчёта в ppm.
	RatePPM int64
	// Period — период начисления понятным текстом.
	Period string
	// Status — статус начисления.
	Status model.InterestAccrualStatus
	// Existing — начисление уже есть в книге.
	Existing bool
}

// accrualPreviewView — результат расчёта начислений.
type accrualPreviewView struct {
	// PeriodFrom и PeriodTo — период в формате «ДД.ММ.ГГГГ».
	PeriodFrom string
	PeriodTo   string
	// Capitalization — проценты вклада добавляются к остатку.
	Capitalization bool
	// Total — сумма всех начислений.
	Total money.Kop
	// Items — рассчитанные начисления.
	Items []accrualItemView
}

// accrualView — данные страницы начисления процентов.
type accrualView struct {
	// UUID — идентификатор ручного начисления.
	UUID string
	// Mode — режим: period (за период) или manual (вручную).
	Mode string
	// Date — дата ручного начисления в формате формы.
	Date string
	// PeriodFrom и PeriodTo — границы периода в формате формы.
	PeriodFrom string
	PeriodTo   string
	// Amount — сумма ручного начисления так, как её ввёл пользователь.
	Amount string
	// Description — описание проводки.
	Description string
	// ActorID — выбранный автор проводки.
	ActorID int64
	// Users — доступные авторы.
	Users []model.User
	// Accounts — счета кредитов и вкладов.
	Accounts []accrualAccountOption
	// Incomes — статьи доходов (для капитализации вклада).
	Incomes []model.Account
	// Expenses — статьи расходов (для процентов по кредиту).
	Expenses []model.Account
	// AccountID, IncomeAccountID, ExpenseAccountID — выбранные счета.
	AccountID        int64
	IncomeAccountID  int64
	ExpenseAccountID int64
	// Error — сообщение об ошибке над формой.
	Error string
	// Preview — рассчитанные начисления; nil до предпросмотра.
	Preview *accrualPreviewView
}

// accrualForm — значения формы начисления.
type accrualForm struct {
	UUID             string
	Mode             string
	Date             string
	PeriodFrom       string
	PeriodTo         string
	Amount           string
	Description      string
	AccountID        int64
	IncomeAccountID  int64
	ExpenseAccountID int64
	ActorID          int64
}

// Режимы формы начисления.
const (
	accrualModePeriod = "period"
	accrualModeManual = "manual"
)

// handleAccruals показывает форму начисления процентов.
func (s *Server) handleAccruals(w http.ResponseWriter, r *http.Request) {
	view, err := s.accrualView(r.Context(), accrualForm{
		UUID:       newUUID(),
		Mode:       accrualModePeriod,
		Date:       todayString(),
		PeriodFrom: todayString(),
		PeriodTo:   tomorrowString(),
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	s.render(w, r, "accruals", pageData{Title: "Проценты", Active: "accruals", Data: view})
}

// handleAccrualPreview рассчитывает начисления без записи в книгу.
func (s *Server) handleAccrualPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	form := accrualFormFromRequest(r)
	view, err := s.accrualView(r.Context(), form)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	form = form.resolve(view)

	input, err := accrualInput(form)
	if err != nil {
		s.accrualFailure(w, r, view, err.Error())
		return
	}
	preview, err := s.accrual.Calculate(r.Context(), input)
	if err != nil {
		s.accrualFailure(w, r, view, statusMessage(err))
		return
	}

	view.UUID = input.UUID
	view.Preview = accrualPreviewFrom(preview)
	s.render(w, r, "accruals", pageData{Title: "Проценты", Active: "accruals", Data: view})
}

// handleAccrualApply рассчитывает и записывает начисления.
func (s *Server) handleAccrualApply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	form := accrualFormFromRequest(r)
	view, err := s.accrualView(ctx, form)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(view.Users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	form = form.resolve(view)

	input, err := accrualInput(form)
	if err != nil {
		s.accrualFailure(w, r, view, err.Error())
		return
	}

	posted, err := s.accrual.Apply(ctx, input)
	if err != nil {
		s.accrualFailure(w, r, view, statusMessage(err))
		return
	}
	if posted.Applied == 0 {
		s.redirectOK(w, r, "/accruals", "Эти проценты уже начислены: повтор не создал дубль")
		return
	}
	message := "Начислено процентов: " + posted.Applied.String()
	if posted.Capitalization {
		message += " (капитализация вклада)"
	}
	s.redirectOK(w, r, "/accruals", message)
}

// accrualView собирает данные страницы и переносит в неё значения формы.
func (s *Server) accrualView(ctx context.Context, form accrualForm) (accrualView, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return accrualView{}, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return accrualView{}, err
	}

	view := accrualView{
		UUID:        form.UUID,
		Mode:        form.Mode,
		Date:        form.Date,
		PeriodFrom:  form.PeriodFrom,
		PeriodTo:    form.PeriodTo,
		Amount:      form.Amount,
		Description: form.Description,
		ActorID:     form.ActorID,
		Users:       users,
	}
	if view.Mode != accrualModeManual {
		view.Mode = accrualModePeriod
	}
	if view.Date == "" {
		view.Date = todayString()
	}
	if view.PeriodFrom == "" {
		view.PeriodFrom = todayString()
	}
	if view.PeriodTo == "" {
		view.PeriodTo = tomorrowString()
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
					return accrualView{}, err
				}
				hasSettings = false
			}
			view.Accounts = append(view.Accounts, accrualAccountOption{
				ID: account.ID, FullPath: account.FullPath,
				HasSettings: hasSettings,
			})
		case model.TypeAsset:
			hasSettings := true
			capitalization := false
			settings, err := s.db.DepositSettings(ctx, account.ID)
			if err != nil {
				if !errors.Is(err, store.ErrNotFound) {
					return accrualView{}, err
				}
				hasSettings = false
			} else {
				capitalization = settings.Capitalization
			}
			view.Accounts = append(view.Accounts, accrualAccountOption{
				ID: account.ID, FullPath: account.FullPath,
				HasSettings: hasSettings, Capitalization: capitalization,
			})
		case model.TypeIncome:
			view.Incomes = append(view.Incomes, account)
		case model.TypeExpense:
			view.Expenses = append(view.Expenses, account)
		}
	}

	view.AccountID = form.AccountID
	if !view.hasAccount(view.AccountID) {
		view.AccountID = view.defaultAccount()
	}
	view.IncomeAccountID = form.IncomeAccountID
	if !view.hasIncome(view.IncomeAccountID) {
		view.IncomeAccountID = firstAccountID(view.Incomes)
	}
	view.ExpenseAccountID = form.ExpenseAccountID
	if !view.hasExpense(view.ExpenseAccountID) {
		view.ExpenseAccountID = firstAccountID(view.Expenses)
	}
	return view, nil
}

// resolve подставляет в форму значения, выбранные страницей по умолчанию.
func (f accrualForm) resolve(view accrualView) accrualForm {
	if f.AccountID == 0 {
		f.AccountID = view.AccountID
	}
	if f.IncomeAccountID == 0 {
		f.IncomeAccountID = view.IncomeAccountID
	}
	if f.ExpenseAccountID == 0 {
		f.ExpenseAccountID = view.ExpenseAccountID
	}
	if f.ActorID == 0 {
		f.ActorID = view.ActorID
	}
	if f.Date == "" {
		f.Date = view.Date
	}
	if f.PeriodFrom == "" {
		f.PeriodFrom = view.PeriodFrom
	}
	if f.PeriodTo == "" {
		f.PeriodTo = view.PeriodTo
	}
	return f
}

// accrualFormFromRequest читает значения формы начисления.
func accrualFormFromRequest(r *http.Request) accrualForm {
	form := accrualForm{
		UUID:             strings.TrimSpace(r.FormValue("uuid")),
		Mode:             strings.TrimSpace(r.FormValue("mode")),
		Date:             strings.TrimSpace(r.FormValue("date")),
		PeriodFrom:       strings.TrimSpace(r.FormValue("period_from")),
		PeriodTo:         strings.TrimSpace(r.FormValue("period_to")),
		Amount:           strings.TrimSpace(r.FormValue("amount")),
		Description:      strings.TrimSpace(r.FormValue("description")),
		AccountID:        parseID(r.FormValue("account_id"), 0),
		IncomeAccountID:  parseID(r.FormValue("income_account_id"), 0),
		ExpenseAccountID: parseID(r.FormValue("expense_account_id"), 0),
		ActorID:          parseID(r.FormValue("actor_id"), 0),
	}
	if form.UUID == "" {
		form.UUID = newUUID()
	}
	if form.Date == "" {
		form.Date = todayString()
	}
	if form.PeriodFrom == "" {
		form.PeriodFrom = todayString()
	}
	if form.PeriodTo == "" {
		form.PeriodTo = tomorrowString()
	}
	return form
}

// accrualInput переводит форму в запрос сервиса начислений.
func accrualInput(form accrualForm) (accrualsvc.Input, error) {
	input := accrualsvc.Input{
		UUID:             form.UUID,
		AccountID:        form.AccountID,
		IncomeAccountID:  form.IncomeAccountID,
		ExpenseAccountID: form.ExpenseAccountID,
		Description:      form.Description,
		ActorID:          form.ActorID,
		Source:           "web",
	}
	if form.Mode == accrualModeManual {
		amount, err := money.Parse(form.Amount)
		if err != nil {
			return accrualsvc.Input{}, errors.New("укажите сумму начисления, например 1 250,00")
		}
		date, err := parseDate(form.Date)
		if err != nil {
			return accrualsvc.Input{}, err
		}
		input.Date = date
		input.Amount = amount
		return input, nil
	}

	from, err := time.Parse("2006-01-02", form.PeriodFrom)
	if err != nil {
		return accrualsvc.Input{}, errors.New("неверная дата начала периода, ожидается формат ГГГГ-ММ-ДД")
	}
	to, err := time.Parse("2006-01-02", form.PeriodTo)
	if err != nil {
		return accrualsvc.Input{}, errors.New("неверная дата конца периода, ожидается формат ГГГГ-ММ-ДД")
	}
	input.PeriodFrom = from
	input.PeriodTo = to
	return input, nil
}

// accrualPreviewFrom переводит результат расчёта в данные шаблона.
func accrualPreviewFrom(preview *accrualsvc.Preview) *accrualPreviewView {
	view := &accrualPreviewView{
		Capitalization: preview.Capitalization,
		Total:          preview.Total,
	}
	if !preview.PeriodFrom.IsZero() {
		view.PeriodFrom = preview.PeriodFrom.Format("02.01.2006")
	}
	if !preview.PeriodTo.IsZero() {
		view.PeriodTo = preview.PeriodTo.Format("02.01.2006")
	}
	for _, item := range preview.Accruals {
		entry := accrualItemView{
			Date:     item.Date.Format("02.01.2006"),
			Kind:     item.Kind,
			Amount:   item.Amount,
			Base:     item.Base,
			RatePPM:  item.RatePPM,
			Status:   item.Status,
			Existing: item.Existing,
		}
		if !item.PeriodFrom.IsZero() && !item.PeriodTo.IsZero() {
			entry.Period = item.PeriodFrom.Format("02.01.2006") + " — " + item.PeriodTo.Format("02.01.2006")
		} else {
			entry.Period = "ручное"
		}
		view.Items = append(view.Items, entry)
	}
	return view
}

// accrualFailure перерисовывает форму с сообщением об ошибке и кодом 422.
func (s *Server) accrualFailure(w http.ResponseWriter, r *http.Request, view accrualView, message string) {
	view.Error = message
	t, ok := s.pages["accruals"]
	if !ok {
		s.fail(w, fmt.Errorf("httpui: нет шаблона страницы %q", "accruals"))
		return
	}
	data := pageData{
		Title: "Проценты", Active: "accruals", Version: s.cfg.Version, Data: view,
	}
	if needsSetup, err := s.needsSetup(r.Context()); err == nil {
		data.Setup = needsSetup
		data.HasChart = !needsSetup
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать форму начисления", "err", err)
	}
}

// hasAccount сообщает, есть ли счёт в списке кредитов и вкладов.
func (v accrualView) hasAccount(id int64) bool {
	for _, account := range v.Accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

// hasIncome сообщает, есть ли статья в списке доходов.
func (v accrualView) hasIncome(id int64) bool {
	for _, account := range v.Incomes {
		if account.ID == id {
			return true
		}
	}
	return false
}

// hasExpense сообщает, есть ли статья в списке расходов.
func (v accrualView) hasExpense(id int64) bool {
	for _, account := range v.Expenses {
		if account.ID == id {
			return true
		}
	}
	return false
}

// defaultAccount возвращает первый счёт с настройками продукта.
func (v accrualView) defaultAccount() int64 {
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

// tomorrowString возвращает завтрашнюю дату в формате формы.
func tomorrowString() string {
	return time.Now().AddDate(0, 0, 1).Format("2006-01-02")
}

// rateLabel выводит ставку в ppm понятным текстом.
func rateLabel(ppm int64) string {
	if ppm == 0 {
		return "0 %"
	}
	whole := ppm / 10_000
	frac := ppm % 10_000
	if frac == 0 {
		return strconv.FormatInt(whole, 10) + " %"
	}
	value := fmt.Sprintf("%d,%04d", whole, frac)
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ",")
	return value + " %"
}

// kindLabel выводит вид начисления понятным текстом.
func kindLabel(kind model.InterestAccrualKind) string {
	switch kind {
	case model.InterestAccrualCredit:
		return "проценты по кредиту"
	case model.InterestAccrualDeposit:
		return "проценты по вкладу"
	case model.InterestAccrualBonus:
		return "бонус по вкладу"
	default:
		return string(kind)
	}
}

// statusLabel выводит статус начисления понятным текстом.
func statusLabel(status model.InterestAccrualStatus) string {
	switch status {
	case model.InterestAccrued:
		return "начислено"
	case model.InterestPaid:
		return "выплачено"
	case model.InterestCapitalized:
		return "капитализировано"
	default:
		return string(status)
	}
}
