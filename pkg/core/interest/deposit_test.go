package interest

import (
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// depositSettings возвращает корректные настройки вклада для тестов.
func depositSettings() model.DepositSettings {
	return model.DepositSettings{
		BaseRatePPM:      150_000,
		BonusType:        model.DepositBonusNone,
		AccrualFrequency: model.DepositAccrualMonthly,
	}
}

// day возвращает дату в UTC.
func day(year int, month time.Month, dayOfMonth int) time.Time {
	return time.Date(year, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
}

// TestASL_18_DepositDaily проверяет ежедневное начисление на фактический
// остаток.
func TestASL_18_DepositDaily(t *testing.T) {
	calc := DepositCalculator{}
	settings := depositSettings()
	settings.AccrualFrequency = model.DepositAccrualDaily
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: settings,
	}

	accruals, err := calc.Accrue(account, day(2026, time.January, 1), day(2026, time.January, 4))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 3 {
		t.Fatalf("начислений = %d, ожидалось 3", len(accruals))
	}
	for i, accrual := range accruals {
		if accrual.Kind != model.InterestAccrualDeposit {
			t.Fatalf("вид начисления %d = %q", i, accrual.Kind)
		}
		if accrual.Amount != money.FromKopecks(41_10) {
			t.Fatalf("сумма дня %d = %s, ожидалось 41,10 ₽", i+1, accrual.Amount)
		}
		if accrual.Base != money.FromKopecks(100_000_00) || accrual.Rate != Rate(150_000) || accrual.Days != 1 {
			t.Fatalf("параметры дня %d: %+v", i+1, accrual)
		}
	}
	if !accruals[0].PeriodFrom.Equal(day(2026, time.January, 1)) ||
		!accruals[0].PeriodTo.Equal(day(2026, time.January, 2)) ||
		!accruals[2].Date.Equal(day(2026, time.January, 4)) {
		t.Fatalf("периоды начислений: %+v", accruals)
	}
}

// TestASL_18_DepositDailyCapitalization проверяет капитализацию процентов.
func TestASL_18_DepositDailyCapitalization(t *testing.T) {
	calc := DepositCalculator{}
	settings := depositSettings()
	settings.AccrualFrequency = model.DepositAccrualDaily
	settings.Capitalization = true
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: settings,
	}

	accruals, err := calc.Accrue(account, day(2026, time.January, 1), day(2026, time.January, 4))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 3 {
		t.Fatalf("начислений = %d, ожидалось 3", len(accruals))
	}
	wantAmounts := []money.Kop{
		money.FromKopecks(41_10),
		money.FromKopecks(41_11),
		money.FromKopecks(41_13),
	}
	wantBases := []money.Kop{
		money.FromKopecks(100_000_00),
		money.FromKopecks(100_041_10),
		money.FromKopecks(100_082_21),
	}
	for i := range accruals {
		if accruals[i].Amount != wantAmounts[i] || accruals[i].Base != wantBases[i] {
			t.Fatalf("день %d: %+v, ожидалось amount=%s base=%s",
				i+1, accruals[i], wantAmounts[i], wantBases[i])
		}
	}
}

// TestASL_18_DepositMonthlyMinBalance проверяет расчёт по минимальному
// остатку за месяц.
func TestASL_18_DepositMonthlyMinBalance(t *testing.T) {
	calc := DepositCalculator{}
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: depositSettings(),
		Movements: []DepositMovement{
			{Date: day(2026, time.January, 15), Amount: money.FromKopecks(-50_000_00)},
		},
	}

	accruals, err := calc.Accrue(account, day(2026, time.January, 1), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 1 {
		t.Fatalf("начислений = %d, ожидалось 1", len(accruals))
	}
	accrual := accruals[0]
	if accrual.Amount != money.FromKopecks(636_99) {
		t.Fatalf("сумма = %s, ожидалось 636,99 ₽", accrual.Amount)
	}
	if accrual.Base != money.FromKopecks(50_000_00) || accrual.Rate != Rate(150_000) || accrual.Days != 31 {
		t.Fatalf("параметры начисления: %+v", accrual)
	}
	if !accrual.Date.Equal(day(2026, time.February, 1)) ||
		!accrual.PeriodFrom.Equal(day(2026, time.January, 1)) ||
		!accrual.PeriodTo.Equal(day(2026, time.February, 1)) {
		t.Fatalf("период начисления: %+v", accrual)
	}
}

