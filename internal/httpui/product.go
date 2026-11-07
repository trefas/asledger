package httpui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// productSummary — строка списка продуктов.
type productSummary struct {
	// ID — идентификатор счёта.
	ID int64
	// Path — полный путь счёта.
	Path string
	// Product — тип продукта: кредит или вклад.
	Product model.ProductType
	// HasSettings — для счёта заданы настройки продукта.
	HasSettings bool
	// Details — короткая расшифровка настроек для списка.
	Details []string
}

// productListView — данные страницы списка продуктов.
type productListView struct {
	// Credits — листовые пассивы (кредиты).
	Credits []productSummary
	// Deposits — листовые активы (вклады).
	Deposits []productSummary
}

// productStepForm — ступень вклада в форме.
type productStepForm struct {
	// From — нижняя граница в рублях так, как её ввёл пользователь.
	From string
	// To — верхняя граница; пусто означает «до бесконечности».
	To string
	// Rate — ставка ступени в процентах.
	Rate string
}

// productForm — значения формы настроек продукта.
type productForm struct {
	// Credit.
	Rate           string
	InterestType   string
	RepaymentOrder string
	GraceEnabled   bool
	GraceEndDate   string

	// Deposit.
	BaseRate         string
	MinBalance       string
	BonusType        string
	BonusRate        string
	BonusAmount      string
	AccrualFrequency string
	Capitalization   bool
	OpenedAt         string
	ClosedAt         string
	Steps            []productStepForm
}

// productView — данные страницы настроек одного продукта.
type productView struct {
	// AccountID — счёт настроек.
	AccountID int64
	// Path и Name — полный и короткий путь счёта.
	Path string
	Name string
	// Product — тип продукта, выведенный из типа счёта.
	Product model.ProductType
	// ProductLabel — понятное название продукта.
	ProductLabel string
	// IsCredit и IsDeposit — признаки типа продукта для шаблона.
	IsCredit  bool
	IsDeposit bool
	// HasSettings — настройки уже сохранены.
	HasSettings bool
	// Error — сообщение об ошибке над формой.
	Error string
	// Form — текущие значения формы.
	Form productForm
	// InterestTypes, RepaymentOrders, BonusTypes и Frequencies — варианты списков.
	InterestTypes   []model.CreditInterestType
	RepaymentOrders []model.RepaymentOrder
	BonusTypes      []model.DepositBonusType
	Frequencies     []model.DepositAccrualFrequency
}

// handleProducts показывает список листовых активов и пассивов.
//
// Настройки привязаны к счёту, поэтому в список попадают все листовые счета
// соответствующих типов: настроенный продукт отмечен расшифровкой, остальные
// можно настроить по ссылке.
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		s.fail(w, err)
		return
	}

	view := productListView{}
	for _, account := range accounts {
		if account.IsArchived {
			continue
		}
		switch account.Type {
		case model.TypeLiability:
			summary := productSummary{
				ID: account.ID, Path: account.FullPath, Product: model.ProductCredit,
			}
			if settings, err := s.db.CreditSettings(ctx, account.ID); err == nil {
				summary.HasSettings = true
				summary.Details = creditDetails(*settings)
			} else if !errors.Is(err, store.ErrNotFound) {
				s.fail(w, err)
				return
			}
			view.Credits = append(view.Credits, summary)
		case model.TypeAsset:
			summary := productSummary{
				ID: account.ID, Path: account.FullPath, Product: model.ProductDeposit,
			}
			if settings, err := s.db.DepositSettings(ctx, account.ID); err == nil {
				summary.HasSettings = true
				summary.Details = depositDetails(*settings)
			} else if !errors.Is(err, store.ErrNotFound) {
				s.fail(w, err)
				return
			}
			view.Deposits = append(view.Deposits, summary)
		}
	}
	s.render(w, r, "products", pageData{Title: "Продукты", Active: "products", Data: view})
}

// handleProductForm показывает форму настроек продукта.
func (s *Server) handleProductForm(w http.ResponseWriter, r *http.Request) {
	account, product, err := s.productAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		s.renderError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	view, err := s.productView(r.Context(), account, product)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "product", pageData{Title: "Настройки продукта", Active: "products", Data: view})
}

