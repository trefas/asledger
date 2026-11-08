package httpui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// creditSettingsForm собирает форму настроек кредита.
func creditSettingsForm(rate, interestType, repayment string) url.Values {
	return url.Values{
		"rate":              {rate},
		"interest_type":     {interestType},
		"repayment_order":   {repayment},
		"grace_enabled":     {""},
		"grace_end_date":    {""},
		"base_rate":         {"0"},
		"min_balance":       {"0"},
		"bonus_type":        {string(model.DepositBonusNone)},
		"bonus_rate":        {"0"},
		"bonus_amount":      {"0"},
		"accrual_frequency": {string(model.DepositAccrualMonthly)},
	}
}

// depositSettingsForm собирает форму настроек вклада без ступеней.
func depositSettingsForm(baseRate string) url.Values {
	return url.Values{
		"rate":              {"0"},
		"interest_type":     {string(model.CreditInterestSimple)},
		"repayment_order":   {string(model.RepaymentInterestFirst)},
		"grace_enabled":     {""},
		"grace_end_date":    {""},
		"base_rate":         {baseRate},
		"min_balance":       {"0"},
		"bonus_type":        {string(model.DepositBonusNone)},
		"bonus_rate":        {"0"},
		"bonus_amount":      {"0"},
		"accrual_frequency": {string(model.DepositAccrualMonthly)},
		"capitalization":    {""},
		"opened_at":         {""},
		"closed_at":         {""},
	}
}

// TestASL_19_ProductList проверяет список кредитов и вкладов.
func TestASL_19_ProductList(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM:        155_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	rec := w.get(t, "/products")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /products: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Кредиты", "Вклады",
		"Обязательства:Кредит", "ставка 15,5 %",
		"Активы:Наличные", "Активы:Карта", "не настроен",
		"Настроить",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("список продуктов не содержит %q", want)
		}
	}
}

// TestASL_19_ProductCreditForm проверяет форму и сохранение настроек кредита.
func TestASL_19_ProductCreditForm(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()
	creditID := w.accountID(t, "Обязательства:Кредит")

	rec := w.get(t, "/products/"+strconvID(creditID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /products/%d: код %d, тело %s", creditID, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Условия кредита", "Ставка, % годовых", "Порядок погашения",
		"беспроцентный период", "Кредит (пассив)", "Сохранить настройки",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("форма кредита не содержит %q", want)
		}
	}

	form := creditSettingsForm("15,5", string(model.CreditInterestSimple),
		string(model.RepaymentPrincipalFirst))
	form.Set("grace_enabled", "1")
	form.Set("grace_end_date", "2027-01-31")
	rec = w.postForm(t, "/products/"+strconvID(creditID), form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сохранение кредита: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if message := flashOf(t, rec); !strings.Contains(message, "Настройки кредита сохранены") {
		t.Errorf("сообщение о сохранении: %q", message)
	}

	settings, err := w.db.CreditSettings(ctx, creditID)
	if err != nil {
		t.Fatalf("CreditSettings: %v", err)
	}
	if settings.RatePPM != 155_000 {
		t.Errorf("ставка %d ppm, ожидалось 155000", settings.RatePPM)
	}
	if settings.RepaymentOrder != model.RepaymentPrincipalFirst {
		t.Errorf("порядок погашения %q", settings.RepaymentOrder)
	}
	if !settings.GraceEnabled || settings.GraceEndDate.Format(time.DateOnly) != "2027-01-31" {
		t.Errorf("беспроцентный период сохранён неверно: %+v", settings)
	}

	// Повторный просмотр показывает сохранённые значения.
	rec = w.get(t, "/products/"+strconvID(creditID))
	if rec.Code != http.StatusOK {
		t.Fatalf("повторный GET: код %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{`value="15,5"`, `value="2027-01-31"`} {
		if !strings.Contains(body, want) {
			t.Errorf("форма не показывает сохранённое значение %q", want)
		}
	}

	t.Run("неверная ставка", func(t *testing.T) {
		rec := w.postForm(t, "/products/"+strconvID(creditID),
			creditSettingsForm("сто", string(model.CreditInterestSimple),
				string(model.RepaymentInterestFirst)))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидалось 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "ставка кредита") {
			t.Error("нет сообщения о неверной ставке")
		}
	})

	t.Run("беспроцентный период без даты", func(t *testing.T) {
		form := creditSettingsForm("10", string(model.CreditInterestSimple),
			string(model.RepaymentInterestFirst))
		form.Set("grace_enabled", "1")
		rec := w.postForm(t, "/products/"+strconvID(creditID), form)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидалось 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "дата") &&
			!strings.Contains(rec.Body.String(), "беспроцентн") {
			t.Error("нет сообщения о дате беспроцентного периода")
		}
	})

	t.Run("счёт без продукта", func(t *testing.T) {
		expenseID := w.accountID(t, "Расходы:Продукты")
		rec := w.get(t, "/products/"+strconvID(expenseID))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидалось 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "только для счёта типа") {
			t.Error("нет сообщения о неподходящем типе счёта")
		}
	})
}

