package accrual_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/accrual"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/store/storetest"
)

// accrualFixture — книга с вкладом, кредитом и статьями процентов.
type accrualFixture struct {
	t        *testing.T
	db       *store.DB
	userID   int64
	deposit  *model.Account
	wallet   *model.Account
	income   *model.Account
	credit   *model.Account
	expense  *model.Account
	svc      *accrual.Service
	today    time.Time
	tomorrow time.Time
}

// newAccrualFixture создаёт книгу и остатки на вчера.
func newAccrualFixture(t *testing.T) *accrualFixture {
	t.Helper()
	ctx := context.Background()
	db := storetest.OpenTemp(t)

	user, err := db.CreateUser(ctx, "Андрей", "секрет-андрея")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	assets := rootAccount(t, db, "Активы", model.TypeAsset)
	deposit := createAccount(t, db, "Вклад", model.TypeAsset, &assets.ID)
	wallet := createAccount(t, db, "Наличные", model.TypeAsset, &assets.ID)
	incomeRoot := rootAccount(t, db, "Доходы", model.TypeIncome)
	income := createAccount(t, db, "Проценты по вкладам", model.TypeIncome, &incomeRoot.ID)
	liabilities := rootAccount(t, db, "Обязательства", model.TypeLiability)
	credit := createAccount(t, db, "Кредит", model.TypeLiability, &liabilities.ID)
	expenses := rootAccount(t, db, "Расходы", model.TypeExpense)
	expense := createAccount(t, db, "Проценты по кредитам", model.TypeExpense, &expenses.ID)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath(Капитал:Начальные): %v", err)
	}

	now := time.Now().UTC()
	year, month, day := now.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	tomorrow := today.AddDate(0, 0, 1)

	if _, err := db.SetDepositSettings(ctx, deposit.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		OpenedAt:         yesterday,
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	if _, err := db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "deposit-initial",
		OpType:  model.OpInitial,
		Date:    yesterday,
		ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: deposit.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: capital.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(вклад): %v", err)
	}
	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "credit-initial",
		OpType:  model.OpInitial,
		Date:    yesterday,
		ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: wallet.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: credit.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(кредит): %v", err)
	}

	return &accrualFixture{
		t: t, db: db, userID: user.ID, deposit: deposit, wallet: wallet,
		income: income, credit: credit, expense: expense,
		svc: accrual.New(db), today: today, tomorrow: tomorrow,
	}
}

// rootAccount возвращает корневой счёт, создавая его при необходимости.
func rootAccount(t *testing.T, db *store.DB, name string, typ model.AccountType) *model.Account {
	t.Helper()
	ctx := context.Background()
	account, err := db.AccountByPath(ctx, name)
	if err == nil {
		return account
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("AccountByPath(%s): %v", name, err)
	}
	return createAccount(t, db, name, typ, nil)
}

// createAccount создаёт счёт и завершает тест при ошибке.
func createAccount(t *testing.T, db *store.DB, name string, typ model.AccountType, parent *int64) *model.Account {
	t.Helper()
	acc, err := db.CreateAccount(context.Background(), store.AccountInput{
		Name: name, Type: typ, ParentID: parent,
	})
	if err != nil {
		t.Fatalf("CreateAccount(%q): %v", name, err)
	}
	return acc
}

// depositInput собирает периодическое начисление по вкладу.
func (f *accrualFixture) depositInput() accrual.Input {
	return accrual.Input{
		AccountID:       f.deposit.ID,
		PeriodFrom:      f.today,
		PeriodTo:        f.tomorrow,
		IncomeAccountID: f.income.ID,
		ActorID:         f.userID,
		Source:          "test",
	}
}

// creditInput собирает периодическое начисление по кредиту.
func (f *accrualFixture) creditInput() accrual.Input {
	return accrual.Input{
		AccountID:        f.credit.ID,
		PeriodFrom:       f.today,
		PeriodTo:         f.tomorrow,
		ExpenseAccountID: f.expense.ID,
		ActorID:          f.userID,
		Source:           "test",
	}
}

// TestASL_18_AccrualCalculateDeposit проверяет расчёт процентов по вкладу.
func TestASL_18_AccrualCalculateDeposit(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	preview, err := f.svc.Calculate(ctx, f.depositInput())
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if len(preview.Accruals) != 1 {
		t.Fatalf("начислений = %d, ожидалось 1", len(preview.Accruals))
	}
	item := preview.Accruals[0]
	if item.Kind != model.InterestAccrualDeposit ||
		item.Amount != money.FromKopecks(41_10) ||
		item.Base != money.FromKopecks(100_000_00) ||
		item.RatePPM != 150_000 ||
		item.Status != model.InterestAccrued ||
		item.Existing {
		t.Fatalf("начисление: %+v", item)
	}
	if preview.Total != item.Amount || preview.Capitalization {
		t.Fatalf("итог: %+v", preview)
	}
	if _, err := f.db.InterestAccrualByUUID(ctx, item.UUID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Calculate записал начисление: %v", err)
	}
}