// handleProductSave сохраняет настройки кредита или вклада.
func (s *Server) handleProductSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	account, product, err := s.productAccount(ctx, r.PathValue("id"))
	if err != nil {
		s.renderError(w, r, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}

	form := productFormFromRequest(r)
	view := productView{
		AccountID:       account.ID,
		Path:            account.FullPath,
		Name:            account.Name,
		Product:         product,
		ProductLabel:    productLabel(product),
		IsCredit:        product == model.ProductCredit,
		IsDeposit:       product == model.ProductDeposit,
		HasSettings:     true,
		Form:            form,
		InterestTypes:   model.CreditInterestTypes(),
		RepaymentOrders: model.RepaymentOrders(),
		BonusTypes:      model.DepositBonusTypes(),
		Frequencies:     model.DepositAccrualFrequencies(),
	}

	switch product {
	case model.ProductCredit:
		input, err := creditSettingsFromForm(form)
		if err != nil {
			s.productFailure(w, r, view, err.Error())
			return
		}
		if _, err := s.db.SetCreditSettings(ctx, account.ID, input); err != nil {
			s.productFailure(w, r, view, statusMessage(err))
			return
		}
		s.redirectOK(w, r, "/products", "Настройки кредита сохранены: "+account.FullPath)
	case model.ProductDeposit:
		input, err := depositSettingsFromForm(form)
		if err != nil {
			s.productFailure(w, r, view, err.Error())
			return
		}
		if _, err := s.db.SetDepositSettings(ctx, account.ID, input); err != nil {
			s.productFailure(w, r, view, statusMessage(err))
			return
		}
		s.redirectOK(w, r, "/products", "Настройки вклада сохранены: "+account.FullPath)
	default:
		s.renderError(w, r, http.StatusUnprocessableEntity,
			"для этого счёта нет настроек продукта")
	}
}

// productView собирает форму настроек по сохранённым значениям.
func (s *Server) productView(ctx context.Context, account *model.Account,
	product model.ProductType) (productView, error) {
	view := productView{
		AccountID:       account.ID,
		Path:            account.FullPath,
		Name:            account.Name,
		Product:         product,
		ProductLabel:    productLabel(product),
		IsCredit:        product == model.ProductCredit,
		IsDeposit:       product == model.ProductDeposit,
		InterestTypes:   model.CreditInterestTypes(),
		RepaymentOrders: model.RepaymentOrders(),
		BonusTypes:      model.DepositBonusTypes(),
		Frequencies:     model.DepositAccrualFrequencies(),
		Form:            defaultProductForm(),
	}
	switch product {
	case model.ProductCredit:
		settings, err := s.db.CreditSettings(ctx, account.ID)
		if errors.Is(err, store.ErrNotFound) {
			return view, nil
		}
		if err != nil {
			return view, err
		}
		view.HasSettings = true
		view.Form.Rate = rateValue(settings.RatePPM)
		view.Form.InterestType = string(settings.InterestType)
		view.Form.RepaymentOrder = string(settings.RepaymentOrder)
		view.Form.GraceEnabled = settings.GraceEnabled
		view.Form.GraceEndDate = dateValue(settings.GraceEndDate)
	case model.ProductDeposit:
		settings, err := s.db.DepositSettings(ctx, account.ID)
		if errors.Is(err, store.ErrNotFound) {
			return view, nil
		}
		if err != nil {
			return view, err
		}
		view.HasSettings = true
		view.Form.BaseRate = rateValue(settings.BaseRatePPM)
		view.Form.MinBalance = settings.MinBalance.String()
		view.Form.BonusType = string(settings.BonusType)
		view.Form.BonusRate = rateValue(settings.BonusRatePPM)
		view.Form.BonusAmount = settings.BonusAmount.String()
		view.Form.AccrualFrequency = string(settings.AccrualFrequency)
		view.Form.Capitalization = settings.Capitalization
		view.Form.OpenedAt = dateValue(settings.OpenedAt)
		view.Form.ClosedAt = dateValue(settings.ClosedAt)
		for _, step := range settings.Steps {
			item := productStepForm{From: step.From.String(), Rate: rateValue(step.RatePPM)}
			if step.To != nil {
				item.To = step.To.String()
			}
			view.Form.Steps = append(view.Form.Steps, item)
		}
	}
	return view, nil
}

// productFailure перерисовывает форму настроек с сообщением об ошибке.
func (s *Server) productFailure(w http.ResponseWriter, r *http.Request, view productView, message string) {
	view.Error = message
	t, ok := s.pages["product"]
	if !ok {
		s.fail(w, fmt.Errorf("httpui: нет шаблона страницы %q", "product"))
		return
	}
	data := pageData{
		Title: "Настройки продукта", Active: "products", Version: s.cfg.Version, Data: view,
	}
	if needsSetup, err := s.needsSetup(r.Context()); err == nil {
		data.Setup = needsSetup
		data.HasChart = !needsSetup
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать форму продукта", "err", err)
	}
}

