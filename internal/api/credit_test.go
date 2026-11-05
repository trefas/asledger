package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// seedDebt создаёт долг по кредитному счёту.
func (a *testAPI) seedDebt(t *testing.T, creditID, sourceID int64, amount money.Kop) {
	t.Helper()
	date := time.Now().UTC()
	year, month, day := date.Date()
	date = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if _, err := a.db.InsertTransaction(context.Background(), &model.Transaction{
		UUID:    "api-initial-debt",
		OpType:  model.OpInitial,
		Date:    date,
		ActorID: a.actor,
		Lines: []model.TransactionLine{
			{AccountID: sourceID, Amount: amount, Side: model.SideDebit},
			{AccountID: creditID, Amount: amount, Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(долг): %v", err)
	}
}

// seedAccrual добавляет начисленные проценты по кредиту.
func (a *testAPI) seedAccrual(t *testing.T, creditID int64, amount money.Kop) {
	t.Helper()
	if _, err := a.db.ExecContext(context.Background(), `
		INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
		                              status, created_at, updated_at)
		VALUES('api-accrual', ?, ?, ?, ?, 'accrued', 1, 1)`,
		creditID, time.Now().Unix(), string(model.InterestAccrualCredit), int64(amount)); err != nil {
		t.Fatalf("interest_accruals: %v", err)
	}
}

// creditRepaymentBody собирает тело запроса на погашение.
func creditRepaymentBody(uuid string, creditID, sourceID, interestID int64, amount money.Kop) map[string]any {
	body := map[string]any{
		"date":                now(),
		"account_id":          creditID,
		"source_account_id":   sourceID,
		"interest_account_id": interestID,
		"amount_kop":          amount,
	}
	if uuid != "" {
		body["uuid"] = uuid
	}
	return body
}

// TestASL_17_CreditRepaymentAPI проверяет предпросмотр, проводку и повтор.
func TestASL_17_CreditRepaymentAPI(t *testing.T) {
	a := newTestAPI(t)
	ctx := context.Background()
	credit := a.account(t, "Обязательства:Кредит")
	wallet := a.account(t, "Активы:Наличные")
	interest := a.account(t, "Расходы:Продукты")

	if _, err := a.db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	a.seedDebt(t, credit.ID, wallet.ID, money.FromKopecks(100_000_00))
	a.seedAccrual(t, credit.ID, money.FromKopecks(5_000_00))

	body := creditRepaymentBody("", credit.ID, wallet.ID, interest.ID, money.FromKopecks(20_000_00))
	rec := a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment/preview", a.token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var preview creditPreviewDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview: разбор ответа: %v", err)
	}
	if preview.UUID == "" {
		t.Fatal("preview не вернул uuid")
	}
	if preview.PrincipalKop != money.FromKopecks(15_000_00) ||
		preview.InterestKop != money.FromKopecks(5_000_00) ||
		preview.TotalKop != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка preview: %+v", preview)
	}
	if preview.DebtKop != money.FromKopecks(100_000_00) ||
		preview.AccruedInterestKop != money.FromKopecks(5_000_00) ||
		preview.RemainingDebtKop != money.FromKopecks(85_000_00) {
		t.Fatalf("остатки preview: %+v", preview)
	}
	if preview.InGrace {
		t.Fatal("grace не должен действовать")
	}

	// Проводка с uuid из предпросмотра.
	body["uuid"] = preview.UUID
	rec = a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted creditRepaymentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("post: разбор ответа: %v", err)
	}
	if posted.UUID != preview.UUID ||
		posted.PrincipalKop != money.FromKopecks(15_000_00) ||
		posted.InterestKop != money.FromKopecks(5_000_00) ||
		posted.TotalKop != money.FromKopecks(20_000_00) {
		t.Fatalf("результат post: %+v", posted)
	}
	if posted.Transaction.OpType != model.OpDebtPayment || len(posted.Transaction.Lines) != 3 {
		t.Fatalf("проводка: %+v", posted.Transaction)
	}

	var debit, creditTotal money.Kop
	for _, line := range posted.Transaction.Lines {
		if line.Side == model.SideDebit {
			debit += line.Amount
		} else {
			creditTotal += line.Amount
		}
	}
	if debit != creditTotal {
		t.Fatalf("ΣDEBIT = %s, ΣCREDIT = %s", debit, creditTotal)
	}

	debt, err := a.db.Balance(ctx, credit.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if debt != money.FromKopecks(85_000_00) {
		t.Fatalf("долг после платежа = %s", debt)
	}

	// Повтор с тем же uuid не создаёт новую проводку.
	rec = a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("повтор: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var replay creditRepaymentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &replay); err != nil {
		t.Fatalf("повтор: разбор ответа: %v", err)
	}
	if replay.Transaction.ID != posted.Transaction.ID {
		t.Fatalf("повтор создал новую проводку: %d != %d",
			replay.Transaction.ID, posted.Transaction.ID)
	}
	if replay.PrincipalKop != posted.PrincipalKop ||
		replay.InterestKop != posted.InterestKop ||
		replay.TotalKop != posted.TotalKop {
		t.Fatalf("повтор вернул другую разбивку: %+v", replay)
	}
}