// TestASL_18_DepositSteps проверяет выбор ступенчатой ставки по остатку.
func TestASL_18_DepositSteps(t *testing.T) {
	calc := DepositCalculator{}
	settings := depositSettings()
	settings.AccrualFrequency = model.DepositAccrualDaily
	fiveMillion := money.FromKopecks(50_000_00)
	settings.Steps = []model.DepositStepRate{
		{From: 0, To: &fiveMillion, RatePPM: 100_000},
		{From: fiveMillion, RatePPM: 200_000},
	}
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: settings,
	}

	accruals, err := calc.Accrue(account, day(2026, time.January, 1), day(2026, time.January, 2))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 1 {
		t.Fatalf("начислений = %d, ожидалось 1", len(accruals))
	}
	if accruals[0].Rate != Rate(200_000) || accruals[0].Amount != money.FromKopecks(54_79) {
		t.Fatalf("ступенчатая ставка: %+v", accruals[0])
	}

	// Минимальный остаток за месяц попадает в нижнюю ступень.
	settings.AccrualFrequency = model.DepositAccrualMonthly
	account.Settings = settings
	account.Movements = []DepositMovement{
		{Date: day(2026, time.January, 15), Amount: money.FromKopecks(-80_000_00)},
	}
	accruals, err = calc.Accrue(account, day(2026, time.January, 1), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 1 {
		t.Fatalf("начислений = %d, ожидалось 1", len(accruals))
	}
	if accruals[0].Rate != Rate(100_000) || accruals[0].Base != money.FromKopecks(20_000_00) ||
		accruals[0].Amount != money.FromKopecks(169_86) {
		t.Fatalf("ставка по минимальному остатку: %+v", accruals[0])
	}
}

// TestASL_18_DepositBonus проверяет фиксированный и процентный бонусы.
func TestASL_18_DepositBonus(t *testing.T) {
	calc := DepositCalculator{}
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: depositSettings(),
	}

	// Фиксированный бонус — отдельная строка.
	account.Settings.BonusType = model.DepositBonusFixed
	account.Settings.BonusAmount = money.FromKopecks(500_00)
	accruals, err := calc.Accrue(account, day(2026, time.January, 1), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 2 {
		t.Fatalf("начислений = %d, ожидалось 2", len(accruals))
	}
	if accruals[0].Kind != model.InterestAccrualDeposit || accruals[0].Amount != money.FromKopecks(1_273_97) {
		t.Fatalf("проценты: %+v", accruals[0])
	}
	if accruals[1].Kind != model.InterestAccrualBonus || accruals[1].Amount != money.FromKopecks(500_00) {
		t.Fatalf("фиксированный бонус: %+v", accruals[1])
	}

	// Процентный бонус считается на минимальный остаток.
	account.Settings.BonusType = model.DepositBonusRate
	account.Settings.BonusAmount = 0
	account.Settings.BonusRatePPM = 50_000
	accruals, err = calc.Accrue(account, day(2026, time.January, 1), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 2 {
		t.Fatalf("начислений = %d, ожидалось 2", len(accruals))
	}
	if accruals[1].Kind != model.InterestAccrualBonus ||
		accruals[1].Amount != money.FromKopecks(424_66) ||
		accruals[1].Rate != Rate(50_000) {
		t.Fatalf("процентный бонус: %+v", accruals[1])
	}
}

// TestASL_18_DepositMonthlyCapitalization проверяет капитализацию по месяцам.
func TestASL_18_DepositMonthlyCapitalization(t *testing.T) {
	calc := DepositCalculator{}
	settings := depositSettings()
	settings.Capitalization = true
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: settings,
	}

	accruals, err := calc.Accrue(account, day(2026, time.January, 1), day(2026, time.March, 1))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if len(accruals) != 2 {
		t.Fatalf("начислений = %d, ожидалось 2", len(accruals))
	}
	if accruals[0].Amount != money.FromKopecks(1_273_97) {
		t.Fatalf("январь: %+v", accruals[0])
	}
	if accruals[1].Base != money.FromKopecks(101_273_97) || accruals[1].Amount != money.FromKopecks(1_165_34) {
		t.Fatalf("февраль с капитализацией: %+v", accruals[1])
	}

	// Без капитализации база второго месяца не меняется.
	settings.Capitalization = false
	account.Settings = settings
	accruals, err = calc.Accrue(account, day(2026, time.January, 1), day(2026, time.March, 1))
	if err != nil {
		t.Fatalf("Accrue: %v", err)
	}
	if accruals[1].Base != money.FromKopecks(100_000_00) {
		t.Fatalf("февраль без капитализации: %+v", accruals[1])
	}
}

// TestASL_18_DepositPreviewWithdrawal проверяет разбивку снятия.
func TestASL_18_DepositPreviewWithdrawal(t *testing.T) {
	calc := DepositCalculator{}
	settings := depositSettings()
	settings.OpenedAt = day(2026, time.January, 1)
	account := DepositAccount{
		Balance:         money.FromKopecks(100_000_00),
		AccruedInterest: money.FromKopecks(5_000_00),
		Settings:        settings,
	}

	payment, err := calc.PreviewWithdrawal(account, money.FromKopecks(20_000_00), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("PreviewWithdrawal: %v", err)
	}
	want := DepositPayment{
		Principal: money.FromKopecks(15_000_00),
		Interest:  money.FromKopecks(5_000_00),
		Total:     money.FromKopecks(20_000_00),
	}
	if payment != want {
		t.Fatalf("PreviewWithdrawal = %+v, ожидалось %+v", payment, want)
	}

	// Недостаточная сумма уходит в проценты.
	payment, err = calc.PreviewWithdrawal(account, money.FromKopecks(3_000_00), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("PreviewWithdrawal: %v", err)
	}
	if payment != (DepositPayment{Interest: money.FromKopecks(3_000_00), Total: money.FromKopecks(3_000_00)}) {
		t.Fatalf("недостаточная сумма: %+v", payment)
	}

	// Сумма больше вклада с процентами ограничивается остатком.
	payment, err = calc.PreviewWithdrawal(account, money.FromKopecks(200_000_00), day(2026, time.February, 1))
	if err != nil {
		t.Fatalf("PreviewWithdrawal: %v", err)
	}
	want = DepositPayment{
		Principal: money.FromKopecks(100_000_00),
		Interest:  money.FromKopecks(5_000_00),
		Total:     money.FromKopecks(105_000_00),
	}
	if payment != want {
		t.Fatalf("снятие больше остатка = %+v, ожидалось %+v", payment, want)
	}
}

