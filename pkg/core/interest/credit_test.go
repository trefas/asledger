package interest

import (
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// creditDate — дата расчёта для тестов.
var creditDate = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// TestASL_17_CreditGrace проверяет беспроцентный период.
func TestASL_17_CreditGrace(t *testing.T) {
	calc := CreditCalculator{}
	graceEnd := creditDate.AddDate(0, 1, 0)
	account := CreditAccount{
		Balance: money.FromKopecks(100_000_00),
		Settings: model.CreditSettings{
			RatePPM:      150_000,
			GraceEnabled: true,
			GraceEndDate: graceEnd,
		},
	}

	if !calc.InGrace(account.Settings, creditDate) {
		t.Fatal("дата внутри grace-периода должна считаться беспроцентной")
	}
	if !calc.InGrace(account.Settings, graceEnd) {
		t.Fatal("последний день grace-периода должен быть беспроцентным")
	}
	if got := calc.AccrueInterest(account, creditDate, 30); got != 0 {
		t.Fatalf("проценты в grace-периоде = %s, ожидался 0", got)
	}

	after := graceEnd.AddDate(0, 0, 1)
	if calc.InGrace(account.Settings, after) {
		t.Fatal("день после grace-периода не должен считаться беспроцентным")
	}
	if got := calc.AccrueInterest(account, after, 30); got == 0 {
		t.Fatal("после grace-периода проценты должны начисляться")
	}

	// Выключенный grace и пустая дата не дают беспроцентного периода.
	account.Settings.GraceEnabled = false
	if calc.InGrace(account.Settings, creditDate) {
		t.Fatal("выключенный grace не должен действовать")
	}
	account.Settings.GraceEnabled = true
	account.Settings.GraceEndDate = time.Time{}
	if calc.InGrace(account.Settings, creditDate) {
		t.Fatal("grace без даты окончания не должен действовать")
	}
}

// TestASL_17_CreditPreviewInterestFirst проверяет порядок «сначала проценты».
func TestASL_17_CreditPreviewInterestFirst(t *testing.T) {
	calc := CreditCalculator{}
	account := CreditAccount{
		Balance:         money.FromKopecks(100_000_00),
		AccruedInterest: money.FromKopecks(5_000_00),
		Settings: model.CreditSettings{
			RepaymentOrder: model.RepaymentInterestFirst,
		},
	}

	payment, err := calc.Preview(account, money.FromKopecks(20_000_00), creditDate)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := CreditPayment{
		Principal: money.FromKopecks(15_000_00),
		Interest:  money.FromKopecks(5_000_00),
		Total:     money.FromKopecks(20_000_00),
	}
	if payment != want {
		t.Fatalf("Preview = %+v, ожидалось %+v", payment, want)
	}
}

// TestASL_17_CreditPreviewPrincipalFirst проверяет порядок «сначала тело».
func TestASL_17_CreditPreviewPrincipalFirst(t *testing.T) {
	calc := CreditCalculator{}
	account := CreditAccount{
		Balance:         money.FromKopecks(100_000_00),
		AccruedInterest: money.FromKopecks(5_000_00),
		Settings: model.CreditSettings{
			RepaymentOrder: model.RepaymentPrincipalFirst,
		},
	}

	payment, err := calc.Preview(account, money.FromKopecks(20_000_00), creditDate)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := CreditPayment{
		Principal: money.FromKopecks(20_000_00),
		Total:     money.FromKopecks(20_000_00),
	}
	if payment != want {
		t.Fatalf("Preview = %+v, ожидалось %+v", payment, want)
	}

	// Суммы хватает на тело и часть процентов.
	payment, err = calc.Preview(account, money.FromKopecks(101_000_00), creditDate)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want = CreditPayment{
		Principal: money.FromKopecks(100_000_00),
		Interest:  money.FromKopecks(1_000_00),
		Total:     money.FromKopecks(101_000_00),
	}
	if payment != want {
		t.Fatalf("Preview с избытком = %+v, ожидалось %+v", payment, want)
	}
}

// TestASL_17_CreditPreviewInsufficient проверяет недостаточную сумму.
func TestASL_17_CreditPreviewInsufficient(t *testing.T) {
	calc := CreditCalculator{}
	account := CreditAccount{
		Balance:         money.FromKopecks(100_000_00),
		AccruedInterest: money.FromKopecks(5_000_00),
		Settings: model.CreditSettings{
			RepaymentOrder: model.RepaymentInterestFirst,
		},
	}

	payment, err := calc.Preview(account, money.FromKopecks(3_000_00), creditDate)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := CreditPayment{
		Interest: money.FromKopecks(3_000_00),
		Total:    money.FromKopecks(3_000_00),
	}
	if payment != want {
		t.Fatalf("Preview = %+v, ожидалось %+v", payment, want)
	}
}

// TestASL_17_CreditPreviewGrace проверяет, что в grace-период вся сумма идёт
// в тело долга.
func TestASL_17_CreditPreviewGrace(t *testing.T) {
	calc := CreditCalculator{}
	account := CreditAccount{
		Balance:         money.FromKopecks(100_000_00),
		AccruedInterest: money.FromKopecks(5_000_00),
		Settings: model.CreditSettings{
			RepaymentOrder: model.RepaymentInterestFirst,
			GraceEnabled:   true,
			GraceEndDate:   creditDate.AddDate(0, 1, 0),
		},
	}

	payment, err := calc.Preview(account, money.FromKopecks(20_000_00), creditDate)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := CreditPayment{
		Principal: money.FromKopecks(20_000_00),
		Total:     money.FromKopecks(20_000_00),
	}
	if payment != want {
		t.Fatalf("Preview в grace = %+v, ожидалось %+v", payment, want)
	}
}

// TestASL_17_CreditPreviewErrors проверяет ошибочные входные данные.
func TestASL_17_CreditPreviewErrors(t *testing.T) {
	calc := CreditCalculator{}
	valid := CreditAccount{Balance: money.FromKopecks(100)}

	cases := []struct {
		name    string
		account CreditAccount
		amount  money.Kop
		want    error
	}{
		{"нулевая сумма", valid, 0, ErrPaymentAmount},
		{"отрицательная сумма", valid, -1, ErrPaymentAmount},
		{"отрицательный долг", CreditAccount{Balance: -1}, 100, ErrCreditBalance},
		{"отрицательные проценты", CreditAccount{AccruedInterest: -1}, 100, ErrAccruedInterest},
	}
	for _, tc := range cases {
		if _, err := calc.Preview(tc.account, tc.amount, creditDate); !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, ожидалось %v", tc.name, err, tc.want)
		}
	}
}

