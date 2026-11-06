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

// apiTomorrow возвращает завтрашнюю дату без времени.
func apiTomorrow() time.Time { return apiToday().AddDate(0, 0, 1) }

// interestBody собирает запрос начисления за период.
func interestBody(accountID int64, from, to time.Time, incomeID, expenseID int64) map[string]any {
	body := map[string]any{
		"account_id":  accountID,
		"period_from": from.Format(time.RFC3339),
		"period_to":   to.Format(time.RFC3339),
	}
	if incomeID != 0 {
		body["income_account_id"] = incomeID
	}
	if expenseID != 0 {
		body["expense_account_id"] = expenseID
	}
	return body
}

// apiCredit создаёт кредит с настройками и долгом на вчера.
func (a *testAPI) apiCredit(t *testing.T) *model.Account {
	t.Helper()
	ctx := context.Background()
	credit := a.account(t, "Обязательства:Кредит")
	if _, err := a.db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	wallet := a.account(t, "Активы:Наличные")
	if _, err := a.db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "api-credit-initial",
		OpType:  model.OpInitial,
		Date:    apiToday().AddDate(0, 0, -1),
		ActorID: a.actor,
		Lines: []model.TransactionLine{
			{AccountID: wallet.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: credit.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(кредит): %v", err)
	}
	return credit
}

// TestASL_18_InterestAPI проверяет расчёт, проведение и повтор по вкладу.
func TestASL_18_InterestAPI(t *testing.T) {
	a := newTestAPI(t)
	deposit := a.apiDeposit(t)
	income := a.account(t, "Доходы:Зарплата")

	body := interestBody(deposit.ID, apiToday(), apiTomorrow(), income.ID, 0)
	rec := a.do(t, http.MethodPost, "/api/v1/interests/calculate", a.token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("calculate: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var preview interestPreviewDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("calculate: разбор: %v", err)
	}
	if len(preview.Accruals) != 1 {
		t.Fatalf("начислений = %d, ожидалось 1", len(preview.Accruals))
	}
	if preview.Accruals[0].AmountKop != money.FromKopecks(41_10) ||
		preview.Accruals[0].Status != model.InterestAccrued ||
		preview.Accruals[0].Existing {
		t.Fatalf("начисление: %+v", preview.Accruals[0])
	}
	if preview.TotalKop != money.FromKopecks(41_10) {
		t.Fatalf("итог = %s", preview.TotalKop)
	}

	rec = a.do(t, http.MethodPost, "/api/v1/interests/apply", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted interestPostedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("apply: разбор: %v", err)
	}
	if posted.AppliedKop != money.FromKopecks(41_10) || len(posted.Accruals) != 1 {
		t.Fatalf("результат: %+v", posted)
	}
	if posted.Accruals[0].Transaction != nil {
		t.Fatalf("проценты без капитализации не должны иметь проводку: %+v", posted.Accruals[0])
	}

	// Повтор за тот же период не создаёт дубль.
	rec = a.do(t, http.MethodPost, "/api/v1/interests/apply", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("повтор: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var replay interestPostedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &replay); err != nil {
		t.Fatalf("повтор: разбор: %v", err)
	}
	if replay.AppliedKop != 0 || !replay.Accruals[0].Existing {
		t.Fatalf("повтор: %+v", replay)
	}
}

// TestASL_18_InterestCapitalizationAPI проверяет капитализацию вклада.
func TestASL_18_InterestCapitalizationAPI(t *testing.T) {
	a := newTestAPI(t)
	deposit := a.apiDeposit(t)
	income := a.account(t, "Доходы:Зарплата")
	if _, err := a.db.SetDepositSettings(context.Background(), deposit.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		Capitalization:   true,
		OpenedAt:         apiToday().AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}

	body := interestBody(deposit.ID, apiToday(), apiTomorrow(), income.ID, 0)
	rec := a.do(t, http.MethodPost, "/api/v1/interests/apply", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted interestPostedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("apply: разбор: %v", err)
	}
	item := posted.Accruals[0]
	if item.Accrual.Status != model.InterestCapitalized || item.Transaction == nil {
		t.Fatalf("капитализация: %+v", item)
	}
	if item.Transaction.OpType != model.OpInterestAccrual {
		t.Fatalf("тип проводки: %q", item.Transaction.OpType)
	}
}

// TestASL_18_InterestCreditAPI проверяет начисление по кредиту.
func TestASL_18_InterestCreditAPI(t *testing.T) {
	a := newTestAPI(t)
	credit := a.apiCredit(t)
	expense := a.account(t, "Расходы:Продукты")

	body := interestBody(credit.ID, apiToday(), apiTomorrow(), 0, expense.ID)
	rec := a.do(t, http.MethodPost, "/api/v1/interests/apply", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted interestPostedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("apply: разбор: %v", err)
	}
	item := posted.Accruals[0]
	if item.Accrual.Kind != model.InterestAccrualCredit ||
		item.Accrual.Status != model.InterestAccrued ||
		item.Transaction == nil {
		t.Fatalf("начисление по кредиту: %+v", item)
	}
}

// TestASL_18_InterestManualAPI проверяет ручное начисление.
func TestASL_18_InterestManualAPI(t *testing.T) {
	a := newTestAPI(t)
	deposit := a.apiDeposit(t)
	income := a.account(t, "Доходы:Зарплата")

	body := map[string]any{
		"uuid":              "api-manual-accrual",
		"account_id":        deposit.ID,
		"date":              now(),
		"amount_kop":        money.FromKopecks(1_000_00),
		"income_account_id": income.ID,
	}
	rec := a.do(t, http.MethodPost, "/api/v1/interests/apply", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var posted interestPostedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &posted); err != nil {
		t.Fatalf("apply: разбор: %v", err)
	}
	if posted.AppliedKop != money.FromKopecks(1_000_00) {
		t.Fatalf("применено = %s", posted.AppliedKop)
	}

	rec = a.do(t, http.MethodPost, "/api/v1/interests/apply", a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("повтор: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var replay interestPostedDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &replay); err != nil {
		t.Fatalf("повтор: разбор: %v", err)
	}
	if replay.AppliedKop != 0 || !replay.Accruals[0].Existing {
		t.Fatalf("повтор: %+v", replay)
	}
}

// TestASL_18_InterestErrorAPI проверяет ошибку без статьи расходов.
func TestASL_18_InterestErrorAPI(t *testing.T) {
	a := newTestAPI(t)
	credit := a.apiCredit(t)

	body := interestBody(credit.ID, apiToday(), apiTomorrow(), 0, 0)
	rec := a.do(t, http.MethodPost, "/api/v1/interests/calculate", a.token, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if envelope.Error.Code != store.CodeInterestExpenseAccount {
		t.Fatalf("код ошибки = %q", envelope.Error.Code)
	}
}