// productAccount читает счёт и проверяет, что для него есть продукт.
func (s *Server) productAccount(ctx context.Context, rawID string) (*model.Account, model.ProductType, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err != nil || id <= 0 {
		return nil, "", errors.New("неверный идентификатор счёта")
	}
	account, err := s.db.Account(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, "", errors.New("счёт не найден")
	}
	if err != nil {
		return nil, "", err
	}
	if account.IsArchived || !account.IsLeaf {
		return nil, "", errors.New("настройки продукта доступны только для листового счёта: " +
			account.FullPath)
	}
	product, ok := model.ProductForAccountType(account.Type)
	if !ok {
		return nil, "", errors.New("настройки продукта доступны только для счёта типа «пассив» или «актив»: " +
			account.FullPath)
	}
	return account, product, nil
}

// defaultProductForm возвращает форму с умолчаниями store.
func defaultProductForm() productForm {
	form := productForm{
		Rate:             "0",
		InterestType:     string(model.CreditInterestSimple),
		RepaymentOrder:   string(model.RepaymentInterestFirst),
		BaseRate:         "0",
		MinBalance:       "0",
		BonusType:        string(model.DepositBonusNone),
		BonusRate:        "0",
		BonusAmount:      "0",
		AccrualFrequency: string(model.DepositAccrualMonthly),
	}
	return form
}

// productFormFromRequest читает значения формы настроек.
func productFormFromRequest(r *http.Request) productForm {
	form := productForm{
		Rate:             strings.TrimSpace(r.FormValue("rate")),
		InterestType:     strings.TrimSpace(r.FormValue("interest_type")),
		RepaymentOrder:   strings.TrimSpace(r.FormValue("repayment_order")),
		GraceEnabled:     r.FormValue("grace_enabled") == "1",
		GraceEndDate:     strings.TrimSpace(r.FormValue("grace_end_date")),
		BaseRate:         strings.TrimSpace(r.FormValue("base_rate")),
		MinBalance:       strings.TrimSpace(r.FormValue("min_balance")),
		BonusType:        strings.TrimSpace(r.FormValue("bonus_type")),
		BonusRate:        strings.TrimSpace(r.FormValue("bonus_rate")),
		BonusAmount:      strings.TrimSpace(r.FormValue("bonus_amount")),
		AccrualFrequency: strings.TrimSpace(r.FormValue("accrual_frequency")),
		Capitalization:   r.FormValue("capitalization") == "1",
		OpenedAt:         strings.TrimSpace(r.FormValue("opened_at")),
		ClosedAt:         strings.TrimSpace(r.FormValue("closed_at")),
	}
	froms := r.Form["step_from"]
	tos := r.Form["step_to"]
	rates := r.Form["step_rate"]
	count := len(froms)
	if len(tos) > count {
		count = len(tos)
	}
	if len(rates) > count {
		count = len(rates)
	}
	for i := 0; i < count; i++ {
		step := productStepForm{}
		if i < len(froms) {
			step.From = strings.TrimSpace(froms[i])
		}
		if i < len(tos) {
			step.To = strings.TrimSpace(tos[i])
		}
		if i < len(rates) {
			step.Rate = strings.TrimSpace(rates[i])
		}
		if step.From == "" && step.To == "" && step.Rate == "" {
			continue
		}
		form.Steps = append(form.Steps, step)
	}
	return form
}

// creditSettingsFromForm переводит форму в настройки кредита.
func creditSettingsFromForm(form productForm) (store.CreditSettingsInput, error) {
	rate, err := parseRateValue(form.Rate)
	if err != nil {
		return store.CreditSettingsInput{}, fmt.Errorf("ставка кредита: %w", err)
	}
	interestType := model.CreditInterestType(form.InterestType)
	if interestType == "" {
		interestType = model.CreditInterestSimple
	}
	repayment := model.RepaymentOrder(form.RepaymentOrder)
	if repayment == "" {
		repayment = model.RepaymentInterestFirst
	}
	graceEnd, err := parseOptionalDate(form.GraceEndDate)
	if err != nil {
		return store.CreditSettingsInput{}, fmt.Errorf("дата окончания беспроцентного периода: %w", err)
	}
	return store.CreditSettingsInput{
		RatePPM:        rate,
		InterestType:   interestType,
		RepaymentOrder: repayment,
		GraceEnabled:   form.GraceEnabled,
		GraceEndDate:   graceEnd,
	}, nil
}

