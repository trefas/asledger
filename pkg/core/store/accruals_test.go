package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// accrualDay возвращает дату без времени.
func accrualDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// accrualFixture — книга с вкладом и кредитом для тестов начислений.
type accrualFixture struct {
	db       *DB
	userID   int64
	deposit  *model.Account
	income   *model.Account
	credit   *model.Account
	expense  *model.Account
	from, to time.Time
}

// accrualRoot возвращает корневой счёт, создавая его при необходимости.
func accrualRoot(t *testing.T, db *DB, name string, typ model.AccountType) *model.Account {
	t.Helper()
	ctx := context.Background()
	account, err := db.AccountByPath(ctx, name)
	if err == nil {
		return account
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("AccountByPath(%s): %v", name, err)
	}
	return mustCreateAccount(t, db, name, typ, nil)
}

// newAccrualFixture создаёт счета и настройки продуктов.
func newAccrualFixture(t *testing.T) *accrualFixture {
	t.Helper()
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")

	assets := accrualRoot(t, db, "Активы", model.TypeAsset)
	deposit := mustCreateAccount(t, db, "Вклад", model.TypeAsset, &assets.ID)
	incomeRoot := accrualRoot(t, db, "Доходы", model.TypeIncome)
	income := mustCreateAccount(t, db, "Проценты", model.TypeIncome, &incomeRoot.ID)
	liabilities := accrualRoot(t, db, "Обязательства", model.TypeLiability)
	credit := mustCreateAccount(t, db, "Кредит", model.TypeLiability, &liabilities.ID)
	expenses := accrualRoot(t, db, "Расходы", model.TypeExpense)
	expense := mustCreateAccount(t, db, "Проценты", model.TypeExpense, &expenses.ID)

	if _, err := db.SetDepositSettings(ctx, deposit.ID, DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	if _, err := db.SetCreditSettings(ctx, credit.ID, CreditSettingsInput{
		RatePPM: 150_000,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	return &accrualFixture{
		db:      db,
		userID:  userID,
		deposit: deposit,
		income:  income,
		credit:  credit,
		expense: expense,
		from:    accrualDay(2026, time.January, 1),
		to:      accrualDay(2026, time.February, 1),
	}
}

// TestASL_18_EnsureInterestAccrual проверяет идемпотентность по uuid и периоду.
func TestASL_18_EnsureInterestAccrual(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	input := InterestAccrualInput{
		UUID:       "accrual-1",
		AccountID:  f.deposit.ID,
		Date:       f.to,
		Kind:       model.InterestAccrualDeposit,
		Amount:     money.FromKopecks(1_273_97),
		BaseAmount: money.FromKopecks(100_000_00),
		RatePPM:    150_000,
		PeriodFrom: f.from,
		PeriodTo:   f.to,
	}
	created, isNew, err := f.db.EnsureInterestAccrual(ctx, input)
	if err != nil {
		t.Fatalf("EnsureInterestAccrual: %v", err)
	}
	if !isNew || created.Status != model.InterestAccrued {
		t.Fatalf("созданное начисление: new=%v, %+v", isNew, created)
	}
	if created.Amount != input.Amount || !created.PeriodFrom.Equal(f.from) {
		t.Fatalf("поля начисления: %+v", created)
	}
	if !changelogHas(t, f.db, model.EntityInterestAccrual, created.ID, model.OpInsert) {
		t.Fatal("в журнале нет создания начисления")
	}

	// Повтор по uuid возвращает ту же запись.
	same, isNew, err := f.db.EnsureInterestAccrual(ctx, input)
	if err != nil {
		t.Fatalf("повтор по uuid: %v", err)
	}
	if isNew || same.ID != created.ID {
		t.Fatalf("повтор по uuid создал дубль: new=%v, %+v", isNew, same)
	}

	// Другой uuid за тот же период тоже не создаёт дубль.
	input.UUID = "accrual-2"
	same, isNew, err = f.db.EnsureInterestAccrual(ctx, input)
	if err != nil {
		t.Fatalf("повтор по периоду: %v", err)
	}
	if isNew || same.ID != created.ID {
		t.Fatalf("повтор по периоду создал дубль: new=%v, %+v", isNew, same)
	}

	// Ручные начисления с нулевым периодом могут повторяться.
	manual := input
	manual.UUID = "manual-1"
	manual.PeriodFrom, manual.PeriodTo = time.Time{}, time.Time{}
	first, _, err := f.db.EnsureInterestAccrual(ctx, manual)
	if err != nil {
		t.Fatalf("ручное начисление: %v", err)
	}
	manual.UUID = "manual-2"
	second, _, err := f.db.EnsureInterestAccrual(ctx, manual)
	if err != nil {
		t.Fatalf("второе ручное начисление: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("ручные начисления ошибочно склеены")
	}
}

// TestASL_18_InterestAccrualsFilter проверяет выборку по счёту, виду и статусу.
func TestASL_18_InterestAccrualsFilter(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	deposit, _, err := f.db.EnsureInterestAccrual(ctx, InterestAccrualInput{
		UUID: "deposit-accrual", AccountID: f.deposit.ID, Date: f.to,
		Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(1_000_00),
	})
	if err != nil {
		t.Fatalf("EnsureInterestAccrual(вклад): %v", err)
	}
	if _, _, err := f.db.EnsureInterestAccrual(ctx, InterestAccrualInput{
		UUID: "credit-accrual", AccountID: f.credit.ID, Date: f.to,
		Kind: model.InterestAccrualCredit, Amount: money.FromKopecks(2_000_00),
	}); err != nil {
		t.Fatalf("EnsureInterestAccrual(кредит): %v", err)
	}

	all, err := f.db.InterestAccruals(ctx, 0, "", "")
	if err != nil {
		t.Fatalf("InterestAccruals: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("всего начислений = %d, ожидалось 2", len(all))
	}
	onlyDeposit, err := f.db.InterestAccruals(ctx, f.deposit.ID, model.InterestAccrualDeposit, "")
	if err != nil {
		t.Fatalf("InterestAccruals(вклад): %v", err)
	}
	if len(onlyDeposit) != 1 || onlyDeposit[0].ID != deposit.ID {
		t.Fatalf("выборка по вкладу: %+v", onlyDeposit)
	}

	byPeriod, err := f.db.InterestAccrualByPeriod(ctx, f.deposit.ID,
		model.InterestAccrualDeposit, f.from, f.to)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("InterestAccrualByPeriod(другой период) = %v, ожидался ErrNotFound", err)
	}
	if byPeriod != nil {
		t.Fatalf("неожиданное начисление: %+v", byPeriod)
	}

	if _, err := f.db.InterestAccrualByUUID(ctx, "нет такого"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("InterestAccrualByUUID = %v, ожидался ErrNotFound", err)
	}
}

// TestASL_18_PostInterestAccrual проверяет запись начисления с проводкой.
func TestASL_18_PostInterestAccrual(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	transaction := &model.Transaction{
		UUID:        "credit-accrual-tx",
		OpType:      model.OpInterestAccrual,
		Date:        f.to,
		Description: "Начисление процентов по кредиту",
		ActorID:     f.userID,
		Lines: []model.TransactionLine{
			{AccountID: f.expense.ID, Amount: money.FromKopecks(2_000_00), Side: model.SideDebit},
			{AccountID: f.credit.ID, Amount: money.FromKopecks(2_000_00), Side: model.SideCredit},
		},
	}
	accrual, err := f.db.PostInterestAccrual(ctx, InterestAccrualInput{
		UUID:       "credit-accrual",
		AccountID:  f.credit.ID,
		Date:       f.to,
		Kind:       model.InterestAccrualCredit,
		Amount:     money.FromKopecks(2_000_00),
		BaseAmount: money.FromKopecks(100_000_00),
		RatePPM:    150_000,
		PeriodFrom: f.from,
		PeriodTo:   f.to,
	}, transaction)
	if err != nil {
		t.Fatalf("PostInterestAccrual: %v", err)
	}
	if accrual.TransactionID == nil {
		t.Fatal("начисление не связано с проводкой")
	}
	stored, err := f.db.Transaction(ctx, *accrual.TransactionID)
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if stored.UUID != transaction.UUID || stored.OpType != model.OpInterestAccrual {
		t.Fatalf("сохранённая проводка: %+v", stored)
	}
	if _, err := f.db.TransactionByUUID(ctx, transaction.UUID); err != nil {
		t.Fatalf("проводка не сохранена: %v", err)
	}
}

// TestASL_18_PayInterestAccruals проверяет выплату по возрастанию даты и
// ограничение по дате операции.
func TestASL_18_PayInterestAccruals(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	// Три начисления: две даты до операции и одна позже.
	for i, spec := range []struct {
		uuid   string
		date   time.Time
		amount money.Kop
	}{
		{"pay-1", accrualDay(2026, time.January, 10), money.FromKopecks(300_00)},
		{"pay-2", accrualDay(2026, time.January, 20), money.FromKopecks(200_00)},
		{"pay-3", accrualDay(2026, time.February, 10), money.FromKopecks(100_00)},
	} {
		if _, _, err := f.db.EnsureInterestAccrual(ctx, InterestAccrualInput{
			UUID: spec.uuid, AccountID: f.deposit.ID, Date: spec.date,
			Kind: model.InterestAccrualDeposit, Amount: spec.amount,
		}); err != nil {
			t.Fatalf("EnsureInterestAccrual(%d): %v", i, err)
		}
	}

	transaction := &model.Transaction{
		UUID: "payout-tx", OpType: model.OpIncome, Date: accrualDay(2026, time.January, 31),
		ActorID: f.userID,
		Lines: []model.TransactionLine{
			{AccountID: f.deposit.ID, Amount: money.FromKopecks(500_00), Side: model.SideDebit},
			{AccountID: f.income.ID, Amount: money.FromKopecks(500_00), Side: model.SideCredit},
		},
	}
	trID := mustInsertTransaction(t, f.db, transaction)

	// 500 ₽ хватает ровно на две записи; третья остаётся accrued.
	paid, err := f.db.PayInterestAccruals(ctx, f.deposit.ID,
		[]model.InterestAccrualKind{model.InterestAccrualDeposit},
		money.FromKopecks(500_00), trID, accrualDay(2026, time.January, 31))
	if err != nil {
		t.Fatalf("PayInterestAccruals: %v", err)
	}
	if paid != money.FromKopecks(500_00) {
		t.Fatalf("выплачено = %s, ожидалось 500,00 ₽", paid)
	}

	accrued, err := f.db.AccruedInterest(ctx, f.deposit.ID, model.InterestAccrualDeposit)
	if err != nil {
		t.Fatalf("AccruedInterest: %v", err)
	}
	if accrued != money.FromKopecks(100_00) {
		t.Fatalf("остаток начислений = %s, ожидалось 100,00 ₽", accrued)
	}
	atDate, err := f.db.AccruedInterestAt(ctx, f.deposit.ID,
		model.InterestAccrualDeposit, accrualDay(2026, time.January, 31))
	if err != nil {
		t.Fatalf("AccruedInterestAt: %v", err)
	}
	if atDate != 0 {
		t.Fatalf("на 31 января остаток = %s, ожидалось 0", atDate)
	}

	paidRows, err := f.db.InterestAccruals(ctx, f.deposit.ID,
		model.InterestAccrualDeposit, model.InterestPaid)
	if err != nil {
		t.Fatalf("InterestAccruals(paid): %v", err)
	}
	if len(paidRows) != 2 {
		t.Fatalf("выплаченных начислений = %d, ожидалось 2", len(paidRows))
	}
	for _, row := range paidRows {
		if row.TransactionID == nil || *row.TransactionID != trID {
			t.Fatalf("начисление %s не связано с проводкой выплаты", row.UUID)
		}
	}
}

// TestASL_18_InterestAccrualValidation проверяет коды ошибок.
func TestASL_18_InterestAccrualValidation(t *testing.T) {
	f := newAccrualFixture(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		input InterestAccrualInput
		code  string
	}{
		{"uuid", InterestAccrualInput{AccountID: f.deposit.ID, Date: f.to,
			Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(100)}, CodeInterestUUID},
		{"account", InterestAccrualInput{UUID: "v-1", Date: f.to,
			Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(100)}, CodeInterestAccount},
		{"date", InterestAccrualInput{UUID: "v-2", AccountID: f.deposit.ID,
			Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(100)}, CodeInterestDate},
		{"kind", InterestAccrualInput{UUID: "v-3", AccountID: f.deposit.ID, Date: f.to,
			Kind: model.InterestAccrualKind("fee"), Amount: money.FromKopecks(100)}, CodeInterestKind},
		{"amount", InterestAccrualInput{UUID: "v-4", AccountID: f.deposit.ID, Date: f.to,
			Kind: model.InterestAccrualDeposit}, CodeInterestAmount},
		{"rate", InterestAccrualInput{UUID: "v-5", AccountID: f.deposit.ID, Date: f.to,
			Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(100),
			RatePPM: 1_000_001}, CodeInterestRate},
		{"status", InterestAccrualInput{UUID: "v-6", AccountID: f.deposit.ID, Date: f.to,
			Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(100),
			Status: model.InterestAccrualStatus("closed")}, CodeInterestStatus},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := f.db.EnsureInterestAccrual(ctx, tc.input)
			wantValidationCode(t, err, tc.code)
		})
	}
}
