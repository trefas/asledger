package interest

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// Ошибки расчёта вклада.
var (
	// ErrDepositPeriod — начало периода не раньше его конца.
	ErrDepositPeriod = errors.New("interest: начало периода должно быть раньше конца")
	// ErrDepositBalance — остаток вклада не может быть отрицательным.
	ErrDepositBalance = errors.New("interest: остаток вклада не может быть отрицательным")
	// ErrDepositRate — ставка вклада вне диапазона 0…100 %.
	ErrDepositRate = errors.New("interest: недопустимая ставка вклада")
	// ErrDepositBonus — недопустимые параметры бонуса.
	ErrDepositBonus = errors.New("interest: недопустимые параметры бонуса")
	// ErrDepositFrequency — недопустимая частота начисления.
	ErrDepositFrequency = errors.New("interest: недопустимая частота начисления")
	// ErrDepositSteps — пересекающиеся или некорректные ступени.
	ErrDepositSteps = errors.New("interest: некорректные ступени вклада")
	// ErrWithdrawalDate — дата снятия вне срока действия вклада.
	ErrWithdrawalDate = errors.New("interest: дата снятия вне срока действия вклада")
)

// DepositMovement — изменение остатка вклада.
type DepositMovement struct {
	// Date — дата изменения; время отбрасывается.
	Date time.Time
	// Amount — изменение остатка: пополнение положительное, снятие отрицательное.
	Amount money.Kop
}

// DepositAccount — снимок вклада для расчёта.
type DepositAccount struct {
	// Balance — остаток на начало периода расчёта.
	Balance money.Kop
	// AccruedInterest — начисленные, но ещё не выплаченные проценты.
	AccruedInterest money.Kop
	// Settings — условия вклада.
	Settings model.DepositSettings
	// Movements — движения средств внутри периода расчёта.
	Movements []DepositMovement
}

// Accrual — одно начисление процентов или бонуса.
type Accrual struct {
	// Date — дата проводки начисления (конец периода).
	Date time.Time
	// PeriodFrom и PeriodTo — период, за который начислено.
	PeriodFrom time.Time
	PeriodTo   time.Time
	// Kind — вид начисления.
	Kind model.InterestAccrualKind
	// Amount — сумма начисления.
	Amount money.Kop
	// Base — база расчёта: остаток или минимальный остаток.
	Base money.Kop
	// Rate — применённая ставка в ppm.
	Rate Rate
	// Days — число дней в периоде начисления.
	Days int
}

// DepositPayment — разбивка снятия со вклада.
type DepositPayment struct {
	// Principal — сумма с тела вклада.
	Principal money.Kop
	// Interest — сумма с начисленных процентов.
	Interest money.Kop
	// Total — фактически распределённая сумма; не больше исходной.
	Total money.Kop
}

// DepositCalculator применяет стратегии вклада: ежедневное начисление, расчёт
// по минимальному остатку, ступенчатые ставки, бонусы и капитализацию.
type DepositCalculator struct {
	// YearDays — база годового расчёта; 0 означает DaysInYear.
	YearDays int
}

// Accrue рассчитывает начисления за период [from, to).
//
// Для частоты daily возвращается по одному начислению на день, для monthly —
// по одному на календарный месяц (или его часть). Проценты с нулевой суммой
// не включаются. Бонус начисляется отдельной строкой за каждый календарный
// месяц периода. При включённой капитализации проценты и бонусы добавляются
// к остатку и участвуют в расчёте следующих периодов.
func (c DepositCalculator) Accrue(account DepositAccount, from, to time.Time) ([]Accrual, error) {
	from, to = dateOnly(from), dateOnly(to)
	if !from.Before(to) {
		return nil, ErrDepositPeriod
	}
	if account.Balance < 0 {
		return nil, ErrDepositBalance
	}
	if err := validateDeposit(account.Settings); err != nil {
		return nil, err
	}

	movements, err := normalizeMovements(account.Movements)
	if err != nil {
		return nil, err
	}

	if account.Settings.AccrualFrequency == model.DepositAccrualDaily {
		return c.accrueDaily(account.Settings, account.Balance, movements, from, to)
	}
	return c.accrueMonthly(account.Settings, account.Balance, movements, from, to)
}

