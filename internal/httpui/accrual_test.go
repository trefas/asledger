package httpui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// seedAccrualDeposit создаёт вклад с настройками и остатком на вчера.
func (w *testWeb) seedAccrualDeposit(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	depositID := w.accountID(t, "Активы:Наличные")
	if _, err := w.db.SetDepositSettings(ctx, depositID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		OpenedAt:         depositTestDate().AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	capital, err := w.db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath(Капитал:Начальные): %v", err)
	}
	if _, err := w.db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "accrual-deposit-initial",
		OpType:  model.OpInitial,
		Date:    depositTestDate().AddDate(0, 0, -1),
		ActorID: w.userID(t),
		Lines: []model.TransactionLine{
			{AccountID: depositID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: capital.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(вклад): %v", err)
	}
	return depositID
}

// seedAccrualCredit создаёт кредит с настройками и долгом на вчера.
func (w *testWeb) seedAccrualCredit(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	creditID := w.accountID(t, "Обязательства:Кредит")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	walletID := w.accountID(t, "Активы:Наличные")
	if _, err := w.db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "accrual-credit-initial",
		OpType:  model.OpInitial,
		Date:    depositTestDate().AddDate(0, 0, -1),
		ActorID: w.userID(t),
		Lines: []model.TransactionLine{
			{AccountID: walletID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: creditID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(кредит): %v", err)
	}
	return creditID
}

// accrualPeriodForm собирает форму начисления за период.
func accrualPeriodForm(uuid string, accountID, incomeID, expenseID int64) url.Values {
	return url.Values{
		"uuid":               {uuid},
		"mode":               {accrualModePeriod},
		"account_id":         {strconv.FormatInt(accountID, 10)},
		"income_account_id":  {strconv.FormatInt(incomeID, 10)},
		"expense_account_id": {strconv.FormatInt(expenseID, 10)},
		"period_from":        {depositTestDate().Format("2006-01-02")},
		"period_to":          {depositTestDate().AddDate(0, 0, 1).Format("2006-01-02")},
		"date":               {depositTestDate().Format("2006-01-02")},
	}
}

// accrualManualForm собирает форму ручного начисления.
func accrualManualForm(uuid string, accountID, incomeID, expenseID int64, amount string) url.Values {
	return url.Values{
		"uuid":               {uuid},
		"mode":               {accrualModeManual},
		"account_id":         {strconv.FormatInt(accountID, 10)},
		"income_account_id":  {strconv.FormatInt(incomeID, 10)},
		"expense_account_id": {strconv.FormatInt(expenseID, 10)},
		"date":               {depositTestDate().Format("2006-01-02")},
		"amount":             {amount},
	}
}

// TestASL_18_AccrualForm проверяет страницу начисления процентов.
func TestASL_18_AccrualForm(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	w.seedAccrualDeposit(t)

	rec := w.get(t, "/accruals")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /accruals: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Начисление процентов",
		"за период",
		"вручную",
		"Доходы:Зарплата",
		"Расходы:Продукты",
		"Начислить",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("страница не содержит %q", want)
		}
	}
}

// TestASL_18_AccrualPreview проверяет расчёт без записи.
func TestASL_18_AccrualPreview(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedAccrualDeposit(t)
	incomeID := w.accountID(t, "Доходы:Зарплата")

	rec := w.postForm(t, "/accruals/preview",
		accrualPeriodForm("preview-accrual", depositID, incomeID, 0))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /accruals/preview: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Рассчитанные начисления",
		"41,10 ₽",
		"15 %",
		"100 000,00 ₽",
		"начислено",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("предпросмотр не содержит %q", want)
		}
	}

	rows, err := w.db.InterestAccruals(ctx, depositID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("предпросмотр записал начисления: %+v", rows)
	}
}