// TestASL_18_DepositErrors проверяет ошибочные входные данные.
func TestASL_18_DepositErrors(t *testing.T) {
	calc := DepositCalculator{}
	valid := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: depositSettings(),
	}
	from := day(2026, time.January, 1)
	to := day(2026, time.February, 1)

	cases := []struct {
		name    string
		account DepositAccount
		from    time.Time
		to      time.Time
		want    error
	}{
		{"пустой период", valid, from, from, ErrDepositPeriod},
		{"обратный период", valid, to, from, ErrDepositPeriod},
		{"отрицательный остаток", DepositAccount{Balance: -1, Settings: depositSettings()}, from, to, ErrDepositBalance},
		{"ставка", DepositAccount{Balance: 1, Settings: func() model.DepositSettings {
			s := depositSettings()
			s.BaseRatePPM = 1_000_001
			return s
		}()}, from, to, ErrDepositRate},
		{"частота", DepositAccount{Balance: 1, Settings: func() model.DepositSettings {
			s := depositSettings()
			s.AccrualFrequency = "weekly"
			return s
		}()}, from, to, ErrDepositFrequency},
		{"бонус без суммы", DepositAccount{Balance: 1, Settings: func() model.DepositSettings {
			s := depositSettings()
			s.BonusType = model.DepositBonusFixed
			return s
		}()}, from, to, ErrDepositBonus},
		{"бонус без ставки", DepositAccount{Balance: 1, Settings: func() model.DepositSettings {
			s := depositSettings()
			s.BonusType = model.DepositBonusRate
			return s
		}()}, from, to, ErrDepositBonus},
		{"пересечение ступеней", DepositAccount{Balance: 1, Settings: func() model.DepositSettings {
			s := depositSettings()
			to := money.FromKopecks(1000)
			s.Steps = []model.DepositStepRate{
				{From: 0, To: &to, RatePPM: 100_000},
				{From: 500, RatePPM: 110_000},
			}
			return s
		}()}, from, to, ErrDepositSteps},
		{"пустая ступень", DepositAccount{Balance: 1, Settings: func() model.DepositSettings {
			s := depositSettings()
			to := money.FromKopecks(0)
			s.Steps = []model.DepositStepRate{{From: 0, To: &to, RatePPM: 100_000}}
			return s
		}()}, from, to, ErrDepositSteps},
		{"уход в минус", DepositAccount{
			Balance:  money.FromKopecks(100),
			Settings: depositSettings(),
			Movements: []DepositMovement{
				{Date: from, Amount: money.FromKopecks(-200)},
			},
		}, from, to, ErrDepositBalance},
	}
	for _, tc := range cases {
		if _, err := calc.Accrue(tc.account, tc.from, tc.to); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, ожидалось %v", tc.name, err, tc.want)
		}
	}

	// Дата снятия вне срока действия вклада.
	settings := depositSettings()
	settings.OpenedAt = day(2026, time.February, 1)
	settings.ClosedAt = day(2026, time.March, 1)
	account := DepositAccount{Balance: money.FromKopecks(100), Settings: settings}
	if _, err := calc.PreviewWithdrawal(account, money.FromKopecks(10), day(2026, time.January, 1)); !errors.Is(err, ErrWithdrawalDate) {
		t.Fatalf("снятие до открытия: err = %v", err)
	}
	if _, err := calc.PreviewWithdrawal(account, money.FromKopecks(10), day(2026, time.April, 1)); !errors.Is(err, ErrWithdrawalDate) {
		t.Fatalf("снятие после закрытия: err = %v", err)
	}
}

// BenchmarkDepositAccrue измеряет стоимость расчёта вклада за месяц.
func BenchmarkDepositAccrue(b *testing.B) {
	calc := DepositCalculator{}
	settings := depositSettings()
	account := DepositAccount{
		Balance:  money.FromKopecks(100_000_00),
		Settings: settings,
	}
	from := day(2026, time.January, 1)
	to := day(2026, time.February, 1)
	for i := 0; i < b.N; i++ {
		if _, err := calc.Accrue(account, from, to); err != nil {
			b.Fatal(err)
		}
	}
}
