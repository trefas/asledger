package interest

import (
	"errors"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// Ошибки расчёта кредита.
var (
	// ErrPaymentAmount — сумма платежа должна быть положительной.
	ErrPaymentAmount = errors.New("interest: сумма платежа должна быть положительной")
	// ErrCreditBalance — остаток долга не может быть отрицательным.
	ErrCreditBalance = errors.New("interest: остаток долга не может быть отрицательным")
	// ErrAccruedInterest — начисленные проценты не могут быть отрицательными.
	ErrAccruedInterest = errors.New("interest: начисленные проценты не могут быть отрицательными")
)

// CreditAccount — снимок кредитного счёта на дату расчёта.
type CreditAccount struct {
	// Balance — остаток тела долга; положительное число.
	Balance money.Kop
	// AccruedInterest — начисленные, но ещё не погашенные проценты.
	AccruedInterest money.Kop
	// Settings — правила кредита.
	Settings model.CreditSettings
}

// CreditPayment — разбивка платежа по кредиту.
type CreditPayment struct {
	// Principal — сумма в погашение тела долга.
	Principal money.Kop
	// Interest — сумма в погашение процентов.
	Interest money.Kop
	// Total — фактически распределённая сумма; не больше исходной.
	Total money.Kop
}

// CreditCalculator применяет стратегии кредита: беспроцентный период и
// порядок погашения.
type CreditCalculator struct{}

// InGrace сообщает, действует ли беспроцентный период на дату date.
//
// Период считается действующим включительно по GraceEndDate. Если grace
// выключен или дата окончания не задана, период не действует.
func (CreditCalculator) InGrace(settings model.CreditSettings, date time.Time) bool {
	if !settings.GraceEnabled || settings.GraceEndDate.IsZero() {
		return false
	}
	return !date.After(settings.GraceEndDate)
}

// AccrueInterest считает проценты по кредиту за days дней на остаток долга.
//
// В беспроцентный период возвращается 0.
func (c CreditCalculator) AccrueInterest(account CreditAccount, date time.Time, days int) money.Kop {
	if c.InGrace(account.Settings, date) {
		return 0
	}
	return Accrue(account.Balance, Rate(account.Settings.RatePPM), days, DaysInYear)
}

// Preview распределяет сумму платежа между телом долга и процентами.
//
// В беспроцентный период проценты не погашаются: вся сумма идёт в тело
// долга. При order == principal_first сначала гасится тело, затем проценты;
// в остальных случаях (включая пустой order) — сначала проценты. Если суммы
// не хватает, распределяется сколько возможно; Total будет меньше amount.
func (c CreditCalculator) Preview(account CreditAccount, amount money.Kop, date time.Time) (CreditPayment, error) {
	if amount <= 0 {
		return CreditPayment{}, ErrPaymentAmount
	}
	if account.Balance < 0 {
		return CreditPayment{}, ErrCreditBalance
	}
	if account.AccruedInterest < 0 {
		return CreditPayment{}, ErrAccruedInterest
	}

	interestDue := account.AccruedInterest
	if c.InGrace(account.Settings, date) {
		interestDue = 0
	}

	var payment CreditPayment
	if account.Settings.RepaymentOrder == model.RepaymentPrincipalFirst {
		payment.Principal = minKop(amount, account.Balance)
		payment.Interest = minKop(amount-payment.Principal, interestDue)
	} else {
		payment.Interest = minKop(amount, interestDue)
		payment.Principal = minKop(amount-payment.Interest, account.Balance)
	}
	payment.Total = payment.Principal + payment.Interest
	return payment, nil
}

// minKop возвращает меньшее из двух значений.
func minKop(a, b money.Kop) money.Kop {
	if a < b {
		return a
	}
	return b
}