// TestASL_17_CreditPreviewDefaults проверяет пустой порядок погашения.
func TestASL_17_CreditPreviewDefaults(t *testing.T) {
	calc := CreditCalculator{}
	account := CreditAccount{
		Balance:         money.FromKopecks(100),
		AccruedInterest: money.FromKopecks(40),
	}
	payment, err := calc.Preview(account, money.FromKopecks(50), creditDate)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if payment.Interest != money.FromKopecks(40) || payment.Principal != money.FromKopecks(10) {
		t.Fatalf("по умолчанию ожидался порядок «сначала проценты», получено %+v", payment)
	}
}

// BenchmarkCreditPreview измеряет стоимость предпросмотра платежа.
func BenchmarkCreditPreview(b *testing.B) {
	calc := CreditCalculator{}
	account := CreditAccount{
		Balance:         money.FromKopecks(100_000_00),
		AccruedInterest: money.FromKopecks(5_000_00),
		Settings: model.CreditSettings{
			RatePPM:        150_000,
			RepaymentOrder: model.RepaymentInterestFirst,
		},
	}
	amount := money.FromKopecks(20_000_00)
	for i := 0; i < b.N; i++ {
		if _, err := calc.Preview(account, amount, creditDate); err != nil {
			b.Fatal(err)
		}
	}
}
