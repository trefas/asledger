package httpui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// seedCreditDebt создаёт долг по кредитному счёту.
func (w *testWeb) seedCreditDebt(t *testing.T, uuid string, creditID, sourceID int64, amount money.Kop) {
	t.Helper()
	if _, err := w.db.InsertTransaction(context.Background(), &model.Transaction{
		UUID:    uuid,
		OpType:  model.OpInitial,
		Date:    creditTestDate(),
		ActorID: w.userID(t),
		Lines: []model.TransactionLine{
			{AccountID: sourceID, Amount: amount, Side: model.SideDebit},
			{AccountID: creditID, Amount: amount, Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(долг): %v", err)
	}
}

// seedCreditAccrual добавляет начисленные проценты по кредиту.
func (w *testWeb) seedCreditAccrual(t *testing.T, uuid string, creditID int64, amount money.Kop) {
	t.Helper()
	if _, err := w.db.ExecContext(context.Background(), `
		INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
		                              status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, 'accrued', 1, 1)`,
		uuid, creditID, creditTestDate().Unix(), string(model.InterestAccrualCredit),
		int64(amount)); err != nil {
		t.Fatalf("interest_accruals: %v", err)
	}
}

// creditTestDate возвращает сегодняшнюю дату без времени.
func creditTestDate() time.Time {
	now := time.Now().UTC()
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// creditFormValues собирает форму погашения кредита.
func creditFormValues(uuid string, creditID, sourceID, interestID int64, amount string) url.Values {
	form := url.Values{
		"uuid":                {uuid},
		"account_id":          {strconv.FormatInt(creditID, 10)},
		"source_account_id":   {strconv.FormatInt(sourceID, 10)},
		"interest_account_id": {strconv.FormatInt(interestID, 10)},
		"amount":              {amount},
		"date":                {creditTestDate().Format("2006-01-02")},
	}
	return form
}

// TestASL_17_CreditForm проверяет страницу погашения кредита.
func TestASL_17_CreditForm(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	rec := w.get(t, "/credits")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /credits: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Погашение кредита",
		"Обязательства:Кредит",
		"Активы:Наличные",
		"Рассчитать",
		"Подтвердить погашение",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("страница не содержит %q", want)
		}
	}
}

// TestASL_17_CreditPreview проверяет предпросмотр без записи.
func TestASL_17_CreditPreview(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	interestID := w.accountID(t, "Расходы:Продукты")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	w.seedCreditDebt(t, "preview-debt", creditID, walletID, money.FromKopecks(100_000_00))
	w.seedCreditAccrual(t, "preview-accrual", creditID, money.FromKopecks(5_000_00))

	rec := w.postForm(t, "/credits/preview",
		creditFormValues("preview-1", creditID, walletID, interestID, "20 000,00"))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /credits/preview: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Разбивка платежа",
		"15 000,00 ₽",
		"5 000,00 ₽",
		"20 000,00 ₽",
		"85 000,00 ₽",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("предпросмотр не содержит %q", want)
		}
	}

	// Предпросмотр не должен создавать проводку.
	if _, err := w.db.TransactionByUUID(ctx, "preview-1"); err == nil {
		t.Fatal("предпросмотр создал проводку")
	}
}

// TestASL_17_CreditPost проверяет проведение и идемпотентность.
func TestASL_17_CreditPost(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	interestID := w.accountID(t, "Расходы:Продукты")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	w.seedCreditDebt(t, "post-debt", creditID, walletID, money.FromKopecks(100_000_00))
	w.seedCreditAccrual(t, "post-accrual", creditID, money.FromKopecks(5_000_00))

	form := creditFormValues("post-1", creditID, walletID, interestID, "20 000,00")
	rec := w.postForm(t, "/credits/post", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /credits/post: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "/credits?flash=") {
		t.Fatalf("адрес после проведения: %q", location)
	}

	tr, err := w.db.TransactionByUUID(ctx, "post-1")
	if err != nil {
		t.Fatalf("TransactionByUUID: %v", err)
	}
	if tr.OpType != model.OpDebtPayment || len(tr.Lines) != 3 {
		t.Fatalf("проводка: %+v", tr)
	}
	var debit, credit money.Kop
	for _, line := range tr.Lines {
		if line.Side == model.SideDebit {
			debit += line.Amount
		} else {
			credit += line.Amount
		}
	}
	if debit != credit || debit != money.FromKopecks(20_000_00) {
		t.Fatalf("ΣDEBIT = %s, ΣCREDIT = %s", debit, credit)
	}

	debt, err := w.db.Balance(ctx, creditID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if debt != money.FromKopecks(85_000_00) {
		t.Fatalf("долг после платежа = %s", debt)
	}

	// Повтор формы с тем же uuid не создаёт дубль.
	rec = w.postForm(t, "/credits/post", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("повтор: код %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "%D1%83%D0%B6%D0%B5") {
		t.Fatalf("повтор не сообщил о дубле: %q", location)
	}
	debt, err = w.db.Balance(ctx, creditID)
	if err != nil {
		t.Fatalf("Balance после повтора: %v", err)
	}
	if debt != money.FromKopecks(85_000_00) {
		t.Fatalf("повтор изменил долг: %s", debt)
	}
}

// TestASL_17_CreditGrace проверяет беспроцентный период в web-форме.
func TestASL_17_CreditGrace(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	interestID := w.accountID(t, "Расходы:Продукты")
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM:      150_000,
		GraceEnabled: true,
		GraceEndDate: creditTestDate().AddDate(0, 1, 0),
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	w.seedCreditDebt(t, "grace-debt", creditID, walletID, money.FromKopecks(100_000_00))
	w.seedCreditAccrual(t, "grace-accrual", creditID, money.FromKopecks(5_000_00))

	rec := w.postForm(t, "/credits/preview",
		creditFormValues("grace-1", creditID, walletID, interestID, "20 000,00"))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /credits/preview: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "беспроцентный период") {
		t.Fatal("нет сообщения о беспроцентном периоде")
	}
	if !strings.Contains(body, "20 000,00 ₽") {
		t.Fatal("вся сумма должна идти в тело долга")
	}
}

// TestASL_17_CreditErrors проверяет ошибки формы.
func TestASL_17_CreditErrors(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	creditID := w.accountID(t, "Обязательства:Кредит")
	walletID := w.accountID(t, "Активы:Наличные")
	interestID := w.accountID(t, "Расходы:Продукты")

	// Пустая сумма.
	rec := w.postForm(t, "/credits/preview",
		creditFormValues("err-empty", creditID, walletID, interestID, ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("пустая сумма: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "укажите сумму погашения") {
		t.Fatal("нет сообщения о пустой сумме")
	}

	// У счёта нет настроек кредита.
	rec = w.postForm(t, "/credits/preview",
		creditFormValues("err-settings", creditID, walletID, interestID, "1 000,00"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("без настроек: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "у счёта нет настроек кредита") {
		t.Fatal("нет сообщения об отсутствии настроек")
	}
	if strings.Contains(body, "ERR_CREDIT") {
		t.Fatal("на странице показан машинный код ошибки")
	}

	// Источник — пассив.
	if _, err := w.db.SetCreditSettings(ctx, creditID, store.CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	w.seedCreditDebt(t, "errors-debt", creditID, walletID, money.FromKopecks(10_000_00))
	rec = w.postForm(t, "/credits/preview",
		creditFormValues("err-source", creditID, creditID, interestID, "1 000,00"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("источник-пассив: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "источник погашения должен быть листовым счётом типа «актив»") {
		t.Fatal("нет сообщения о неверном источнике")
	}
}
