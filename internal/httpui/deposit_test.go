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

// depositTestDate возвращает сегодняшнюю дату без времени.
func depositTestDate() time.Time {
	now := time.Now().UTC()
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// seedDeposit создаёт вклад с настройками, остатком на вчера и процентами.
func (w *testWeb) seedDeposit(t *testing.T) int64 {
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
		UUID:    "deposit-initial",
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
	if _, _, err := w.db.EnsureInterestAccrual(ctx, store.InterestAccrualInput{
		UUID: "deposit-accrual", AccountID: depositID, Date: depositTestDate(),
		Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(5_000_00),
	}); err != nil {
		t.Fatalf("EnsureInterestAccrual: %v", err)
	}
	return depositID
}

// depositFormValues собирает форму снятия с вклада.
func depositFormValues(uuid string, depositID, destinationID, incomeID int64, amount string) url.Values {
	return url.Values{
		"uuid":                   {uuid},
		"account_id":             {strconv.FormatInt(depositID, 10)},
		"destination_account_id": {strconv.FormatInt(destinationID, 10)},
		"income_account_id":      {strconv.FormatInt(incomeID, 10)},
		"amount":                 {amount},
		"date":                   {depositTestDate().Format("2006-01-02")},
	}
}

// TestASL_18_DepositForm проверяет страницу снятия с вклада.
func TestASL_18_DepositForm(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	rec := w.get(t, "/deposits")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /deposits: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Снятие с вклада",
		"Активы:Наличные",
		"Доходы:Зарплата",
		"Рассчитать",
		"Подтвердить снятие",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("страница не содержит %q", want)
		}
	}
}

// TestASL_18_DepositPreview проверяет разбивку без записи.
func TestASL_18_DepositPreview(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedDeposit(t)
	destinationID := w.accountID(t, "Активы:Карта")
	incomeID := w.accountID(t, "Доходы:Зарплата")

	rec := w.postForm(t, "/deposits/preview",
		depositFormValues("preview-1", depositID, destinationID, incomeID, "20 000,00"))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /deposits/preview: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Разбивка снятия",
		"15 000,00 ₽",
		"5 000,00 ₽",
		"20 000,00 ₽",
		"85 000,00 ₽",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("предпросмотр не содержит %q", want)
		}
	}

	if _, err := w.db.TransactionByUUID(ctx, "preview-1-principal"); err == nil {
		t.Fatal("предпросмотр создал проводку")
	}
}

// TestASL_18_DepositPost проверяет проведение и идемпотентность.
func TestASL_18_DepositPost(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedDeposit(t)
	destinationID := w.accountID(t, "Активы:Карта")
	incomeID := w.accountID(t, "Доходы:Зарплата")

	form := depositFormValues("post-1", depositID, destinationID, incomeID, "20 000,00")
	rec := w.postForm(t, "/deposits/post", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /deposits/post: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "/deposits?flash=") {
		t.Fatalf("адрес после проведения: %q", location)
	}

	principal, err := w.db.TransactionByUUID(ctx, "post-1-principal")
	if err != nil {
		t.Fatalf("TransactionByUUID(тело): %v", err)
	}
	if principal.OpType != model.OpTransfer {
		t.Fatalf("тип проводки тела: %q", principal.OpType)
	}
	interest, err := w.db.TransactionByUUID(ctx, "post-1-interest")
	if err != nil {
		t.Fatalf("TransactionByUUID(проценты): %v", err)
	}
	if interest.OpType != model.OpIncome {
		t.Fatalf("тип проводки процентов: %q", interest.OpType)
	}

	balance, err := w.db.Balance(ctx, depositID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != money.FromKopecks(85_000_00) {
		t.Fatalf("остаток вклада после снятия = %s", balance)
	}

	// Повтор формы с тем же uuid не создаёт дубль.
	rec = w.postForm(t, "/deposits/post", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("повтор: код %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "%D1%83%D0%B6%D0%B5") {
		t.Fatalf("повтор не сообщил о дубле: %q", location)
	}
	balance, err = w.db.Balance(ctx, depositID)
	if err != nil {
		t.Fatalf("Balance после повтора: %v", err)
	}
	if balance != money.FromKopecks(85_000_00) {
		t.Fatalf("повтор изменил остаток: %s", balance)
	}
}

// TestASL_18_DepositErrors проверяет ошибки формы снятия.
func TestASL_18_DepositErrors(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	ctx := context.Background()

	depositID := w.seedDeposit(t)
	destinationID := w.accountID(t, "Активы:Карта")
	incomeID := w.accountID(t, "Доходы:Зарплата")

	// Пустая сумма.
	rec := w.postForm(t, "/deposits/preview",
		depositFormValues("err-empty", depositID, destinationID, incomeID, ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("пустая сумма: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "укажите сумму снятия") {
		t.Fatal("нет сообщения о пустой сумме")
	}

	// Счёт без настроек вклада.
	other, err := w.db.CreateAccount(ctx, store.AccountInput{
		Name: "Без настроек", Type: model.TypeAsset,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	rec = w.postForm(t, "/deposits/preview",
		depositFormValues("err-settings", other.ID, destinationID, incomeID, "1 000,00"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("без настроек: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "у счёта нет настроек вклада") {
		t.Fatal("нет сообщения об отсутствии настроек")
	}
	if strings.Contains(body, "ERR_DEPOSIT") {
		t.Fatal("на странице показан машинный код ошибки")
	}
}