// TestASL_18_AccrualApplyDeposit проверяет проведение и повтор.
func TestASL_18_AccrualApplyDeposit(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedAccrualDeposit(t)
	incomeID := w.accountID(t, "Доходы:Зарплата")

	form := accrualPeriodForm("apply-accrual", depositID, incomeID, 0)
	rec := w.postForm(t, "/accruals/apply", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /accruals/apply: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "/accruals?flash=") {
		t.Fatalf("адрес после начисления: %q", location)
	}

	rows, err := w.db.InterestAccruals(ctx, depositID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != model.InterestAccrued || rows[0].TransactionID != nil {
		t.Fatalf("начисление в книге: %+v", rows)
	}
	if rows[0].Amount != money.FromKopecks(41_10) {
		t.Fatalf("сумма начисления = %s", rows[0].Amount)
	}

	// Повтор за тот же период не создаёт дубль.
	rec = w.postForm(t, "/accruals/apply", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("повтор: код %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "%D1%83%D0%B6%D0%B5") {
		t.Fatalf("повтор не сообщил о дубле: %q", location)
	}
	rows, err = w.db.InterestAccruals(ctx, depositID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals после повтора: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("повтор создал дубль: %d записей", len(rows))
	}
}

// TestASL_18_AccrualCapitalization проверяет капитализацию вклада.
func TestASL_18_AccrualCapitalization(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedAccrualDeposit(t)
	incomeID := w.accountID(t, "Доходы:Зарплата")
	if _, err := w.db.SetDepositSettings(ctx, depositID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		Capitalization:   true,
		OpenedAt:         depositTestDate().AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}

	rec := w.postForm(t, "/accruals/apply",
		accrualPeriodForm("capitalize-accrual", depositID, incomeID, 0))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /accruals/apply: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rows, err := w.db.InterestAccruals(ctx, depositID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != model.InterestCapitalized || rows[0].TransactionID == nil {
		t.Fatalf("капитализация: %+v", rows)
	}
	transaction, err := w.db.Transaction(ctx, *rows[0].TransactionID)
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if transaction.OpType != model.OpInterestAccrual {
		t.Fatalf("тип проводки капитализации: %q", transaction.OpType)
	}
	balance, err := w.db.Balance(ctx, depositID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != money.FromKopecks(100_041_10) {
		t.Fatalf("остаток после капитализации = %s", balance)
	}
}

// TestASL_18_AccrualManual проверяет ручное начисление.
func TestASL_18_AccrualManual(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedAccrualDeposit(t)
	incomeID := w.accountID(t, "Доходы:Зарплата")

	form := accrualManualForm("manual-accrual", depositID, incomeID, 0, "1 000,00")
	rec := w.postForm(t, "/accruals/apply", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /accruals/apply: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rows, err := w.db.InterestAccruals(ctx, depositID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(rows) != 1 || rows[0].Amount != money.FromKopecks(1_000_00) {
		t.Fatalf("ручное начисление: %+v", rows)
	}

	// Повтор с тем же uuid идемпотентен.
	rec = w.postForm(t, "/accruals/apply", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("повтор: код %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "%D1%83%D0%B6%D0%B5") {
		t.Fatalf("повтор не сообщил о дубле: %q", location)
	}
}

// TestASL_18_AccrualCredit проверяет начисление процентов по кредиту.
func TestASL_18_AccrualCredit(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.seedAccrualCredit(t)
	expenseID := w.accountID(t, "Расходы:Продукты")

	rec := w.postForm(t, "/accruals/apply",
		accrualPeriodForm("credit-accrual", creditID, 0, expenseID))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /accruals/apply: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rows, err := w.db.InterestAccruals(ctx, creditID, model.InterestAccrualCredit, "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != model.InterestAccrued || rows[0].TransactionID == nil {
		t.Fatalf("начисление по кредиту: %+v", rows)
	}
	transaction, err := w.db.Transaction(ctx, *rows[0].TransactionID)
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if transaction.OpType != model.OpInterestAccrual {
		t.Fatalf("тип проводки: %q", transaction.OpType)
	}
	debt, err := w.db.Balance(ctx, creditID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if debt != money.FromKopecks(100_041_10) {
		t.Fatalf("долг после начисления = %s", debt)
	}
}

// TestASL_18_AccrualErrors проверяет ошибки формы начисления.
func TestASL_18_AccrualErrors(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedAccrualDeposit(t)
	creditID := w.seedAccrualCredit(t)
	incomeID := w.accountID(t, "Доходы:Зарплата")

	// Пустая сумма ручного начисления.
	rec := w.postForm(t, "/accruals/preview",
		accrualManualForm("err-empty", depositID, incomeID, 0, ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("пустая сумма: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "укажите сумму начисления") {
		t.Fatal("нет сообщения о пустой сумме")
	}

	// Счёт без настроек продукта.
	other, err := w.db.CreateAccount(ctx, store.AccountInput{
		Name: "Без настроек", Type: model.TypeAsset,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	rec = w.postForm(t, "/accruals/preview",
		accrualPeriodForm("err-settings", other.ID, incomeID, 0))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("без настроек: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "у счёта нет настроек вклада") {
		t.Fatal("нет сообщения об отсутствии настроек")
	}
	if strings.Contains(body, "ERR_INTEREST") {
		t.Fatal("на странице показан машинный код ошибки")
	}

	// В качестве статьи расходов выбрана статья доходов.
	rec = w.postForm(t, "/accruals/preview",
		accrualPeriodForm("err-expense", creditID, 0, incomeID))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("неверная статья расходов: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "статья расходов должна быть") {
		t.Fatal("нет сообщения о неверной статье расходов")
	}
}