// TestASL_19_ProductDepositForm проверяет форму и сохранение настроек вклада.
func TestASL_19_ProductDepositForm(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()
	depositID := w.accountID(t, "Активы:Карта")

	rec := w.get(t, "/products/"+strconvID(depositID))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /products/%d: код %d, тело %s", depositID, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Условия вклада", "Базовая ставка", "Минимальный остаток",
		"Ступени ставки", "Добавить ступень", "Бонус", "Вклад (актив)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("форма вклада не содержит %q", want)
		}
	}

	form := depositSettingsForm("9,25")
	form.Set("min_balance", "10 000,00")
	form.Set("bonus_type", string(model.DepositBonusFixed))
	form.Set("bonus_amount", "500,00")
	form.Set("capitalization", "1")
	form.Set("opened_at", "2026-01-01")
	form["step_from"] = []string{"0", "50000"}
	form["step_to"] = []string{"50000", ""}
	form["step_rate"] = []string{"8", "10,5"}

	rec = w.postForm(t, "/products/"+strconvID(depositID), form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сохранение вклада: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if message := flashOf(t, rec); !strings.Contains(message, "Настройки вклада сохранены") {
		t.Errorf("сообщение о сохранении: %q", message)
	}

	settings, err := w.db.DepositSettings(ctx, depositID)
	if err != nil {
		t.Fatalf("DepositSettings: %v", err)
	}
	if settings.BaseRatePPM != 92_500 {
		t.Errorf("базовая ставка %d ppm, ожидалось 92500", settings.BaseRatePPM)
	}
	if settings.MinBalance != money.FromKopecks(10_000_00) {
		t.Errorf("минимальный остаток %s", settings.MinBalance)
	}
	if settings.BonusType != model.DepositBonusFixed ||
		settings.BonusAmount != money.FromKopecks(500_00) {
		t.Errorf("бонус сохранён неверно: %+v", settings)
	}
	if !settings.Capitalization {
		t.Error("капитализация не сохранилась")
	}
	if len(settings.Steps) != 2 {
		t.Fatalf("ступеней %d, ожидалось 2", len(settings.Steps))
	}
	if settings.Steps[0].RatePPM != 80_000 || settings.Steps[1].RatePPM != 105_000 {
		t.Errorf("ставки ступеней: %+v", settings.Steps)
	}
	if settings.Steps[1].To != nil {
		t.Errorf("последняя ступень должна быть без верхней границы")
	}

	// Повторный просмотр показывает ступени.
	rec = w.get(t, "/products/"+strconvID(depositID))
	if rec.Code != http.StatusOK {
		t.Fatalf("повторный GET: код %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{`value="9,25"`, `value="50 000,00 ₽"`, `value="10,5"`} {
		if !strings.Contains(body, want) {
			t.Errorf("форма не показывает сохранённое значение %q", want)
		}
	}

	t.Run("пересекающиеся ступени", func(t *testing.T) {
		form := depositSettingsForm("9")
		form["step_from"] = []string{"0", "100"}
		form["step_to"] = []string{"200", ""}
		form["step_rate"] = []string{"8", "10"}
		rec := w.postForm(t, "/products/"+strconvID(depositID), form)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидалось 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "пересека") {
			t.Error("нет сообщения о пересечении ступеней")
		}
	})

	t.Run("неверная сумма", func(t *testing.T) {
		form := depositSettingsForm("9")
		form.Set("min_balance", "много")
		rec := w.postForm(t, "/products/"+strconvID(depositID), form)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидалось 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "минимальный остаток") {
			t.Error("нет сообщения о минимальном остатке")
		}
	})
}