// TestASL_17_CreditRepaymentGraceAPI проверяет беспроцентный период.
func TestASL_17_CreditRepaymentGraceAPI(t *testing.T) {
	a := newTestAPI(t)
	ctx := context.Background()
	credit := a.account(t, "Обязательства:Кредит")
	wallet := a.account(t, "Активы:Наличные")
	interest := a.account(t, "Расходы:Продукты")

	if _, err := a.db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM:      150_000,
		GraceEnabled: true,
		GraceEndDate: time.Now().UTC().AddDate(0, 1, 0),
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	a.seedDebt(t, credit.ID, wallet.ID, money.FromKopecks(100_000_00))
	a.seedAccrual(t, credit.ID, money.FromKopecks(5_000_00))

	body := creditRepaymentBody("grace-1", credit.ID, wallet.ID, interest.ID, money.FromKopecks(20_000_00))
	rec := a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment/preview", a.token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var preview creditPreviewDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview: разбор ответа: %v", err)
	}
	if !preview.InGrace {
		t.Fatal("grace должен действовать")
	}
	if preview.PrincipalKop != money.FromKopecks(20_000_00) || preview.InterestKop != 0 {
		t.Fatalf("в grace всё должно идти в тело: %+v", preview)
	}

	rec = a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted creditRepaymentDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("post: разбор ответа: %v", err)
	}
	if posted.PrincipalKop != money.FromKopecks(20_000_00) || posted.InterestKop != 0 {
		t.Fatalf("в grace всё должно идти в тело: %+v", posted)
	}
}

// TestASL_17_CreditRepaymentErrorsAPI проверяет ошибочные запросы.
func TestASL_17_CreditRepaymentErrorsAPI(t *testing.T) {
	a := newTestAPI(t)
	ctx := context.Background()
	credit := a.account(t, "Обязательства:Кредит")
	wallet := a.account(t, "Активы:Наличные")
	interest := a.account(t, "Расходы:Продукты")

	if _, err := a.db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	a.seedDebt(t, credit.ID, wallet.ID, money.FromKopecks(100_000_00))

	cases := []struct {
		name string
		body map[string]any
		code string
	}{
		{
			"нулевая сумма",
			creditRepaymentBody("err-amount", credit.ID, wallet.ID, interest.ID, 0),
			store.CodeCreditAmount,
		},
		{
			"источник не актив",
			creditRepaymentBody("err-source", credit.ID, credit.ID, interest.ID, money.FromKopecks(1000)),
			store.CodeCreditSourceInvalid,
		},
		{
			"статья процентов не расход",
			creditRepaymentBody("err-interest", credit.ID, wallet.ID, wallet.ID, money.FromKopecks(1000)),
			store.CodeCreditInterestAccountInvalid,
		},
		{
			"неизвестный кредит",
			creditRepaymentBody("err-account", 999999, wallet.ID, interest.ID, money.FromKopecks(1000)),
			store.CodeCreditAccountInvalid,
		},
	}
	for _, tc := range cases {
		rec := a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment", a.token, tc.body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: код %d, тело %s", tc.name, rec.Code, rec.Body.String())
		}
		var envelope errorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("%s: разбор ошибки: %v", tc.name, err)
		}
		if envelope.Error.Code != tc.code {
			t.Fatalf("%s: код ошибки %q, ожидался %q", tc.name, envelope.Error.Code, tc.code)
		}
	}

	// Предпросмотр без токена запрещён.
	rec := a.do(t, http.MethodPost, "/api/v1/transactions/credit-repayment/preview", "",
		creditRepaymentBody("no-auth", credit.ID, wallet.ID, interest.ID, money.FromKopecks(1000)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена: код %d", rec.Code)
	}
}
