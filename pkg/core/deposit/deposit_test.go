package deposit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/deposit"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/store/storetest"
)

// depositFixture — книга с вкладом, получателем и статьёй доходов.
type depositFixture struct {
	t      *testing.T
	db     *store.DB
	userID int64
	depo   *model.Account
	wallet *model.Account
	income *model.Account
	svc    *deposit.Service
	date   time.Time
}

// newDepositFixture создаёт книгу с вкладом на 100 000 ₽ и процентами 5 000 ₽.
func newDepositFixture(t *testing.T) *depositFixture {
	t.Helper()
	ctx := context.Background()
	db := storetest.OpenTemp(t)

	user, err := db.CreateUser(ctx, "Андрей", "секрет-андрея")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	assets, err := db.AccountByPath(ctx, "Активы")
	if err != nil {
		t.Fatalf("AccountByPath(Активы): %v", err)
	}
	depo := createAccount(t, db, "Вклад", model.TypeAsset, &assets.ID)
	wallet := createAccount(t, db, "Наличные", model.TypeAsset, &assets.ID)
	incomeRoot := createAccount(t, db, "Доходы", model.TypeIncome, nil)
	income := createAccount(t, db, "Проценты по вкладам", model.TypeIncome, &incomeRoot.ID)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath(Капитал:Начальные): %v", err)
	}

	now := time.Now().UTC()
	year, month, day := now.Date()
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if _, err := db.SetDepositSettings(ctx, depo.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		OpenedAt:         date,
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}
	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "deposit-initial",
		OpType:  model.OpInitial,
		Date:    date,
		ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: depo.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: capital.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(вклад): %v", err)
	}
	if _, _, err := db.EnsureInterestAccrual(ctx, store.InterestAccrualInput{
		UUID: "deposit-accrual", AccountID: depo.ID, Date: date,
		Kind: model.InterestAccrualDeposit, Amount: money.FromKopecks(5_000_00),
	}); err != nil {
		t.Fatalf("EnsureInterestAccrual: %v", err)
	}

	return &depositFixture{
		t: t, db: db, userID: user.ID, depo: depo, wallet: wallet,
		income: income, svc: deposit.New(db), date: date,
	}
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

// input собирает запрос на снятие.
func (f *depositFixture) input(uuid string, amount money.Kop) deposit.WithdrawalInput {
	return deposit.WithdrawalInput{
		UUID:                 uuid,
		Date:                 f.date,
		AccountID:            f.depo.ID,
		DestinationAccountID: f.wallet.ID,
		IncomeAccountID:      f.income.ID,
		Amount:               amount,
		ActorID:              f.userID,
		Source:               "test",
	}
}

