package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// wantValidationCode проверяет, что ошибка — ValidationError с нужным кодом.
func wantValidationCode(t *testing.T, err error, code string) {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Code != code {
		t.Fatalf("err = %v, ожидалась ValidationError с кодом %s", err, code)
	}
}

// changelogHas проверяет наличие записи журнала изменений.
func changelogHas(t *testing.T, db *DB, entity string, id int64, op string) bool {
	t.Helper()
	entries, err := db.Changelog(context.Background(), 0)
	if err != nil {
		t.Fatalf("Changelog: %v", err)
	}
	for _, entry := range entries {
		if entry.Entity == entity && entry.EntityID == id && entry.Op == op {
			return true
		}
	}
	return false
}

// TestASL_16_CreditSettingsCRUD проверяет создание, чтение, изменение и
// удаление настроек кредита.
func TestASL_16_CreditSettingsCRUD(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	parent := mustCreateAccount(t, db, "Кредиты", model.TypeLiability, nil)
	credit := mustCreateAccount(t, db, "Карта", model.TypeLiability, &parent.ID)
	asset := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)
	graceEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	settings, err := db.SetCreditSettings(ctx, credit.ID, CreditSettingsInput{
		RatePPM:        150000,
		InterestType:   model.CreditInterestOnBalance,
		RepaymentOrder: model.RepaymentInterestFirst,
		GraceEnabled:   true,
		GraceEndDate:   graceEnd,
	})
	if err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	if settings.RatePPM != 150000 ||
		settings.InterestType != model.CreditInterestOnBalance ||
		settings.RepaymentOrder != model.RepaymentInterestFirst ||
		!settings.GraceEnabled || !settings.GraceEndDate.Equal(graceEnd) {
		t.Fatalf("настройки кредита: %+v", settings)
	}
	if !changelogHas(t, db, model.EntityCreditSettings, credit.ID, model.OpInsert) {
		t.Fatal("в журнале нет создания настроек кредита")
	}

	loaded, err := db.CreditSettings(ctx, credit.ID)
	if err != nil {
		t.Fatalf("CreditSettings: %v", err)
	}
	if loaded.RatePPM != settings.RatePPM || !loaded.GraceEndDate.Equal(graceEnd) {
		t.Fatalf("прочитанные настройки: %+v", loaded)
	}

	// Повторное сохранение обновляет запись и не создаёт дубль.
	updated, err := db.SetCreditSettings(ctx, credit.ID, CreditSettingsInput{
		RatePPM:        120000,
		RepaymentOrder: model.RepaymentPrincipalFirst,
	})
	if err != nil {
		t.Fatalf("SetCreditSettings update: %v", err)
	}
	if updated.RatePPM != 120000 || updated.InterestType != model.CreditInterestSimple ||
		updated.RepaymentOrder != model.RepaymentPrincipalFirst || updated.GraceEnabled {
		t.Fatalf("обновлённые настройки: %+v", updated)
	}
	if !changelogHas(t, db, model.EntityCreditSettings, credit.ID, model.OpUpdate) {
		t.Fatal("в журнале нет изменения настроек кредита")
	}

	// Удаление настроек не удаляет счёт.
	if err := db.DeleteCreditSettings(ctx, credit.ID); err != nil {
		t.Fatalf("DeleteCreditSettings: %v", err)
	}
	if _, err := db.CreditSettings(ctx, credit.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreditSettings после удаления = %v, ожидался ErrNotFound", err)
	}
	if _, err := db.Account(ctx, credit.ID); err != nil {
		t.Fatalf("счёт удалён вместе с настройками: %v", err)
	}
	if !changelogHas(t, db, model.EntityCreditSettings, credit.ID, model.OpDelete) {
		t.Fatal("в журнале нет удаления настроек кредита")
	}
	if err := db.DeleteCreditSettings(ctx, credit.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("повторное DeleteCreditSettings = %v, ожидался ErrNotFound", err)
	}

	// Счёт другого типа, нелистовой и архивный не подходят.
	for _, accountID := range []int64{asset.ID, parent.ID} {
		_, err := db.SetCreditSettings(ctx, accountID, CreditSettingsInput{RatePPM: 100000})
		wantValidationCode(t, err, CodeCreditAccountInvalid)
	}
	if err := db.ArchiveAccount(ctx, asset.ID); err != nil {
		t.Fatalf("ArchiveAccount: %v", err)
	}
	_, err = db.SetCreditSettings(ctx, asset.ID, CreditSettingsInput{RatePPM: 100000})
	wantValidationCode(t, err, CodeCreditAccountInvalid)

	// Несуществующий счёт.
	_, err = db.SetCreditSettings(ctx, 999999, CreditSettingsInput{RatePPM: 100000})
	wantValidationCode(t, err, CodeCreditAccountInvalid)

	// Некорректные поля.
	cases := []struct {
		name string
		in   CreditSettingsInput
		code string
	}{
		{"ставка ниже нуля", CreditSettingsInput{RatePPM: -1}, CodeCreditRate},
		{"ставка выше 100%", CreditSettingsInput{RatePPM: maxRatePPM + 1}, CodeCreditRate},
		{"способ расчёта", CreditSettingsInput{RatePPM: 1, InterestType: "annuity"}, CodeCreditInterestType},
		{"порядок погашения", CreditSettingsInput{RatePPM: 1, RepaymentOrder: "body_first"}, CodeCreditRepaymentOrder},
		{"grace без даты", CreditSettingsInput{RatePPM: 1, GraceEnabled: true}, CodeCreditGraceDate},
	}
	for _, tc := range cases {
		if _, err := db.SetCreditSettings(ctx, credit.ID, tc.in); err == nil {
			t.Fatalf("%s: ожидалась ошибка", tc.name)
		} else {
			wantValidationCode(t, err, tc.code)
		}
	}
}