// PreviewWithdrawal распределяет сумму снятия между телом вклада и
// начисленными процентами.
//
// Сначала списываются начисленные проценты, затем тело вклада. Если суммы не
// хватает, распределяется сколько возможно; Total будет меньше amount.
// Капитализированные проценты уже входят в Balance, поэтому для них
// AccruedInterest должен быть равен нулю.
func (c DepositCalculator) PreviewWithdrawal(account DepositAccount, amount money.Kop, date time.Time) (DepositPayment, error) {
	if amount <= 0 {
		return DepositPayment{}, ErrPaymentAmount
	}
	if account.Balance < 0 {
		return DepositPayment{}, ErrDepositBalance
	}
	if account.AccruedInterest < 0 {
		return DepositPayment{}, ErrAccruedInterest
	}

	date = dateOnly(date)
	if opened := dateOnly(account.Settings.OpenedAt); !account.Settings.OpenedAt.IsZero() && date.Before(opened) {
		return DepositPayment{}, fmt.Errorf("%w: %s", ErrWithdrawalDate, date.Format(time.DateOnly))
	}
	if closed := dateOnly(account.Settings.ClosedAt); !account.Settings.ClosedAt.IsZero() && date.After(closed) {
		return DepositPayment{}, fmt.Errorf("%w: %s", ErrWithdrawalDate, date.Format(time.DateOnly))
	}

	payment := DepositPayment{
		Interest: minKop(amount, account.AccruedInterest),
	}
	payment.Principal = minKop(amount-payment.Interest, account.Balance)
	payment.Total = payment.Principal + payment.Interest
	return payment, nil
}

// accrueDaily начисляет проценты на фактический остаток каждый день.
func (c DepositCalculator) accrueDaily(settings model.DepositSettings, balance money.Kop,
	movements []DepositMovement, from, to time.Time) ([]Accrual, error) {
	var accruals []Accrual
	monthStart := from
	monthMin := balance
	monthDays := 0
	yearDays := c.yearDays()

	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		balance += movementsOn(movements, day)
		if balance < 0 {
			return nil, ErrDepositBalance
		}
		if balance < monthMin {
			monthMin = balance
		}

		rate := rateFor(balance, settings)
		amount := Accrue(balance, rate, 1, yearDays)
		if amount > 0 {
			next := day.AddDate(0, 0, 1)
			accruals = append(accruals, Accrual{
				Date:       next,
				PeriodFrom: day,
				PeriodTo:   next,
				Kind:       model.InterestAccrualDeposit,
				Amount:     amount,
				Base:       balance,
				Rate:       rate,
				Days:       1,
			})
			if settings.Capitalization {
				balance += amount
			}
		}
		monthDays++

		next := day.AddDate(0, 0, 1)
		if next.Month() == day.Month() && next.Before(to) {
			continue
		}
		if bonus := c.bonus(settings, monthMin, monthDays); bonus != nil {
			bonus.Date = next
			bonus.PeriodFrom = monthStart
			bonus.PeriodTo = next
			accruals = append(accruals, *bonus)
			if settings.Capitalization {
				balance += bonus.Amount
			}
		}
		monthStart = next
		monthMin = balance
		monthDays = 0
	}
	return accruals, nil
}

// accrueMonthly начисляет проценты на минимальный остаток за календарный месяц.
func (c DepositCalculator) accrueMonthly(settings model.DepositSettings, balance money.Kop,
	movements []DepositMovement, from, to time.Time) ([]Accrual, error) {
	var accruals []Accrual
	yearDays := c.yearDays()

	for start := from; start.Before(to); {
		monthEnd := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		end := monthEnd
		if end.After(to) {
			end = to
		}
		days := daysBetween(start, end)

		minBalance := balance
		for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
			balance += movementsOn(movements, day)
			if balance < 0 {
				return nil, ErrDepositBalance
			}
			if balance < minBalance {
				minBalance = balance
			}
		}

		rate := rateFor(minBalance, settings)
		amount := Accrue(minBalance, rate, days, yearDays)
		if amount > 0 {
			accruals = append(accruals, Accrual{
				Date:       end,
				PeriodFrom: start,
				PeriodTo:   end,
				Kind:       model.InterestAccrualDeposit,
				Amount:     amount,
				Base:       minBalance,
				Rate:       rate,
				Days:       days,
			})
			if settings.Capitalization {
				balance += amount
			}
		}
		if bonus := c.bonus(settings, minBalance, days); bonus != nil {
			bonus.Date = end
			bonus.PeriodFrom = start
			bonus.PeriodTo = end
			accruals = append(accruals, *bonus)
			if settings.Capitalization {
				balance += bonus.Amount
			}
		}
		start = end
	}
	return accruals, nil
}