// TestASL_18_DepositWithdrawalPreview проверяет разбивку без записи.
func TestASL_18_DepositWithdrawalPreview(t *testing.T) {
	f := newDepositFixture(t)
	ctx := context.Background()

	preview, err := f.svc.Preview(ctx, f.input("preview-1", money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Principal != money.FromKopecks(15_000_00) ||
		preview.Interest != money.FromKopecks(5_000_00) ||
		preview.Total != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка: %+v", preview)
	}
	if preview.Balance != money.FromKopecks(100_000_00) ||
		preview.AccruedInterest != money.FromKopecks(5_000_00) ||
		preview.RemainingBalance != money.FromKopecks(85_000_00) ||
		preview.RemainingInterest != 0 {
		t.Fatalf("остатки: %+v", preview)
	}
	if _, err := f.db.TransactionByUUID(ctx, "preview-1-principal"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("предпросмотр создал проводку: %v", err)
	}
}

// TestASL_18_DepositWithdrawalPost проверяет две проводки и статусы.
func TestASL_18_DepositWithdrawalPost(t *testing.T) {
	f := newDepositFixture(t)
	ctx := context.Background()

	posted, err := f.svc.Post(ctx, f.input("post-1", money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if posted.Principal != money.FromKopecks(15_000_00) ||
		posted.Interest != money.FromKopecks(5_000_00) ||
		posted.Total != money.FromKopecks(20_000_00) {
		t.Fatalf("результат: %+v", posted)
	}
	if posted.PrincipalTransaction == nil || posted.InterestTransaction == nil {
		t.Fatal("не созданы обе проводки")
	}
	if posted.PrincipalTransaction.OpType != model.OpTransfer ||
		posted.InterestTransaction.OpType != model.OpIncome {
		t.Fatalf("типы проводок: %q, %q",
			posted.PrincipalTransaction.OpType, posted.InterestTransaction.OpType)
	}

	depositBalance, err := f.db.Balance(ctx, f.depo.ID)
	if err != nil {
		t.Fatalf("Balance(вклад): %v", err)
	}
	walletBalance, err := f.db.Balance(ctx, f.wallet.ID)
	if err != nil {
		t.Fatalf("Balance(наличные): %v", err)
	}
	incomeBalance, err := f.db.Balance(ctx, f.income.ID)
	if err != nil {
		t.Fatalf("Balance(доходы): %v", err)
	}
	if depositBalance != money.FromKopecks(85_000_00) ||
		walletBalance != money.FromKopecks(20_000_00) ||
		incomeBalance != money.FromKopecks(5_000_00) {
		t.Fatalf("остатки: вклад %s, наличные %s, доходы %s",
			depositBalance, walletBalance, incomeBalance)
	}

	accrued, err := f.db.AccruedInterest(ctx, f.depo.ID, model.InterestAccrualDeposit)
	if err != nil {
		t.Fatalf("AccruedInterest: %v", err)
	}
	if accrued != 0 {
		t.Fatalf("проценты не отмечены выплаченными: %s", accrued)
	}
}

// TestASL_18_DepositWithdrawalRepeat проверяет идемпотентность.
func TestASL_18_DepositWithdrawalRepeat(t *testing.T) {
	f := newDepositFixture(t)
	ctx := context.Background()

	first, err := f.svc.Post(ctx, f.input("repeat-1", money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	second, err := f.svc.Post(ctx, f.input("repeat-1", money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("повтор Post: %v", err)
	}
	if first.PrincipalTransaction.ID != second.PrincipalTransaction.ID ||
		first.InterestTransaction.ID != second.InterestTransaction.ID {
		t.Fatal("повтор создал новые проводки")
	}
	depositBalance, err := f.db.Balance(ctx, f.depo.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if depositBalance != money.FromKopecks(85_000_00) {
		t.Fatalf("повтор изменил остаток: %s", depositBalance)
	}
}

// TestASL_18_DepositWithdrawalClose проверяет полное закрытие вклада.
func TestASL_18_DepositWithdrawalClose(t *testing.T) {
	f := newDepositFixture(t)
	ctx := context.Background()

	if _, err := f.db.SetDepositSettings(ctx, f.depo.ID, store.DepositSettingsInput{
		BaseRatePPM:      150_000,
		AccrualFrequency: model.DepositAccrualMonthly,
		OpenedAt:         f.date,
		ClosedAt:         f.date,
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}

	posted, err := f.svc.Post(ctx, f.input("close-1", money.FromKopecks(105_000_00)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if posted.Principal != money.FromKopecks(100_000_00) ||
		posted.Interest != money.FromKopecks(5_000_00) ||
		posted.Total != money.FromKopecks(105_000_00) {
		t.Fatalf("закрытие: %+v", posted)
	}

	balance, err := f.db.Balance(ctx, f.depo.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	accrued, err := f.db.AccruedInterest(ctx, f.depo.ID, model.InterestAccrualDeposit)
	if err != nil {
		t.Fatalf("AccruedInterest: %v", err)
	}
	if balance != 0 || accrued != 0 {
		t.Fatalf("после закрытия остаток %s, проценты %s", balance, accrued)
	}

	// Повторное снятие нечего проводить.
	_, err = f.svc.Post(ctx, f.input("close-2", money.FromKopecks(1_00)))
	var validation *store.ValidationError
	if !errors.As(err, &validation) || validation.Code != store.CodeDepositNothingToWithdraw {
		t.Fatalf("повторное снятие = %v, ожидалась ERR_DEPOSIT_NOTHING_TO_WITHDRAW", err)
	}
}

// TestASL_18_DepositWithdrawalOverpay проверяет ограничение суммы остатком.
func TestASL_18_DepositWithdrawalOverpay(t *testing.T) {
	f := newDepositFixture(t)

	preview, err := f.svc.Preview(context.Background(),
		f.input("overpay-1", money.FromKopecks(200_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Total != money.FromKopecks(105_000_00) ||
		preview.Principal != money.FromKopecks(100_000_00) ||
		preview.Interest != money.FromKopecks(5_000_00) {
		t.Fatalf("ограничение суммы: %+v", preview)
	}
}

// TestASL_18_DepositWithdrawalErrors проверяет коды ошибок.
func TestASL_18_DepositWithdrawalErrors(t *testing.T) {
	f := newDepositFixture(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		input deposit.WithdrawalInput
		code  string
	}{
		{"amount", func() deposit.WithdrawalInput {
			in := f.input("err-amount", 0)
			return in
		}(), store.CodeDepositAmount},
		{"date", func() deposit.WithdrawalInput {
			in := f.input("err-date", money.FromKopecks(100_00))
			in.Date = time.Time{}
			return in
		}(), store.CodeDepositDate},
		{"account", func() deposit.WithdrawalInput {
			in := f.input("err-account", money.FromKopecks(100_00))
			in.AccountID = 999999
			return in
		}(), store.CodeDepositAccountInvalid},
		{"destination", func() deposit.WithdrawalInput {
			in := f.input("err-dest", money.FromKopecks(100_00))
			in.DestinationAccountID = f.income.ID
			return in
		}(), store.CodeDepositDestinationInvalid},
		{"income", func() deposit.WithdrawalInput {
			in := f.input("err-income", money.FromKopecks(100_00))
			in.IncomeAccountID = f.wallet.ID
			return in
		}(), store.CodeDepositIncomeInvalid},
		{"income-required", func() deposit.WithdrawalInput {
			in := f.input("err-income-required", money.FromKopecks(20_000_00))
			in.IncomeAccountID = 0
			return in
		}(), store.CodeDepositIncomeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.Preview(ctx, tc.input)
			var validation *store.ValidationError
			if !errors.As(err, &validation) || validation.Code != tc.code {
				t.Fatalf("err = %v, ожидалась ValidationError с кодом %s", err, tc.code)
			}
		})
	}

	// Счёт без настроек вклада.
	other := createAccount(t, f.db, "Без настроек", model.TypeAsset, nil)
	in := f.input("err-settings", money.FromKopecks(100_00))
	in.AccountID = other.ID
	_, err := f.svc.Preview(ctx, in)
	var validation *store.ValidationError
	if !errors.As(err, &validation) || validation.Code != store.CodeDepositSettingsMissing {
		t.Fatalf("счёт без настроек = %v, ожидалась ERR_DEPOSIT_SETTINGS_MISSING", err)
	}
}