// depositSettingsFromForm переводит форму в настройки вклада.
func depositSettingsFromForm(form productForm) (store.DepositSettingsInput, error) {
	baseRate, err := parseRateValue(form.BaseRate)
	if err != nil {
		return store.DepositSettingsInput{}, fmt.Errorf("базовая ставка: %w", err)
	}
	minBalance, err := parseOptionalMoney(form.MinBalance)
	if err != nil {
		return store.DepositSettingsInput{}, fmt.Errorf("минимальный остаток: %w", err)
	}
	bonusRate, err := parseRateValue(form.BonusRate)
	if err != nil {
		return store.DepositSettingsInput{}, fmt.Errorf("бонусная ставка: %w", err)
	}
	bonusAmount, err := parseOptionalMoney(form.BonusAmount)
	if err != nil {
		return store.DepositSettingsInput{}, fmt.Errorf("сумма бонуса: %w", err)
	}
	openedAt, err := parseOptionalDate(form.OpenedAt)
	if err != nil {
		return store.DepositSettingsInput{}, fmt.Errorf("дата открытия: %w", err)
	}
	closedAt, err := parseOptionalDate(form.ClosedAt)
	if err != nil {
		return store.DepositSettingsInput{}, fmt.Errorf("дата закрытия: %w", err)
	}
	steps := make([]store.DepositStepRateInput, 0, len(form.Steps))
	for i, step := range form.Steps {
		from, err := parseRequiredMoney(step.From)
		if err != nil {
			return store.DepositSettingsInput{}, fmt.Errorf("ступень %d: нижняя граница: %w", i+1, err)
		}
		var to *money.Kop
		if strings.TrimSpace(step.To) != "" {
			value, err := parseOptionalMoney(step.To)
			if err != nil {
				return store.DepositSettingsInput{}, fmt.Errorf("ступень %d: верхняя граница: %w", i+1, err)
			}
			to = &value
		}
		rate, err := parseRateValue(step.Rate)
		if err != nil {
			return store.DepositSettingsInput{}, fmt.Errorf("ступень %d: ставка: %w", i+1, err)
		}
		steps = append(steps, store.DepositStepRateInput{From: from, To: to, RatePPM: rate})
	}
	return store.DepositSettingsInput{
		BaseRatePPM:      baseRate,
		MinBalance:       minBalance,
		BonusType:        model.DepositBonusType(form.BonusType),
		BonusRatePPM:     bonusRate,
		BonusAmount:      bonusAmount,
		AccrualFrequency: model.DepositAccrualFrequency(form.AccrualFrequency),
		Capitalization:   form.Capitalization,
		OpenedAt:         openedAt,
		ClosedAt:         closedAt,
		Steps:            steps,
	}, nil
}

// creditDetails собирает расшифровку настроек кредита для списка.
func creditDetails(settings model.CreditSettings) []string {
	details := []string{
		"ставка " + rateLabel(settings.RatePPM),
		interestTypeLabel(settings.InterestType),
		repaymentOrderLabel(settings.RepaymentOrder),
	}
	if settings.GraceEnabled {
		if settings.GraceEndDate.IsZero() {
			details = append(details, "беспроцентный период без даты окончания")
		} else {
			details = append(details, "без процентов до "+settings.GraceEndDate.Format("02.01.2006"))
		}
	}
	return details
}

// depositDetails собирает расшифровку настроек вклада для списка.
func depositDetails(settings model.DepositSettings) []string {
	details := []string{
		"ставка " + rateLabel(settings.BaseRatePPM),
		accrualFrequencyLabel(settings.AccrualFrequency),
	}
	if settings.Capitalization {
		details = append(details, "капитализация")
	}
	if settings.BonusType != model.DepositBonusNone {
		details = append(details, bonusTypeLabel(settings.BonusType))
	}
	if settings.MinBalance > 0 {
		details = append(details, "минимум "+settings.MinBalance.String())
	}
	if len(settings.Steps) > 0 {
		details = append(details, "ступеней: "+strconv.Itoa(len(settings.Steps)))
	}
	return details
}