// bonus создаёт начисление бонуса за период, если бонус включён.
func (c DepositCalculator) bonus(settings model.DepositSettings, base money.Kop, days int) *Accrual {
	switch settings.BonusType {
	case model.DepositBonusFixed:
		if settings.BonusAmount <= 0 {
			return nil
		}
		return &Accrual{
			Kind:   model.InterestAccrualBonus,
			Amount: settings.BonusAmount,
			Base:   base,
			Days:   days,
		}
	case model.DepositBonusRate:
		rate := Rate(settings.BonusRatePPM)
		amount := Accrue(base, rate, days, c.yearDays())
		if amount <= 0 {
			return nil
		}
		return &Accrual{
			Kind:   model.InterestAccrualBonus,
			Amount: amount,
			Base:   base,
			Rate:   rate,
			Days:   days,
		}
	default:
		return nil
	}
}

// rateFor выбирает ставку по остатку: ступень, в диапазон которой попадает
// остаток, иначе базовую ставку.
func rateFor(balance money.Kop, settings model.DepositSettings) Rate {
	for _, step := range settings.Steps {
		if balance < step.From {
			continue
		}
		if step.To != nil && balance >= *step.To {
			continue
		}
		return Rate(step.RatePPM)
	}
	return Rate(settings.BaseRatePPM)
}

// validateDeposit проверяет условия вклада перед расчётом.
func validateDeposit(settings model.DepositSettings) error {
	if !Rate(settings.BaseRatePPM).Valid() {
		return fmt.Errorf("%w: %d ppm", ErrDepositRate, settings.BaseRatePPM)
	}
	if settings.MinBalance < 0 {
		return fmt.Errorf("%w: минимальный остаток %s", ErrDepositBalance, settings.MinBalance)
	}
	if !settings.BonusType.Valid() {
		return fmt.Errorf("%w: вид «%s»", ErrDepositBonus, settings.BonusType)
	}
	if !Rate(settings.BonusRatePPM).Valid() {
		return fmt.Errorf("%w: ставка %d ppm", ErrDepositBonus, settings.BonusRatePPM)
	}
	if settings.BonusAmount < 0 {
		return fmt.Errorf("%w: сумма %s", ErrDepositBonus, settings.BonusAmount)
	}
	if settings.BonusType == model.DepositBonusFixed && settings.BonusAmount == 0 {
		return fmt.Errorf("%w: для фиксированного бонуса нужна сумма", ErrDepositBonus)
	}
	if settings.BonusType == model.DepositBonusRate && settings.BonusRatePPM == 0 {
		return fmt.Errorf("%w: для процентного бонуса нужна ставка", ErrDepositBonus)
	}
	if !settings.AccrualFrequency.Valid() {
		return fmt.Errorf("%w: «%s»", ErrDepositFrequency, settings.AccrualFrequency)
	}
	return validateSteps(settings.Steps)
}

// validateSteps сортирует копию ступеней и проверяет границы и пересечения.
func validateSteps(steps []model.DepositStepRate) error {
	sorted := append([]model.DepositStepRate(nil), steps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From < sorted[j].From })
	for i, step := range sorted {
		if step.From < 0 {
			return fmt.Errorf("%w: нижняя граница %s", ErrDepositSteps, step.From)
		}
		if !Rate(step.RatePPM).Valid() {
			return fmt.Errorf("%w: ставка %d ppm", ErrDepositSteps, step.RatePPM)
		}
		if step.To != nil && *step.To <= step.From {
			return fmt.Errorf("%w: верхняя граница не больше нижней", ErrDepositSteps)
		}
		if i == 0 {
			continue
		}
		previous := sorted[i-1]
		if previous.To == nil || step.From < *previous.To {
			return fmt.Errorf("%w: диапазоны пересекаются", ErrDepositSteps)
		}
	}
	return nil
}

// normalizeMovements приводит движения к началу дня и сортирует их.
func normalizeMovements(movements []DepositMovement) ([]DepositMovement, error) {
	normalized := make([]DepositMovement, 0, len(movements))
	for _, movement := range movements {
		if movement.Date.IsZero() {
			return nil, fmt.Errorf("%w: движение без даты", ErrDepositPeriod)
		}
		normalized = append(normalized, DepositMovement{
			Date:   dateOnly(movement.Date),
			Amount: movement.Amount,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Date.Before(normalized[j].Date)
	})
	return normalized, nil
}

// movementsOn суммирует движения за указанный день.
func movementsOn(movements []DepositMovement, day time.Time) money.Kop {
	var total money.Kop
	for _, movement := range movements {
		if movement.Date.Equal(day) {
			total += movement.Amount
		}
	}
	return total
}

// dateOnly отбрасывает время и приводит дату к UTC.
func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// daysBetween возвращает число суток между датами.
func daysBetween(from, to time.Time) int {
	return int(to.Sub(from) / (24 * time.Hour))
}

// yearDays возвращает базу годового расчёта.
func (c DepositCalculator) yearDays() int {
	if c.YearDays > 0 {
		return c.YearDays
	}
	return DaysInYear
}