// TestASL_16_DepositSettingsCRUD проверяет создание, чтение, изменение и
// удаление настроек вклада и ступеней.
func TestASL_16_DepositSettingsCRUD(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	parent := mustCreateAccount(t, db, "Вклады", model.TypeAsset, nil)
	deposit := mustCreateAccount(t, db, "Накопительный", model.TypeAsset, &parent.ID)
	liability := mustCreateAccount(t, db, "Кредит", model.TypeLiability, nil)
	opened := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	top := money.FromKopecks(200_000_00)
	settings, err := db.SetDepositSettings(ctx, deposit.ID, DepositSettingsInput{
		BaseRatePPM:      180000,
		MinBalance:       money.FromKopecks(10_000_00),
		BonusType:        model.DepositBonusRate,
		BonusRatePPM:     5000,
		AccrualFrequency: model.DepositAccrualDaily,
		Capitalization:   true,
		OpenedAt:         opened,
		Steps: []DepositStepRateInput{
			{From: money.FromKopecks(100_000_00), To: &top, RatePPM: 160000},
			{From: 0, To: ptrKop(100_000_00), RatePPM: 150000},
		},
	})
	if err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	if settings.BaseRatePPM != 180000 || settings.MinBalance != money.FromKopecks(10_000_00) ||
		settings.BonusType != model.DepositBonusRate || !settings.Capitalization ||
		!settings.OpenedAt.Equal(opened) || len(settings.Steps) != 2 {
		t.Fatalf("настройки вклада: %+v", settings)
	}
	// Ступени сортируются по нижней границе.
	if settings.Steps[0].From != 0 || settings.Steps[0].RatePPM != 150000 ||
		settings.Steps[1].From != money.FromKopecks(100_000_00) {
		t.Fatalf("порядок ступеней: %+v", settings.Steps)
	}
	if !changelogHas(t, db, model.EntityDepositSettings, deposit.ID, model.OpInsert) {
		t.Fatal("в журнале нет создания настроек вклада")
	}

	loaded, err := db.DepositSettings(ctx, deposit.ID)
	if err != nil {
		t.Fatalf("DepositSettings: %v", err)
	}
	if len(loaded.Steps) != 2 || loaded.Steps[0].RatePPM != 150000 {
		t.Fatalf("прочитанные ступени: %+v", loaded.Steps)
	}

	// Обновление заменяет полный набор ступеней.
	updated, err := db.SetDepositSettings(ctx, deposit.ID, DepositSettingsInput{
		BaseRatePPM:      170000,
		BonusType:        model.DepositBonusFixed,
		BonusAmount:      money.FromKopecks(500_00),
		AccrualFrequency: model.DepositAccrualMonthly,
	})
	if err != nil {
		t.Fatalf("SetDepositSettings update: %v", err)
	}
	if updated.BaseRatePPM != 170000 || len(updated.Steps) != 0 ||
		updated.BonusType != model.DepositBonusFixed {
		t.Fatalf("обновлённые настройки: %+v", updated)
	}
	if !changelogHas(t, db, model.EntityDepositSettings, deposit.ID, model.OpUpdate) {
		t.Fatal("в журнале нет изменения настроек вклада")
	}

	// Удаление настроек не удаляет счёт.
	if err := db.DeleteDepositSettings(ctx, deposit.ID); err != nil {
		t.Fatalf("DeleteDepositSettings: %v", err)
	}
	if _, err := db.DepositSettings(ctx, deposit.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DepositSettings после удаления = %v, ожидался ErrNotFound", err)
	}
	if _, err := db.Account(ctx, deposit.ID); err != nil {
		t.Fatalf("счёт удалён вместе с настройками: %v", err)
	}
	if !changelogHas(t, db, model.EntityDepositSettings, deposit.ID, model.OpDelete) {
		t.Fatal("в журнале нет удаления настроек вклада")
	}
	if err := db.DeleteDepositSettings(ctx, deposit.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("повторное DeleteDepositSettings = %v, ожидался ErrNotFound", err)
	}

	// Счёт другого типа и нелистовой счёт не подходят.
	for _, accountID := range []int64{liability.ID, parent.ID} {
		_, err := db.SetDepositSettings(ctx, accountID, DepositSettingsInput{BaseRatePPM: 100000})
		wantValidationCode(t, err, CodeDepositAccountInvalid)
	}
	_, err = db.SetDepositSettings(ctx, 999999, DepositSettingsInput{BaseRatePPM: 100000})
	wantValidationCode(t, err, CodeDepositAccountInvalid)

	// Некорректные поля.
	cases := []struct {
		name string
		in   DepositSettingsInput
		code string
	}{
		{"ставка ниже нуля", DepositSettingsInput{BaseRatePPM: -1}, CodeDepositRate},
		{"ставка выше 100%", DepositSettingsInput{BaseRatePPM: maxRatePPM + 1}, CodeDepositRate},
		{"отрицательный минимум", DepositSettingsInput{MinBalance: -1}, CodeDepositMinBalance},
		{"вид бонуса", DepositSettingsInput{BonusType: "cashback"}, CodeDepositBonus},
		{"фиксированный бонус", DepositSettingsInput{BonusType: model.DepositBonusFixed}, CodeDepositBonus},
		{"процентный бонус", DepositSettingsInput{BonusType: model.DepositBonusRate}, CodeDepositBonus},
		{"бонусная ставка", DepositSettingsInput{BonusType: model.DepositBonusRate, BonusRatePPM: maxRatePPM + 1}, CodeDepositBonus},
		{"частота", DepositSettingsInput{AccrualFrequency: "weekly"}, CodeDepositFrequency},
		{"даты", DepositSettingsInput{
			OpenedAt: opened,
			ClosedAt: opened.Add(-time.Hour),
		}, CodeDepositDates},
	}
	for _, tc := range cases {
		if _, err := db.SetDepositSettings(ctx, deposit.ID, tc.in); err == nil {
			t.Fatalf("%s: ожидалась ошибка", tc.name)
		} else {
			wantValidationCode(t, err, tc.code)
		}
	}

	// Пересекающиеся и некорректные ступени.
	stepCases := []struct {
		name  string
		steps []DepositStepRateInput
	}{
		{"пересечение", []DepositStepRateInput{
			{From: 0, To: ptrKop(1000), RatePPM: 100000},
			{From: 500, To: ptrKop(2000), RatePPM: 110000},
		}},
		{"открытый диапазон", []DepositStepRateInput{
			{From: 0, RatePPM: 100000},
			{From: 1000, To: ptrKop(2000), RatePPM: 110000},
		}},
		{"пустой диапазон", []DepositStepRateInput{
			{From: 1000, To: ptrKop(1000), RatePPM: 100000},
		}},
		{"отрицательная граница", []DepositStepRateInput{
			{From: -1, To: ptrKop(1000), RatePPM: 100000},
		}},
		{"ставка ступени", []DepositStepRateInput{
			{From: 0, To: ptrKop(1000), RatePPM: maxRatePPM + 1},
		}},
	}
	for _, tc := range stepCases {
		if _, err := db.SetDepositSettings(ctx, deposit.ID, DepositSettingsInput{Steps: tc.steps}); err == nil {
			t.Fatalf("%s: ожидалась ошибка", tc.name)
		} else {
			wantValidationCode(t, err, CodeDepositSteps)
		}
	}
}