// TestASL_18_AccrualApplyDeposit проверяет начисление без капитализации и повтор.
func TestASL_18_AccrualApplyDeposit(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	posted, err := f.svc.Apply(ctx, f.depositInput())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(posted.Accruals) != 1 || posted.Applied != money.FromKopecks(41_10) {
		t.Fatalf("результат: %+v", posted)
	}
	item := posted.Accruals[0]
	if item.Existing || item.Transaction != nil {
		t.Fatalf("проценты без капитализации не должны иметь проводку: %+v", item)
	}
	if item.Accrual.Status != model.InterestAccrued {
		t.Fatalf("статус: %q", item.Accrual.Status)
	}
	accrued, err := f.db.AccruedInterest(ctx, f.deposit.ID, model.InterestAccrualDeposit)
	if err != nil {
		t.Fatalf("AccruedInterest: %v", err)
	}
	if accrued != money.FromKopecks(41_10) {
		t.Fatalf("начислено = %s", accrued)
	}

	// Повтор за тот же период не создаёт дубль.
	replay, err := f.svc.Apply(ctx, f.depositInput())
	if err != nil {
		t.Fatalf("повтор Apply: %v", err)
	}
	if replay.Applied != 0 || len(replay.Accruals) != 1 || !replay.Accruals[0].Existing {
		t.Fatalf("повтор: %+v", replay)
	}
	all, err := f.db.InterestAccruals(ctx, f.deposit.ID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("начислений в книге = %d, ожидалось 1", len(all))
	}
}