// productLabel переводит тип продукта в понятное название.
func productLabel(product model.ProductType) string {
	switch product {
	case model.ProductCredit:
		return "Кредит (пассив)"
	case model.ProductDeposit:
		return "Вклад (актив)"
	default:
		return string(product)
	}
}

// interestTypeLabel переводит способ расчёта процентов в кириллицу.
func interestTypeLabel(t model.CreditInterestType) string {
	switch t {
	case model.CreditInterestSimple:
		return "простые проценты на остаток долга"
	case model.CreditInterestOnBalance:
		return "проценты на остаток на дату операции"
	default:
		return string(t)
	}
}

// repaymentOrderLabel переводит порядок погашения в кириллицу.
func repaymentOrderLabel(o model.RepaymentOrder) string {
	switch o {
	case model.RepaymentInterestFirst:
		return "сначала проценты, затем тело долга"
	case model.RepaymentPrincipalFirst:
		return "сначала тело долга, затем проценты"
	default:
		return string(o)
	}
}

// bonusTypeLabel переводит вид бонуса в кириллицу.
func bonusTypeLabel(t model.DepositBonusType) string {
	switch t {
	case model.DepositBonusNone:
		return "без бонуса"
	case model.DepositBonusFixed:
		return "фиксированный бонус"
	case model.DepositBonusRate:
		return "процентный бонус"
	default:
		return string(t)
	}
}

// accrualFrequencyLabel переводит частоту начисления в кириллицу.
func accrualFrequencyLabel(f model.DepositAccrualFrequency) string {
	switch f {
	case model.DepositAccrualDaily:
		return "начисление каждый день"
	case model.DepositAccrualMonthly:
		return "начисление раз в месяц"
	default:
		return string(f)
	}
}

// rateValue выводит ppm как проценты для поля ввода: 150000 → «15».
func rateValue(ppm int64) string {
	if ppm == 0 {
		return "0"
	}
	whole := ppm / 10_000
	frac := ppm % 10_000
	if frac == 0 {
		return strconv.FormatInt(whole, 10)
	}
	text := fmt.Sprintf("%d,%04d", whole, frac)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ",")
	return text
}

// parseRateValue разбирает ставку в процентах в ppm без плавающей точки.
func parseRateValue(raw string) (int64, error) {
	text := strings.TrimSpace(raw)
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "\u00a0", "")
	text = strings.TrimSuffix(text, "%")
	text = strings.ReplaceAll(text, ",", ".")
	if text == "" {
		return 0, errors.New("укажите ставку в процентах, например 15,5")
	}
	parts := strings.Split(text, ".")
	if len(parts) > 2 {
		return 0, errors.New("ставка должна быть числом, например 15,5")
	}
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if !allDigits(whole) || (frac != "" && !allDigits(frac)) {
		return 0, errors.New("ставка должна быть числом, например 15,5")
	}
	if len(frac) > 4 {
		return 0, errors.New("ставка задаётся не более чем с четырьмя знаками после запятой")
	}
	for len(frac) < 4 {
		frac += "0"
	}
	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, errors.New("ставка должна быть числом, например 15,5")
	}
	fracValue, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errors.New("ставка должна быть числом, например 15,5")
	}
	ppm := wholeValue*10_000 + fracValue
	if ppm < 0 || ppm > 1_000_000 {
		return 0, errors.New("ставка должна быть в диапазоне 0…100 %")
	}
	return ppm, nil
}

// parseOptionalDate разбирает необязательную дату в формате ГГГГ-ММ-ДД.
func parseOptionalDate(raw string) (time.Time, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return time.Time{}, nil
	}
	date, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return time.Time{}, errors.New("неверная дата, ожидается формат ГГГГ-ММ-ДД")
	}
	return date, nil
}

// parseOptionalMoney разбирает необязательную денежную сумму.
func parseOptionalMoney(raw string) (money.Kop, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, nil
	}
	value, err := money.Parse(text)
	if err != nil {
		return 0, errors.New("неверная сумма, например 1 000,00")
	}
	return value, nil
}

// parseRequiredMoney разбирает обязательную денежную сумму.
func parseRequiredMoney(raw string) (money.Kop, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, errors.New("укажите сумму, например 1 000,00")
	}
	return parseOptionalMoney(text)
}

// parseKopField разбирает скрытое поле с суммой в копейках; ошибка — 0.
func parseKopField(raw string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// dateValue выводит дату в формате поля ввода; нулевая дата — пустая строка.
func dateValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateOnly)
}