// TestASL_19_PreviewRequired проверяет запрет проведения без предпросмотра.
func TestASL_19_PreviewRequired(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	interestID := w.accountID(t, "Расходы:Продукты")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	w.seedCreditDebt(t, "no-preview-debt", creditID, walletID, money.FromKopecks(100_000_00))
	w.seedCreditAccrual(t, "no-preview-accrual", creditID, money.FromKopecks(5_000_00))

	rec := w.postForm(t, "/credits/post",
		creditFormValues("no-preview-1", creditID, walletID, interestID, "20 000,00"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("кредит без предпросмотра: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Сначала рассчитайте разбивку") {
		t.Error("нет сообщения об обязательном предпросмотре кредита")
	}
	if _, err := w.db.TransactionByUUID(ctx, "no-preview-1"); err == nil {
		t.Fatal("проводка создана без предпросмотра")
	}

	depositID := w.seedDeposit(t)
	destinationID := w.accountID(t, "Активы:Карта")
	incomeID := w.accountID(t, "Доходы:Зарплата")
	rec = w.postForm(t, "/deposits/post",
		depositFormValues("no-preview-2", depositID, destinationID, incomeID, "20 000,00"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("вклад без предпросмотра: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Сначала рассчитайте разбивку") {
		t.Error("нет сообщения об обязательном предпросмотре вклада")
	}
	if _, err := w.db.TransactionByUUID(ctx, "no-preview-2-principal"); err == nil {
		t.Fatal("снятие с вклада проведено без предпросмотра")
	}

	t.Run("устаревшая разбивка", func(t *testing.T) {
		form := creditFormValues("stale-1", creditID, walletID, interestID, "20 000,00")
		previewRec := w.postForm(t, "/credits/preview", form)
		if previewRec.Code != http.StatusOK {
			t.Fatalf("предпросмотр: код %d", previewRec.Code)
		}
		for key, value := range previewValues(t, previewRec.Body.String()) {
			form.Set(key, value[0])
		}
		// Сумма изменилась после расчёта — сервер обязан отказать.
		form.Set("amount", "30 000,00")
		rec := w.postForm(t, "/credits/post", form)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидалось 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "изменились после расчёта") {
			t.Error("нет сообщения об устаревшей разбивке")
		}
	})
}

// TestASL_19_ReportsProducts проверяет сводки, графики и прогноз в отчётах.
func TestASL_19_ReportsProducts(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	w.seedCreditDebt(t, "reports-debt", creditID, walletID, money.FromKopecks(100_000_00))
	w.seedCreditAccrual(t, "reports-accrual", creditID, money.FromKopecks(5_000_00))

	depositID := w.seedDeposit(t)

	rec := w.get(t, "/reports")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /reports: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Сводка по кредитам", "Сводка по вкладам",
		"Обязательства:Кредит", "Активы:Наличные",
		"Прогноз погашения кредита", "Рассчитать прогноз",
		"105 000,00 ₽", "15 %",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("отчёты не содержат %q", want)
		}
	}
	if !strings.Contains(body, `class="chart"`) || !strings.Contains(body, "<svg") {
		t.Error("в отчётах нет SVG-графиков")
	}
	if !strings.Contains(body, `class="table-scroll"`) {
		t.Error("таблицы сводок не адаптированы для прокрутки")
	}

	t.Run("прогноз по параметрам", func(t *testing.T) {
		query := "/reports?forecast_account=" + strconvID(creditID) +
			"&forecast_amount=" + url.QueryEscape("20 000,00") +
			"&forecast_horizon=30"
		rec := w.get(t, query)
		if rec.Code != http.StatusOK {
			t.Fatalf("прогноз: код %d, тело %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{
			"До платежа", "После платежа", "Изменение",
			"Тело долга", "Начисленные проценты", "Всего долг",
			"Платёж 20 000,00 ₽",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("прогноз не содержит %q", want)
			}
		}
	})

	t.Run("неверный горизонт", func(t *testing.T) {
		query := "/reports?forecast_account=" + strconvID(creditID) +
			"&forecast_amount=" + url.QueryEscape("20 000,00") +
			"&forecast_horizon=999"
		rec := w.get(t, query)
		if rec.Code != http.StatusOK {
			t.Fatalf("прогноз: код %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "от 1 до 366") {
			t.Error("нет сообщения о неверном горизонте")
		}
	})

	t.Run("счёт вклада в прогнозе", func(t *testing.T) {
		query := "/reports?forecast_account=" + strconvID(depositID) +
			"&forecast_amount=" + url.QueryEscape("1 000,00")
		rec := w.get(t, query)
		if rec.Code != http.StatusOK {
			t.Fatalf("прогноз по вкладу: код %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "кредита") {
			t.Error("нет сообщения об ошибке выбора счёта")
		}
	})
}

// TestASL_19_BarChart проверяет целочисленную геометрию SVG-графика.
func TestASL_19_BarChart(t *testing.T) {
	chart := newBarChart("Долг", []string{"Кредит", "Вклад"},
		[]money.Kop{money.FromKopecks(100_00), money.FromKopecks(50_00)}, true)
	if len(chart.Bars) != 2 {
		t.Fatalf("столбцов %d, ожидалось 2", len(chart.Bars))
	}
	if chart.Bars[0].Height <= chart.Bars[1].Height {
		t.Errorf("высота столбцов не отражает значения: %d и %d",
			chart.Bars[0].Height, chart.Bars[1].Height)
	}
	for _, bar := range chart.Bars {
		if bar.Height < 0 || bar.Y < 0 || bar.X < 0 {
			t.Errorf("отрицательная геометрия столбца: %+v", bar)
		}
		if bar.ValueLabel == "" {
			t.Error("нет подписи суммы")
		}
	}

	if empty := newBarChart("Пусто", nil, nil, false); len(empty.Bars) != 0 {
		t.Error("пустой график содержит столбцы")
	}
	if zero := newBarChart("Ноль", []string{"а"}, []money.Kop{0}, false); len(zero.Bars) != 0 {
		t.Error("график с нулевым максимумом содержит столбцы")
	}

	long := newBarChart("Длинно", []string{"Очень длинное название счёта для подписи"},
		[]money.Kop{money.FromKopecks(1_00)}, false)
	if len(long.Bars) != 1 {
		t.Fatal("нет столбца для длинной подписи")
	}
	if label := []rune(long.Bars[0].Label); len(label) > 16 {
		t.Errorf("подпись не сокращена: %q", long.Bars[0].Label)
	}
}

// seedDepositBalance вносит начальный остаток вклада на дату.
func (w *testWeb) seedDepositBalance(t *testing.T, uuid string, depositID int64,
	amount money.Kop, date time.Time) {
	t.Helper()
	capital, err := w.db.AccountByPath(context.Background(), "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath(Капитал:Начальные): %v", err)
	}
	if _, err := w.db.InsertTransaction(context.Background(), &model.Transaction{
		UUID:    uuid,
		OpType:  model.OpInitial,
		Date:    date,
		ActorID: w.userID(t),
		Lines: []model.TransactionLine{
			{AccountID: depositID, Amount: amount, Side: model.SideDebit},
			{AccountID: capital.ID, Amount: amount, Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(остаток вклада): %v", err)
	}
}

// TestASL_16_ProductsE2E проходит сценарий эпика целиком: настроить кредит,
// погасить его с предпросмотром, настроить вклад, начислить проценты, снять
// деньги и проверить сводки и прогноз в отчётах.
func TestASL_16_ProductsE2E(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	interestID := w.accountID(t, "Расходы:Продукты")

	// 1. Настройка кредита через web-форму.
	rec := w.postForm(t, "/products/"+strconvID(creditID),
		creditSettingsForm("15", string(model.CreditInterestSimple),
			string(model.RepaymentInterestFirst)))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("настройки кредита: код %d, тело %s", rec.Code, rec.Body.String())
	}

	// 2. Долг и начисленные проценты.
	w.seedCreditDebt(t, "e2e-debt", creditID, walletID, money.FromKopecks(100_000_00))
	w.seedCreditAccrual(t, "e2e-accrual", creditID, money.FromKopecks(5_000_00))

	rec = w.get(t, "/reports")
	if rec.Code != http.StatusOK {
		t.Fatalf("отчёты до погашения: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "105 000,00 ₽") {
		t.Error("сводка не показывает долг с процентами до погашения")
	}

	// 3. Предпросмотр и проведение погашения.
	form := creditFormValues("e2e-repayment", creditID, walletID, interestID, "20 000,00")
	rec = w.postForm(t, "/credits/preview", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("предпросмотр погашения: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"15 000,00 ₽", "5 000,00 ₽"} {
		if !strings.Contains(body, want) {
			t.Errorf("предпросмотр погашения не содержит %q", want)
		}
	}
	for key, value := range previewValues(t, body) {
		form.Set(key, value[0])
	}
	rec = w.postForm(t, "/credits/post", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("проведение погашения: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if _, err := w.db.TransactionByUUID(ctx, "e2e-repayment"); err != nil {
		t.Fatalf("проводка погашения не найдена: %v", err)
	}

	// 4. Прогноз после погашения.
	query := "/reports?forecast_account=" + strconvID(creditID) +
		"&forecast_amount=" + url.QueryEscape("10 000,00")
	rec = w.get(t, query)
	if rec.Code != http.StatusOK {
		t.Fatalf("прогноз: код %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "85 000,00 ₽") {
		t.Error("сводка не обновилась после погашения")
	}
	if !strings.Contains(body, "75 000,00 ₽") {
		t.Error("прогноз не показывает долг после платежа")
	}

	// 5. Настройка вклада и начальный остаток.
	depositID := w.accountID(t, "Активы:Карта")
	cardID := w.accountID(t, "Активы:Наличные")
	incomeID := w.accountID(t, "Доходы:Зарплата")
	depositForm := depositSettingsForm("15")
	depositForm.Set("opened_at", depositTestDate().AddDate(0, 0, -2).Format(time.DateOnly))
	rec = w.postForm(t, "/products/"+strconvID(depositID), depositForm)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("настройки вклада: код %d, тело %s", rec.Code, rec.Body.String())
	}
	w.seedDepositBalance(t, "e2e-deposit", depositID,
		money.FromKopecks(100_000_00), depositTestDate().AddDate(0, 0, -2))

	// 6. Начисление процентов за день через web-форму.
	accrualForm := url.Values{
		"uuid":               {"e2e-accrual-run"},
		"mode":               {accrualModePeriod},
		"account_id":         {strconvID(depositID)},
		"income_account_id":  {strconvID(incomeID)},
		"expense_account_id": {strconvID(interestID)},
		"period_from":        {depositTestDate().AddDate(0, 0, -1).Format(time.DateOnly)},
		"period_to":          {depositTestDate().Format(time.DateOnly)},
		"date":               {depositTestDate().Format(time.DateOnly)},
	}
	rec = w.postForm(t, "/accruals/preview", accrualForm)
	if rec.Code != http.StatusOK {
		t.Fatalf("предпросмотр начисления: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "41,10 ₽") {
		t.Error("предпросмотр начисления не показывает проценты за день")
	}
	rec = w.postForm(t, "/accruals/apply", accrualForm)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("начисление процентов: код %d, тело %s", rec.Code, rec.Body.String())
	}

	// 7. Предпросмотр и проведение снятия.
	withdrawal := depositFormValues("e2e-withdrawal", depositID, cardID, incomeID, "20 000,00")
	rec = w.postForm(t, "/deposits/preview", withdrawal)
	if rec.Code != http.StatusOK {
		t.Fatalf("предпросмотр снятия: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{"19 958,90 ₽", "41,10 ₽"} {
		if !strings.Contains(body, want) {
			t.Errorf("предпросмотр снятия не содержит %q", want)
		}
	}
	for key, value := range previewValues(t, body) {
		withdrawal.Set(key, value[0])
	}
	rec = w.postForm(t, "/deposits/post", withdrawal)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("проведение снятия: код %d, тело %s", rec.Code, rec.Body.String())
	}

	// 8. Итоговая сводка: тело вклада уменьшилось, проценты погашены.
	rec = w.get(t, "/reports")
	if rec.Code != http.StatusOK {
		t.Fatalf("отчёты после снятия: код %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "80 041,10 ₽") {
		t.Error("сводка по вкладам не показывает остаток после снятия")
	}
	if !strings.Contains(body, "Активы:Карта") {
		t.Error("в сводке нет счёта вклада")
	}
}