// TestASL_17_AccruedInterest проверяет сумму начисленных процентов по счёту.
func TestASL_17_AccruedInterest(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	credit := mustCreateAccount(t, db, "Кредит", model.TypeLiability, nil)
	if _, err := db.SetCreditSettings(ctx, credit.ID, CreditSettingsInput{RatePPM: 150000}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	insertAccrual := func(uuid string, kind model.InterestAccrualKind, amount int64,
		status model.InterestAccrualStatus) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `
			INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
			                              status, created_at, updated_at)
			VALUES(?, ?, 1700000000, ?, ?, ?, 1, 1)`,
			uuid, credit.ID, string(kind), amount, string(status)); err != nil {
			t.Fatalf("interest_accruals %s: %v", uuid, err)
		}
	}
	insertAccrual("accrued-1", model.InterestAccrualCredit, 100, model.InterestAccrued)
	insertAccrual("accrued-2", model.InterestAccrualCredit, 250, model.InterestAccrued)
	insertAccrual("paid", model.InterestAccrualCredit, 999, model.InterestPaid)
	insertAccrual("deposit", model.InterestAccrualDeposit, 777, model.InterestAccrued)

	got, err := db.AccruedInterest(ctx, credit.ID, model.InterestAccrualCredit)
	if err != nil {
		t.Fatalf("AccruedInterest: %v", err)
	}
	if got != money.FromKopecks(350) {
		t.Fatalf("начисленные проценты = %s, ожидалось 3,50 ₽", got)
	}

	got, err = db.AccruedInterest(ctx, credit.ID, model.InterestAccrualDeposit)
	if err != nil {
		t.Fatalf("AccruedInterest(вклад): %v", err)
	}
	if got != money.FromKopecks(777) {
		t.Fatalf("начисленные проценты вклада = %s, ожидалось 7,77 ₽", got)
	}

	if _, err := db.AccruedInterest(ctx, credit.ID, model.InterestAccrualKind("fee")); err == nil {
		t.Fatal("недопустимый вид начисления принят")
	}
	if _, err := db.AccruedInterest(ctx, 999999, model.InterestAccrualCredit); !errors.Is(err, ErrNotFound) {
		t.Fatalf("неизвестный счёт: err = %v, ожидалась ErrNotFound", err)
	}
}

// ptrKop возвращает указатель на сумму в копейках.
func ptrKop(v int64) *money.Kop {
	value := money.FromKopecks(v)
	return &value
}
