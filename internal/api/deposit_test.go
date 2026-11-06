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

// apiToday возвращает сегодняшнюю дату без времени.
func apiToday() time.Time {
	now := time.Now().UTC()
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// apiDeposit создаёт вклад с настройками и остатком на вчера.
func (a *testAPI) apiDeposit(t *testing.T) *model.Account {
	t.Helper()
	ctx := context.Background()
	assets := a.account(t, "Активы")
	deposit, err := a.db.CreateAccount(ctx, store.AccountInput{
		Name: "Вклад", Type: model.TypeAsset, ParentID: &assets.ID,
	})
	if err != nil {
		t.Fatalf("CreateAccount(Вклад): %v", err)
	}
	if _, err := a.db.SetDepositSettings(ctx, deposit.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		OpenedAt:         apiToday().AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	capital, err := a.db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath(Капитал:Начальные): %v", err)
	}
	if _, err := a.db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "api-deposit-initial",
		OpType:  model.OpInitial,
		Date:    apiToday().AddDate(0, 0, -1),
		ActorID: a.actor,
		Lines: []model.TransactionLine{
			{AccountID: deposit.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: capital.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(вклад): %v", err)
	}
	if _, _, err := a.db.EnsureInterestAccrual(ctx, store.InterestAccrualInput{
		UUID: "api-deposit-accrual", AccountID: deposit.ID, Date: apiToday(),
		Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(5_000_00),
	}); err != nil {
		t.Fatalf("EnsureInterestAccrual: %v", err)
	}
	return deposit
}

// depositBody собирает запрос на снятие с вклада.
func (a *testAPI) depositBody(t *testing.T, uuid string, deposit, destination, income *model.Account, amount money.Kop) map[string]any {
	t.Helper()
	body := map[string]any{
		"date":                   now(),
		"account_id":             deposit.ID,
		"destination_account_id": destination.ID,
		"income_account_id":      income.ID,
		"amount_kop":             amount,
	}
	if uuid != "" {
		body["uuid"] = uuid
	}
	return body
}

// TestASL_18_DepositWithdrawalAPI проверяет предпросмотр, проводки и повтор.
func TestASL_18_DepositWithdrawalAPI(t *testing.T) {
	a := newTestAPI(t)
	deposit := a.apiDeposit(t)
	wallet := a.account(t, "Активы:Наличные")
	income := a.account(t, "Доходы:Зарплата")

	body := a.depositBody(t, "", deposit, wallet, income, money.FromKopecks(20_000_00))
	rec := a.do(t, http.MethodPost, "/api/v1/transactions/deposit-withdrawal/preview", a.token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var preview depositPreviewDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview: разбор: %v", err)
	}
	if preview.UUID == "" {
		t.Fatal("preview не вернул uuid")
	}
	if preview.PrincipalKop != money.FromKopecks(15_000_00) ||
		preview.InterestKop != money.FromKopecks(5_000_00) ||
		preview.TotalKop != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка: %+v", preview)
	}
	if preview.BalanceKop != money.FromKopecks(100_000_00) ||
		preview.AccruedInterestKop != money.FromKopecks(5_000_00) ||
		preview.RemainingBalanceKop != money.FromKopecks(85_000_00) {
		t.Fatalf("остатки: %+v", preview)
	}

	body["uuid"] = preview.UUID
	rec = a.do(t, http.MethodPost, "/api/v1/transactions/deposit-withdrawal", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted depositWithdrawalDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("post: разбор: %v", err)
	}
	if posted.UUID != preview.UUID ||
		posted.PrincipalKop != money.FromKopecks(15_000_00) ||
		posted.InterestKop != money.FromKopecks(5_000_00) ||
		posted.TotalKop != money.FromKopecks(20_000_00) {
		t.Fatalf("результат: %+v", posted)
	}
	if posted.PrincipalTransaction.OpType != model.OpTransfer ||
		posted.InterestTransaction == nil ||
		posted.InterestTransaction.OpType != model.OpIncome {
		t.Fatalf("проводки: %+v", posted)
	}

	// Повтор не создаёт дубль.
	rec = a.do(t, http.MethodPost, "/api/v1/transactions/deposit-withdrawal", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("повтор: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var replay depositWithdrawalDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &replay); err != nil {
		t.Fatalf("повтор: разбор: %v", err)
	}
	if replay.PrincipalTransaction.ID != posted.PrincipalTransaction.ID ||
		replay.InterestTransaction.ID != posted.InterestTransaction.ID {
		t.Fatal("повтор создал новые проводки")
	}
}

// TestASL_18_DepositWithdrawalErrorAPI проверяет ошибку без настроек.
func TestASL_18_DepositWithdrawalErrorAPI(t *testing.T) {
	a := newTestAPI(t)
	assets := a.account(t, "Активы")
	deposit, err := a.db.CreateAccount(context.Background(), store.AccountInput{
		Name: "Без настроек", Type: model.TypeAsset, ParentID: &assets.ID,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	wallet := a.account(t, "Активы:Наличные")
	income := a.account(t, "Доходы:Зарплата")

	body := a.depositBody(t, "", deposit, wallet, income, money.FromKopecks(100_00))
	rec := a.do(t, http.MethodPost, "/api/v1/transactions/deposit-withdrawal/preview", a.token, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if envelope.Error.Code != store.CodeDepositSettingsMissing {
		t.Fatalf("код ошибки = %q", envelope.Error.Code)
	}
}