// TestASL_18_AccrualCapitalization проверяет капитализацию процентов.
func TestASL_18_AccrualCapitalization(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	if _, err := f.db.SetDepositSettings(ctx, f.deposit.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		Capitalization:   true,
		OpenedAt:         f.today.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}

	posted, err := f.svc.Apply(ctx, f.depositInput())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(posted.Accruals) != 1 {
		t.Fatalf("начислений = %d", len(posted.Accruals))
	}
	item := posted.Accruals[0]
	if item.Accrual.Status != model.InterestCapitalized || item.Transaction == nil {
		t.Fatalf("капитализация: %+v", item)
	}
	if item.Transaction.OpType != model.OpInterestAccrual || len(item.Transaction.Lines) != 2 {
		t.Fatalf("проводка: %+v", item.Transaction)
	}
	var debit, credit money.Kop
	for _, line := range item.Transaction.Lines {
		if line.Side == model.SideDebit {
			debit += line.Amount
		} else {
			credit += line.Amount
		}
	}
	if debit != credit || debit != money.FromKopecks(41_10) {
		t.Fatalf("ΣDEBIT = %s, ΣCREDIT = %s", debit, credit)
	}
	balance, err := f.db.Balance(ctx, f.deposit.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != money.FromKopecks(100_041_10) {
		t.Fatalf("остаток вклада после капитализации = %s", balance)
	}

	replay, err := f.svc.Apply(ctx, f.depositInput())
	if err != nil {
		t.Fatalf("повтор Apply: %v", err)
	}
	if replay.Applied != 0 || !replay.Accruals[0].Existing {
		t.Fatalf("повтор: %+v", replay)
	}
}

// TestASL_18_AccrualCredit проверяет начисление процентов по кредиту.
func TestASL_18_AccrualCredit(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	posted, err := f.svc.Apply(ctx, f.creditInput())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	item := posted.Accruals[0]
	if item.Accrual.Kind != model.InterestAccrualCredit ||
		item.Accrual.Status != model.InterestAccrued ||
		item.Transaction == nil {
		t.Fatalf("начисление по кредиту: %+v", item)
	}
	if item.Transaction.OpType != model.OpInterestAccrual || len(item.Transaction.Lines) != 2 {
		t.Fatalf("проводка: %+v", item.Transaction)
	}
	var expenseDebit, creditCredit money.Kop
	for _, line := range item.Transaction.Lines {
		if line.AccountID == f.expense.ID && line.Side == model.SideDebit {
			expenseDebit += line.Amount
		}
		if line.AccountID == f.credit.ID && line.Side == model.SideCredit {
			creditCredit += line.Amount
		}
	}
	if expenseDebit != creditCredit || expenseDebit != money.FromKopecks(41_10) {
		t.Fatalf("строки процентов: расход %s, пассив %s", expenseDebit, creditCredit)
	}
	debt, err := f.db.Balance(ctx, f.credit.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if debt != money.FromKopecks(100_041_10) {
		t.Fatalf("долг после начисления = %s", debt)
	}
	accrued, err := f.db.AccruedInterest(ctx, f.credit.ID, model.InterestAccrualCredit)
	if err != nil {
		t.Fatalf("AccruedInterest: %v", err)
	}
	if accrued != money.FromKopecks(41_10) {
		t.Fatalf("начислено процентов = %s", accrued)
	}
}

// TestASL_18_AccrualManual проверяет ручное начисление и идемпотентность по uuid.
func TestASL_18_AccrualManual(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	input := accrual.Input{
		UUID:            "manual-accrual",
		AccountID:       f.deposit.ID,
		Date:            f.today,
		Amount:          money.FromKopecks(1_000_00),
		IncomeAccountID: f.income.ID,
		ActorID:         f.userID,
		Source:          "test",
	}
	preview, err := f.svc.Calculate(ctx, input)
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if len(preview.Accruals) != 1 || preview.Accruals[0].Amount != input.Amount ||
		preview.Accruals[0].Kind != model.InterestAccrualDeposit {
		t.Fatalf("ручное начисление: %+v", preview)
	}

	posted, err := f.svc.Apply(ctx, input)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if posted.Applied != input.Amount || posted.Accruals[0].Transaction != nil {
		t.Fatalf("результат: %+v", posted)
	}
	replay, err := f.svc.Apply(ctx, input)
	if err != nil {
		t.Fatalf("повтор Apply: %v", err)
	}
	if replay.Applied != 0 || !replay.Accruals[0].Existing {
		t.Fatalf("повтор: %+v", replay)
	}
}

// TestASL_18_AccrualBonus проверяет бонус отдельной строкой.
func TestASL_18_AccrualBonus(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	if _, err := f.db.SetDepositSettings(ctx, f.deposit.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		BonusType:        model.DepositBonusFixed,
		BonusAmount:      money.FromKopecks(500_00),
		OpenedAt:         f.today.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}

	preview, err := f.svc.Calculate(ctx, f.depositInput())
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}
	if len(preview.Accruals) != 2 {
		t.Fatalf("начислений = %d, ожидалось 2", len(preview.Accruals))
	}
	if preview.Accruals[0].Kind != model.InterestAccrualDeposit ||
		preview.Accruals[1].Kind != model.InterestAccrualBonus ||
		preview.Accruals[1].Amount != money.FromKopecks(500_00) {
		t.Fatalf("бонус: %+v", preview.Accruals)
	}
	if preview.Total != money.FromKopecks(541_10) {
		t.Fatalf("итог с бонусом = %s", preview.Total)
	}
}

// TestASL_18_AccrualErrors проверяет коды ошибок.
func TestASL_18_AccrualErrors(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	other := createAccount(t, f.db, "Без настроек", model.TypeAsset, nil)

	cases := []struct {
		name  string
		input accrual.Input
		code  string
	}{
		{"account", accrual.Input{AccountID: 999999, PeriodFrom: f.today,
			PeriodTo: f.tomorrow}, store.CodeInterestAccountInvalid},
		{"settings", accrual.Input{AccountID: other.ID, PeriodFrom: f.today,
			PeriodTo: f.tomorrow}, store.CodeInterestSettingsMissing},
		{"kind", accrual.Input{AccountID: f.credit.ID, Kind: model.InterestAccrualDeposit,
			PeriodFrom: f.today, PeriodTo: f.tomorrow}, store.CodeInterestKind},
		{"period", accrual.Input{AccountID: f.deposit.ID, PeriodFrom: f.today,
			PeriodTo: f.today}, store.CodeInterestPeriod},
		{"manual-date", accrual.Input{AccountID: f.deposit.ID,
			Amount: money.FromKopecks(100_00)}, store.CodeInterestDate},
		{"manual-amount", accrual.Input{AccountID: f.deposit.ID, Date: f.today},
			store.CodeInterestAmount},
		{"expense-required", accrual.Input{AccountID: f.credit.ID,
			PeriodFrom: f.today, PeriodTo: f.tomorrow}, store.CodeInterestExpenseAccount},
		{"income-required", func() accrual.Input {
			if _, err := f.db.SetDepositSettings(ctx, f.deposit.ID, store.DepositSettingsInput{
				BaseRatePPM:      150_000,
				AccrualFrequency: model.DepositAccrualMonthly,
				Capitalization:   true,
				OpenedAt:         f.today.AddDate(0, 0, -1),
			}); err != nil {
				t.Fatalf("SetDepositSettings: %v", err)
			}
			in := f.depositInput()
			in.IncomeAccountID = 0
			return in
		}(), store.CodeInterestIncomeAccount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.Calculate(ctx, tc.input)
			var validation *store.ValidationError
			if !errors.As(err, &validation) || validation.Code != tc.code {
				t.Fatalf("err = %v, ожидалась ValidationError с кодом %s", err, tc.code)
			}
		})
	}
}

// TestASL_18_AccrualNothing проверяет пустой период.
func TestASL_18_AccrualNothing(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	// Нулевой остаток: начислять нечего.
	empty := createAccount(t, f.db, "Пустой вклад", model.TypeAsset, nil)
	if _, err := f.db.SetDepositSettings(ctx, empty.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	in := f.depositInput()
	in.AccountID = empty.ID
	_, err := f.svc.Calculate(ctx, in)
	var validation *store.ValidationError
	if !errors.As(err, &validation) || validation.Code != store.CodeInterestNothing {
		t.Fatalf("err = %v, ожидалась ERR_INTEREST_NOTHING", err)
	}
}
