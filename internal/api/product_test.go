package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// productSettingsResponse — ответ настроек продукта для тестов.
type productSettingsResponse struct {
	AccountID int64             `json:"account_id"`
	Path      string            `json:"path"`
	Product   model.ProductType `json:"product"`
	Settings  json.RawMessage   `json:"settings"`
}

// getProductSettings запрашивает настройки продукта по счёту.
func (a *testAPI) getProductSettings(t *testing.T, accountID int64) productSettingsResponse {
	t.Helper()
	rec := a.do(t, http.MethodGet,
		fmt.Sprintf("/api/v1/accounts/%d/product-settings", accountID), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("product-settings: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var resp productSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("product-settings: разбор ответа: %v", err)
	}
	return resp
}

// TestASL_18_ProductSettingsAPI проверяет чтение и частичное изменение настроек.
func TestASL_18_ProductSettingsAPI(t *testing.T) {
	a := newTestAPI(t)
	credit := a.account(t, "Обязательства:Кредит")
	card := a.account(t, "Активы:Карта")
	expense := a.account(t, "Расходы:Продукты")

	// До настройки продукт известен по типу счёта, настроек ещё нет.
	resp := a.getProductSettings(t, credit.ID)
	if resp.Product != model.ProductCredit || resp.Path != "Обязательства:Кредит" {
		t.Fatalf("настройки до создания: %+v", resp)
	}
	if string(resp.Settings) != "null" {
		t.Fatalf("ожидались пустые настройки, получено %s", resp.Settings)
	}

	rec := a.do(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/accounts/%d/credit-settings", credit.ID), a.token, map[string]any{
			"rate_ppm":        150_000,
			"interest_type":   "simple",
			"repayment_order": "principal_first",
		})
	if rec.Code != http.StatusOK {
		t.Fatalf("credit-settings: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var created productSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("credit-settings: разбор ответа: %v", err)
	}
	var creditSettings model.CreditSettings
	if err := json.Unmarshal(created.Settings, &creditSettings); err != nil {
		t.Fatalf("credit-settings: разбор настроек: %v", err)
	}
	if creditSettings.RatePPM != 150_000 ||
		creditSettings.RepaymentOrder != model.RepaymentPrincipalFirst ||
		creditSettings.InterestType != model.CreditInterestSimple {
		t.Fatalf("созданные настройки: %+v", creditSettings)
	}

	// Частичное изменение сохраняет остальные поля.
	rec = a.do(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/accounts/%d/credit-settings", credit.ID), a.token, map[string]any{
			"rate_ppm": 180_000,
		})
	if rec.Code != http.StatusOK {
		t.Fatalf("credit-settings (частично): код %d, тело %s", rec.Code, rec.Body.String())
	}
	resp = a.getProductSettings(t, credit.ID)
	if err := json.Unmarshal(resp.Settings, &creditSettings); err != nil {
		t.Fatalf("credit-settings: разбор настроек: %v", err)
	}
	if creditSettings.RatePPM != 180_000 ||
		creditSettings.RepaymentOrder != model.RepaymentPrincipalFirst {
		t.Fatalf("после частичного изменения: %+v", creditSettings)
	}

	// Настройки кредита нельзя записать на актив.
	rec = a.do(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/accounts/%d/credit-settings", card.ID), a.token, map[string]any{
			"rate_ppm": 100_000,
		})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("кредит на активе: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil ||
		envelope.Error.Code != store.CodeCreditAccountInvalid {
		t.Fatalf("кредит на активе: ошибка %+v", envelope.Error)
	}

	// Настройки вклада нельзя записать на пассив.
	rec = a.do(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/accounts/%d/deposit-settings", credit.ID), a.token, map[string]any{
			"base_rate_ppm": 100_000,
		})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("вклад на пассиве: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil ||
		envelope.Error.Code != store.CodeDepositAccountInvalid {
		t.Fatalf("вклад на пассиве: ошибка %+v", envelope.Error)
	}

	// Для счёта расходов продукта нет.
	rec = a.do(t, http.MethodGet,
		fmt.Sprintf("/api/v1/accounts/%d/product-settings", expense.ID), a.token, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("настройки расходов: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil ||
		envelope.Error.Code != store.CodeProductAccountInvalid {
		t.Fatalf("настройки расходов: ошибка %+v", envelope.Error)
	}

	// Вклад со ступенями: пустой to_kop означает «до бесконечности».
	rec = a.do(t, http.MethodPatch,
		fmt.Sprintf("/api/v1/accounts/%d/deposit-settings", card.ID), a.token, map[string]any{
			"base_rate_ppm":     100_000,
			"accrual_frequency": "daily",
			"steps": []map[string]any{
				{"from_kop": 0, "to_kop": 500_000, "rate_ppm": 50_000},
				{"from_kop": 500_000, "rate_ppm": 100_000},
			},
		})
	if rec.Code != http.StatusOK {
		t.Fatalf("deposit-settings: код %d, тело %s", rec.Code, rec.Body.String())
	}
	resp = a.getProductSettings(t, card.ID)
	if resp.Product != model.ProductDeposit {
		t.Fatalf("продукт вклада: %s", resp.Product)
	}
	var depositSettings model.DepositSettings
	if err := json.Unmarshal(resp.Settings, &depositSettings); err != nil {
		t.Fatalf("deposit-settings: разбор настроек: %v", err)
	}
	if depositSettings.BaseRatePPM != 100_000 ||
		depositSettings.AccrualFrequency != model.DepositAccrualDaily ||
		len(depositSettings.Steps) != 2 {
		t.Fatalf("настройки вклада: %+v", depositSettings)
	}
	if depositSettings.Steps[1].To != nil {
		t.Fatalf("последняя ступень должна быть без верхней границы: %+v", depositSettings.Steps[1])
	}
}

// TestASL_18_ProductReportsAPI проверяет сводки и прогноз через REST API.
func TestASL_18_ProductReportsAPI(t *testing.T) {
	a := newTestAPI(t)
	ctx := context.Background()
	credit := a.account(t, "Обязательства:Кредит")
	wallet := a.account(t, "Активы:Наличные")
	card := a.account(t, "Активы:Карта")
	capital := a.account(t, "Капитал:Начальные")

	if _, err := a.db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	a.seedDebt(t, credit.ID, wallet.ID, money.FromKopecks(100_000_00))
	a.seedAccrual(t, credit.ID, money.FromKopecks(5_000_00))

	rec := a.do(t, http.MethodGet, "/api/v1/reports/credit-summary", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("credit-summary: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var summary creditSummaryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("credit-summary: разбор ответа: %v", err)
	}
	if summary.HorizonDays != 30 || len(summary.Rows) != 1 {
		t.Fatalf("credit-summary: %+v", summary)
	}
	row := summary.Rows[0]
	if row.AccountID != credit.ID ||
		row.DebtKop != money.FromKopecks(100_000_00) ||
		row.AccruedInterestKop != money.FromKopecks(5_000_00) ||
		row.TotalDebtKop != money.FromKopecks(105_000_00) {
		t.Fatalf("строка кредита: %+v", row)
	}
	if len(row.Upcoming) != 1 || row.UpcomingInterestKop <= 0 {
		t.Fatalf("предстоящие начисления кредита: %+v", row.Upcoming)
	}

	rec = a.do(t, http.MethodGet, fmt.Sprintf(
		"/api/v1/reports/credit-forecast?account_id=%d&amount=%d", credit.ID, 2_000_000),
		a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("credit-forecast: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var forecast creditForecastDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &forecast); err != nil {
		t.Fatalf("credit-forecast: разбор ответа: %v", err)
	}
	if forecast.PrincipalKop != money.FromKopecks(15_000_00) ||
		forecast.InterestKop != money.FromKopecks(5_000_00) ||
		forecast.TotalKop != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка прогноза: %+v", forecast)
	}
	if forecast.DebtAfterKop != money.FromKopecks(85_000_00) ||
		forecast.DebtChangeKop != money.FromKopecks(-15_000_00) ||
		forecast.InterestChangeKop != money.FromKopecks(-5_000_00) {
		t.Fatalf("изменение в прогнозе: %+v", forecast)
	}
	if forecast.UpcomingInterestKop <= 0 {
		t.Fatalf("прогноз предстоящих процентов: %+v", forecast)
	}

	// Вклад: остаток, накопленные проценты и прогноз начислений.
	if _, err := a.db.SetDepositSettings(ctx, card.ID, store.DepositSettingsInput{
		BaseRatePPM:      100_000,
		AccrualFrequency: model.DepositAccrualDaily,
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	if _, err := a.db.InsertTransaction(ctx, &model.Transaction{
		UUID: "api-deposit-balance", OpType: model.OpInitial,
		Date: time.Now().UTC(), ActorID: a.actor,
		Lines: []model.TransactionLine{
			{AccountID: card.ID, Amount: money.FromKopecks(50_000_00), Side: model.SideDebit},
			{AccountID: capital.ID, Amount: money.FromKopecks(50_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(вклад): %v", err)
	}
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
		                              status, created_at, updated_at)
		VALUES('api-deposit-accrual', ?, ?, 'deposit_interest', ?, 'accrued', 1, 1)`,
		card.ID, time.Now().Unix(), int64(1_000_00)); err != nil {
		t.Fatalf("interest_accruals(вклад): %v", err)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/reports/deposit-summary", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("deposit-summary: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var deposits depositSummaryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &deposits); err != nil {
		t.Fatalf("deposit-summary: разбор ответа: %v", err)
	}
	if len(deposits.Rows) != 1 {
		t.Fatalf("deposit-summary: %+v", deposits)
	}
	depositRow := deposits.Rows[0]
	if depositRow.AccountID != card.ID ||
		depositRow.BalanceKop != money.FromKopecks(50_000_00) ||
		depositRow.AccruedInterestKop != money.FromKopecks(1_000_00) ||
		depositRow.TotalKop != money.FromKopecks(51_000_00) {
		t.Fatalf("строка вклада: %+v", depositRow)
	}
	if len(depositRow.Upcoming) != 30 || depositRow.UpcomingInterestKop <= 0 {
		t.Fatalf("предстоящие начисления вклада: %+v", depositRow.Upcoming)
	}
}
