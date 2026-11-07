package report

import (
	"context"
	"errors"
	"time"

	"github.com/trefas/asledger/pkg/core/interest"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// DefaultHorizonDays — горизонт прогноза предстоящих начислений по умолчанию.
const DefaultHorizonDays = 30

// ProductOptions — параметры отчётов по кредитам и вкладам.
type ProductOptions struct {
	// AccountID ограничивает отчёт одним счётом; 0 — все счета продукта.
	AccountID int64
	// AsOf — дата отчёта; нулевая означает текущую дату.
	AsOf time.Time
	// HorizonDays — горизонт прогноза предстоящих начислений.
	// Нуль означает DefaultHorizonDays.
	HorizonDays int
}

// asOf возвращает дату отчёта без времени.
func (o ProductOptions) asOf() time.Time {
	if o.AsOf.IsZero() {
		return day(time.Now())
	}
	return day(o.AsOf)
}

// horizon возвращает горизонт прогноза в днях.
func (o ProductOptions) horizon() int {
	if o.HorizonDays <= 0 {
		return DefaultHorizonDays
	}
	return o.HorizonDays
}

// UpcomingAccrual — предстоящее начисление процентов или бонуса.
type UpcomingAccrual struct {
	// Date — дата начисления.
	Date time.Time
	// Kind — вид начисления.
	Kind model.InterestAccrualKind
	// Amount — сумма начисления.
	Amount money.Kop
	// Base — база расчёта.
	Base money.Kop
	// RatePPM — применённая ставка.
	RatePPM int64
	// Days — число дней в периоде начисления.
	Days int
}

// CreditSummaryRow — состояние одного кредитного счёта.
type CreditSummaryRow struct {
	AccountID int64
	Path      string
	Name      string
	// RatePPM, InterestType и RepaymentOrder — условия кредита.
	RatePPM        int64
	InterestType   model.CreditInterestType
	RepaymentOrder model.RepaymentOrder
	// GraceEnabled и GraceEndDate — беспроцентный период.
	GraceEnabled bool
	GraceEndDate time.Time
	// InGrace — на дату отчёта действует беспроцентный период.
	InGrace bool
	// Debt — остаток тела долга.
	Debt money.Kop
	// AccruedInterest — начисленные, но ещё не погашенные проценты.
	AccruedInterest money.Kop
	// TotalDebt — долг вместе с процентами.
	TotalDebt money.Kop
	// Upcoming — предстоящие начисления на горизонте прогноза.
	Upcoming []UpcomingAccrual
	// UpcomingInterest — сумма предстоящих начислений.
	UpcomingInterest money.Kop
}

// CreditSummaryReport — отчёт по всем кредитным счетам.
type CreditSummaryReport struct {
	// AsOf — дата отчёта.
	AsOf time.Time
	// HorizonDays — горизонт прогноза предстоящих начислений.
	HorizonDays int
	// Rows — кредитные счета в порядке путей.
	Rows []CreditSummaryRow
	// Debt, AccruedInterest и TotalDebt — итоги по всем кредитам.
	Debt            money.Kop
	AccruedInterest money.Kop
	TotalDebt       money.Kop
	// UpcomingInterest — итог предстоящих начислений.
	UpcomingInterest money.Kop
}

// DepositSummaryRow — состояние одного вклада.
type DepositSummaryRow struct {
	AccountID int64
	Path      string
	Name      string
	// BaseRatePPM, BonusType, AccrualFrequency и Capitalization — условия вклада.
	BaseRatePPM      int64
	BonusType        model.DepositBonusType
	AccrualFrequency model.DepositAccrualFrequency
	Capitalization   bool
	// Balance — остаток тела вклада.
	Balance money.Kop
	// AccruedInterest — начисленные, но ещё не выплаченные проценты и бонусы.
	AccruedInterest money.Kop
	// Total — остаток вместе с накопленными процентами.
	Total money.Kop
	// Upcoming — предстоящие начисления на горизонте прогноза.
	Upcoming []UpcomingAccrual
	// UpcomingInterest, UpcomingBonus и UpcomingTotal — итоги прогноза.
	UpcomingInterest money.Kop
	UpcomingBonus    money.Kop
	UpcomingTotal    money.Kop
}

// DepositSummaryReport — отчёт по всем вкладам.
type DepositSummaryReport struct {
	// AsOf — дата отчёта.
	AsOf time.Time
	// HorizonDays — горизонт прогноза предстоящих начислений.
	HorizonDays int
	// Rows — вклады в порядке путей.
	Rows []DepositSummaryRow
	// Balance, AccruedInterest и Total — итоги по всем вкладам.
	Balance         money.Kop
	AccruedInterest money.Kop
	Total           money.Kop
	// UpcomingInterest, UpcomingBonus и UpcomingTotal — итоги прогноза.
	UpcomingInterest money.Kop
	UpcomingBonus    money.Kop
	UpcomingTotal    money.Kop
}

// CreditForecastOptions — параметры прогноза погашения кредита.
type CreditForecastOptions struct {
	// AccountID — кредитный счёт.
	AccountID int64
	// Amount — сумма планируемого платежа.
	Amount money.Kop
	// Date — дата платежа; нулевая означает текущую дату.
	Date time.Time
	// HorizonDays — горизонт прогноза после платежа.
	HorizonDays int
}

// CreditForecastReport — прогноз изменения долга и процентов после платежа.
type CreditForecastReport struct {
	AccountID int64
	Path      string
	Name      string
	// Date — дата платежа.
	Date time.Time
	// HorizonDays — горизонт прогноза после платежа.
	HorizonDays int
	// Requested — запрошенная сумма платежа.
	Requested money.Kop
	// Principal и Interest — разбивка платежа.
	Principal money.Kop
	Interest  money.Kop
	// Total — фактически распределённая сумма.
	Total money.Kop
	// DebtBefore, AccruedInterestBefore и TotalDebtBefore — состояние до платежа.
	DebtBefore            money.Kop
	AccruedInterestBefore money.Kop
	TotalDebtBefore       money.Kop
	// DebtAfter, AccruedInterestAfter и TotalDebtAfter — состояние после платежа.
	DebtAfter            money.Kop
	AccruedInterestAfter money.Kop
	TotalDebtAfter       money.Kop
	// DebtChange и InterestChange — изменение долга и процентов.
	DebtChange     money.Kop
	InterestChange money.Kop
	// InGrace — на дату платежа действует беспроцентный период.
	InGrace bool
	// Upcoming — предстоящие начисления после платежа.
	Upcoming []UpcomingAccrual
	// UpcomingInterest — сумма предстоящих начислений после платежа.
	UpcomingInterest money.Kop
}

// CreditSummary строит отчёт по кредитным счетам.
//
// Для каждого счёта показываются остаток тела долга, начисленные проценты и
// прогноз предстоящих начислений на горизонте. Счета без настроек кредита в
// отчёт не попадают; если счёт задан явно, отсутствие настроек — ошибка.
func CreditSummary(ctx context.Context, db *store.DB, opts ProductOptions) (*CreditSummaryReport, error) {
	asOf := opts.asOf()
	horizon := opts.horizon()
	rep := &CreditSummaryReport{AsOf: asOf, HorizonDays: horizon}

	accounts, err := db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return nil, err
	}

	if opts.AccountID != 0 {
		account, err := productAccount(ctx, db, opts.AccountID, model.TypeLiability,
			store.CodeCreditAccountInvalid, "счёт кредита")
		if err != nil {
			return nil, err
		}
		if _, err := db.CreditSettings(ctx, account.ID); errors.Is(err, store.ErrNotFound) {
			return nil, productValidation(store.CodeCreditSettingsMissing,
				"у счёта нет настроек кредита: "+account.FullPath)
		} else if err != nil {
			return nil, err
		}
		accounts = []model.Account{*account}
	}

	calc := interest.CreditCalculator{}
	for _, account := range accounts {
		if account.Type != model.TypeLiability {
			continue
		}
		if opts.AccountID != 0 && account.ID != opts.AccountID {
			continue
		}
		settings, err := db.CreditSettings(ctx, account.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		debt, err := db.BalanceAt(ctx, account.ID, asOf.AddDate(0, 0, 1))
		if err != nil {
			return nil, err
		}
		accrued, err := db.AccruedInterestAt(ctx, account.ID, model.InterestAccrualCredit, endOfDay(asOf))
		if err != nil {
			return nil, err
		}
		upcoming := creditUpcoming(*settings, debt, asOf, horizon)
		row := CreditSummaryRow{
			AccountID:        account.ID,
			Path:             account.FullPath,
			Name:             account.Name,
			RatePPM:          settings.RatePPM,
			InterestType:     settings.InterestType,
			RepaymentOrder:   settings.RepaymentOrder,
			GraceEnabled:     settings.GraceEnabled,
			GraceEndDate:     settings.GraceEndDate,
			InGrace:          calc.InGrace(*settings, asOf),
			Debt:             debt,
			AccruedInterest:  accrued,
			TotalDebt:        debt + accrued,
			Upcoming:         upcoming,
			UpcomingInterest: sumUpcoming(upcoming),
		}
		rep.Rows = append(rep.Rows, row)
		rep.Debt += row.Debt
		rep.AccruedInterest += row.AccruedInterest
		rep.TotalDebt += row.TotalDebt
		rep.UpcomingInterest += row.UpcomingInterest
	}
	return rep, nil
}

// DepositSummary строит отчёт по вкладам.
//
// Для каждого вклада показываются остаток, накопленные проценты и прогноз
// предстоящих начислений на горизонте. Счета без настроек вклада в отчёт не
// попадают; если счёт задан явно, отсутствие настроек — ошибка.
func DepositSummary(ctx context.Context, db *store.DB, opts ProductOptions) (*DepositSummaryReport, error) {
	asOf := opts.asOf()
	horizon := opts.horizon()
	rep := &DepositSummaryReport{AsOf: asOf, HorizonDays: horizon}

	accounts, err := db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return nil, err
	}
	if opts.AccountID != 0 {
		account, err := productAccount(ctx, db, opts.AccountID, model.TypeAsset,
			store.CodeDepositAccountInvalid, "счёт вклада")
		if err != nil {
			return nil, err
		}
		if _, err := db.DepositSettings(ctx, account.ID); errors.Is(err, store.ErrNotFound) {
			return nil, productValidation(store.CodeDepositSettingsMissing,
				"у счёта нет настроек вклада: "+account.FullPath)
		} else if err != nil {
			return nil, err
		}
		accounts = []model.Account{*account}
	}

	calc := interest.DepositCalculator{}
	for _, account := range accounts {
		if account.Type != model.TypeAsset {
			continue
		}
		if opts.AccountID != 0 && account.ID != opts.AccountID {
			continue
		}
		settings, err := db.DepositSettings(ctx, account.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		balance, err := db.BalanceAt(ctx, account.ID, asOf.AddDate(0, 0, 1))
		if err != nil {
			return nil, err
		}
		interestAccrued, err := db.AccruedInterestAt(ctx, account.ID, model.InterestAccrualDeposit, endOfDay(asOf))
		if err != nil {
			return nil, err
		}
		bonusAccrued, err := db.AccruedInterestAt(ctx, account.ID, model.InterestAccrualBonus, endOfDay(asOf))
		if err != nil {
			return nil, err
		}
		upcoming, err := depositUpcoming(calc, *settings, balance, asOf, horizon)
		if err != nil {
			return nil, err
		}
		row := DepositSummaryRow{
			AccountID:        account.ID,
			Path:             account.FullPath,
			Name:             account.Name,
			BaseRatePPM:      settings.BaseRatePPM,
			BonusType:        settings.BonusType,
			AccrualFrequency: settings.AccrualFrequency,
			Capitalization:   settings.Capitalization,
			Balance:          balance,
			AccruedInterest:  interestAccrued + bonusAccrued,
			Total:            balance + interestAccrued + bonusAccrued,
			Upcoming:         upcoming,
		}
		for _, item := range upcoming {
			if item.Kind == model.InterestAccrualBonus {
				row.UpcomingBonus += item.Amount
			} else {
				row.UpcomingInterest += item.Amount
			}
		}
		row.UpcomingTotal = row.UpcomingInterest + row.UpcomingBonus
		rep.Rows = append(rep.Rows, row)
		rep.Balance += row.Balance
		rep.AccruedInterest += row.AccruedInterest
		rep.Total += row.Total
		rep.UpcomingInterest += row.UpcomingInterest
		rep.UpcomingBonus += row.UpcomingBonus
		rep.UpcomingTotal += row.UpcomingTotal
	}
	return rep, nil
}

// CreditForecast рассчитывает, как платёж изменит долг и проценты.
func CreditForecast(ctx context.Context, db *store.DB, opts CreditForecastOptions) (*CreditForecastReport, error) {
	if opts.Amount <= 0 {
		return nil, productValidation(store.CodeCreditAmount,
			"сумма прогноза должна быть положительной")
	}
	date := opts.asOfDate()
	horizon := opts.horizon()
	account, err := productAccount(ctx, db, opts.AccountID, model.TypeLiability,
		store.CodeCreditAccountInvalid, "счёт кредита")
	if err != nil {
		return nil, err
	}
	settings, err := db.CreditSettings(ctx, account.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, productValidation(store.CodeCreditSettingsMissing,
			"у счёта нет настроек кредита: "+account.FullPath)
	}
	if err != nil {
		return nil, err
	}
	debt, err := db.BalanceAt(ctx, account.ID, date.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	accrued, err := db.AccruedInterestAt(ctx, account.ID, model.InterestAccrualCredit, endOfDay(date))
	if err != nil {
		return nil, err
	}

	calc := interest.CreditCalculator{}
	payment, err := calc.Preview(interest.CreditAccount{
		Balance:         debt,
		AccruedInterest: accrued,
		Settings:        *settings,
	}, opts.Amount, date)
	if err != nil {
		code := store.CodeCreditAccountInvalid
		if errors.Is(err, interest.ErrPaymentAmount) {
			code = store.CodeCreditAmount
		}
		return nil, productValidation(code, "не удалось рассчитать прогноз: "+err.Error())
	}

	debtAfter := debt - payment.Principal
	accruedAfter := accrued - payment.Interest
	upcoming := creditUpcoming(*settings, debtAfter, date, horizon)
	return &CreditForecastReport{
		AccountID:             account.ID,
		Path:                  account.FullPath,
		Name:                  account.Name,
		Date:                  date,
		HorizonDays:           horizon,
		Requested:             opts.Amount,
		Principal:             payment.Principal,
		Interest:              payment.Interest,
		Total:                 payment.Total,
		DebtBefore:            debt,
		AccruedInterestBefore: accrued,
		TotalDebtBefore:       debt + accrued,
		DebtAfter:             debtAfter,
		AccruedInterestAfter:  accruedAfter,
		TotalDebtAfter:        debtAfter + accruedAfter,
		DebtChange:            debtAfter - debt,
		InterestChange:        accruedAfter - accrued,
		InGrace:               calc.InGrace(*settings, date),
		Upcoming:              upcoming,
		UpcomingInterest:      sumUpcoming(upcoming),
	}, nil
}

// asOfDate возвращает дату прогноза без времени.
func (o CreditForecastOptions) asOfDate() time.Time {
	if o.Date.IsZero() {
		return day(time.Now())
	}
	return day(o.Date)
}

// horizon возвращает горизонт прогноза в днях.
func (o CreditForecastOptions) horizon() int {
	if o.HorizonDays <= 0 {
		return DefaultHorizonDays
	}
	return o.HorizonDays
}

// productAccount читает счёт и проверяет, что он листовой и нужного типа.
func productAccount(ctx context.Context, db *store.DB, accountID int64,
	typ model.AccountType, code, what string) (*model.Account, error) {
	account, err := db.Account(ctx, accountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, productValidation(code, what+" не найден")
	}
	if err != nil {
		return nil, err
	}
	if account.IsArchived || !account.IsLeaf || account.Type != typ {
		return nil, productValidation(code,
			what+" должен быть листовым счётом типа «"+string(typ)+"»: "+account.FullPath)
	}
	return account, nil
}

// creditUpcoming прогнозирует начисления по кредиту на горизонте.
//
// В беспроцентный период начисления начинаются со дня после его окончания.
func creditUpcoming(settings model.CreditSettings, debt money.Kop, asOf time.Time, horizon int) []UpcomingAccrual {
	if debt <= 0 {
		return nil
	}
	start := asOf
	calc := interest.CreditCalculator{}
	if calc.InGrace(settings, asOf) {
		if settings.GraceEndDate.IsZero() {
			return nil
		}
		start = day(settings.GraceEndDate).AddDate(0, 0, 1)
	}
	end := asOf.AddDate(0, 0, horizon)
	if !start.Before(end) {
		return nil
	}
	days := int(end.Sub(start).Hours() / 24)
	amount := interest.Accrue(debt, interest.Rate(settings.RatePPM), days, interest.DaysInYear)
	if amount <= 0 {
		return nil
	}
	return []UpcomingAccrual{{
		Date:    end,
		Kind:    model.InterestAccrualCredit,
		Amount:  amount,
		Base:    debt,
		RatePPM: settings.RatePPM,
		Days:    days,
	}}
}

// depositUpcoming прогнозирует начисления по вкладу на горизонте.
func depositUpcoming(calc interest.DepositCalculator, settings model.DepositSettings,
	balance money.Kop, asOf time.Time, horizon int) ([]UpcomingAccrual, error) {
	if balance <= 0 {
		return nil, nil
	}
	accruals, err := calc.Accrue(interest.DepositAccount{
		Balance:  balance,
		Settings: settings,
	}, asOf, asOf.AddDate(0, 0, horizon))
	if err != nil {
		return nil, err
	}
	upcoming := make([]UpcomingAccrual, 0, len(accruals))
	for _, accrual := range accruals {
		upcoming = append(upcoming, UpcomingAccrual{
			Date:    accrual.Date,
			Kind:    accrual.Kind,
			Amount:  accrual.Amount,
			Base:    accrual.Base,
			RatePPM: int64(accrual.Rate),
			Days:    accrual.Days,
		})
	}
	return upcoming, nil
}

// sumUpcoming складывает суммы предстоящих начислений.
func sumUpcoming(items []UpcomingAccrual) money.Kop {
	var total money.Kop
	for _, item := range items {
		total += item.Amount
	}
	return total
}

// productValidation создаёт ошибку валидации отчёта по продукту.
func productValidation(code, message string) *store.ValidationError {
	return &store.ValidationError{Code: code, Message: message}
}

// endOfDay возвращает последнюю секунду календарного дня.
func endOfDay(t time.Time) time.Time {
	return day(t).AddDate(0, 0, 1).Add(-time.Second)
}

// day отбрасывает время и приводит дату к UTC.
func day(t time.Time) time.Time {
	year, month, date := t.Date()
	return time.Date(year, month, date, 0, 0, 0, 0, time.UTC)
}
