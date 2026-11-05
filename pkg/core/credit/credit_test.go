package credit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/credit"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/store/storetest"
)

// fixture — книга с кредитом, наличными и статьёй процентов.
type fixture struct {
	t        *testing.T
	db       *store.DB
	userID   int64
	wallet   *model.Account
	credit   *model.Account
	interest *model.Account
	service  *credit.Service
	date     time.Time
}

// newFixture создаёт книгу и долг 100 000 ₽ с начисленными процентами.
func newFixture(t *testing.T) *fixture {
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
	wallet := createAccount(t, db, "Наличные", model.TypeAsset, &assets.ID)
	liabilities := createAccount(t, db, "Обязательства", model.TypeLiability, nil)
	creditAccount := createAccount(t, db, "Кредит", model.TypeLiability, &liabilities.ID)
	expenses := createAccount(t, db, "Расходы", model.TypeExpense, nil)
	interest := createAccount(t, db, "Проценты", model.TypeExpense, &expenses.ID)

	if _, err := db.SetCreditSettings(ctx, creditAccount.ID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	date := time.Now().UTC()
	year, month, day := date.Date()
	date = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID:    "initial-debt",
		OpType:  model.OpInitial,
		Date:    date,
		ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: wallet.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideDebit},
			{AccountID: creditAccount.ID, Amount: money.FromKopecks(100_000_00), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(долг): %v", err)
	}

	return &fixture{
		t:        t,
		db:       db,
		userID:   user.ID,
		wallet:   wallet,
		credit:   creditAccount,
		interest: interest,
		service:  credit.New(db),
		date:     date,
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

// addAccrual добавляет начисление процентов со статусом accrued.
func (f *fixture) addAccrual(uuid string, amount money.Kop) {
	f.t.Helper()
	if _, err := f.db.ExecContext(context.Background(), `
		INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
		                              status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, 1, 1)`,
		uuid, f.credit.ID, f.date.Unix(), string(model.InterestAccrualCredit),
		int64(amount), string(model.InterestAccrued)); err != nil {
		f.t.Fatalf("interest_accruals: %v", err)
	}
}

// input собирает запрос на погашение с текущими счетами.
func (f *fixture) input(amount money.Kop) credit.RepaymentInput {
	return credit.RepaymentInput{
		UUID:              "repayment-1",
		Date:              f.date,
		AccountID:         f.credit.ID,
		SourceAccountID:   f.wallet.ID,
		InterestAccountID: f.interest.ID,
		Amount:            amount,
		ActorID:           f.userID,
		Source:            "test",
	}
}

// wantCode проверяет код ошибки валидации.
func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var ve *store.ValidationError
	if !errors.As(err, &ve) || ve.Code != code {
		t.Fatalf("err = %v, ожидалась ValidationError с кодом %s", err, code)
	}
}

// TestASL_17_PreviewInterestFirst проверяет порядок «сначала проценты».
func TestASL_17_PreviewInterestFirst(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))

	preview, err := f.service.Preview(context.Background(), f.input(money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Principal != money.FromKopecks(15_000_00) ||
		preview.Interest != money.FromKopecks(5_000_00) ||
		preview.Total != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка: %+v", preview)
	}
	if preview.Debt != money.FromKopecks(100_000_00) ||
		preview.AccruedInterest != money.FromKopecks(5_000_00) {
		t.Fatalf("остатки до платежа: %+v", preview)
	}
	if preview.RemainingDebt != money.FromKopecks(85_000_00) || preview.RemainingInterest != 0 {
		t.Fatalf("остатки после платежа: %+v", preview)
	}
	if preview.InGrace {
		t.Fatal("беспроцентный период не должен действовать")
	}
	if preview.UUID == "" || preview.Date != f.date {
		t.Fatalf("uuid/дата предпросмотра: %+v", preview)
	}
}

// TestASL_17_PreviewPrincipalFirst проверяет порядок «сначала тело».
func TestASL_17_PreviewPrincipalFirst(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))
	if _, err := f.db.SetCreditSettings(context.Background(), f.credit.ID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentPrincipalFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	preview, err := f.service.Preview(context.Background(), f.input(money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Principal != money.FromKopecks(20_000_00) || preview.Interest != 0 ||
		preview.Total != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка: %+v", preview)
	}

	// Суммы хватает на тело и часть процентов.
	preview, err = f.service.Preview(context.Background(), f.input(money.FromKopecks(101_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Principal != money.FromKopecks(100_000_00) ||
		preview.Interest != money.FromKopecks(1_000_00) ||
		preview.Total != money.FromKopecks(101_000_00) {
		t.Fatalf("разбивка с избытком: %+v", preview)
	}
}

// TestASL_17_PreviewGrace проверяет беспроцентный период: всё идёт в тело.
func TestASL_17_PreviewGrace(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))
	if _, err := f.db.SetCreditSettings(context.Background(), f.credit.ID, store.CreditSettingsInput{
		RatePPM:        150_000,
		RepaymentOrder: model.RepaymentInterestFirst,
		GraceEnabled:   true,
		GraceEndDate:   f.date.AddDate(0, 1, 0),
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	preview, err := f.service.Preview(context.Background(), f.input(money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !preview.InGrace {
		t.Fatal("беспроцентный период должен действовать")
	}
	if preview.Principal != money.FromKopecks(20_000_00) || preview.Interest != 0 {
		t.Fatalf("в grace всё должно идти в тело: %+v", preview)
	}
	if preview.RemainingInterest != money.FromKopecks(5_000_00) {
		t.Fatalf("начисленные проценты не должны списываться: %+v", preview)
	}
}

// TestASL_17_PreviewInsufficient проверяет недостаточную сумму.
func TestASL_17_PreviewInsufficient(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))

	preview, err := f.service.Preview(context.Background(), f.input(money.FromKopecks(3_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Principal != 0 || preview.Interest != money.FromKopecks(3_000_00) ||
		preview.Total != money.FromKopecks(3_000_00) {
		t.Fatalf("недостаточная сумма: %+v", preview)
	}
}

// TestASL_17_PreviewOverpayment проверяет сумму больше остатка долга.
func TestASL_17_PreviewOverpayment(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))

	preview, err := f.service.Preview(context.Background(), f.input(money.FromKopecks(200_000_00)))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Principal != money.FromKopecks(100_000_00) ||
		preview.Interest != money.FromKopecks(5_000_00) ||
		preview.Total != money.FromKopecks(105_000_00) ||
		preview.Requested != money.FromKopecks(200_000_00) {
		t.Fatalf("избыточная сумма: %+v", preview)
	}
	if preview.RemainingDebt != 0 || preview.RemainingInterest != 0 {
		t.Fatalf("остатки после избыточного платежа: %+v", preview)
	}
}

// TestASL_17_PreviewErrors проверяет ошибочные запросы.
func TestASL_17_PreviewErrors(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(*credit.RepaymentInput)
		code   string
	}{
		{"нулевая сумма", func(in *credit.RepaymentInput) { in.Amount = 0 }, store.CodeCreditAmount},
		{"отрицательная сумма", func(in *credit.RepaymentInput) { in.Amount = -1 }, store.CodeCreditAmount},
		{"без даты", func(in *credit.RepaymentInput) { in.Date = time.Time{} }, store.CodeCreditDate},
		{"неизвестный кредит", func(in *credit.RepaymentInput) { in.AccountID = 999999 }, store.CodeCreditAccountInvalid},
		{"кредит — актив", func(in *credit.RepaymentInput) { in.AccountID = f.wallet.ID }, store.CodeCreditAccountInvalid},
		{"источник — пассив", func(in *credit.RepaymentInput) { in.SourceAccountID = f.credit.ID }, store.CodeCreditSourceInvalid},
		{"проценты — доход", func(in *credit.RepaymentInput) { in.InterestAccountID = f.wallet.ID }, store.CodeCreditInterestAccountInvalid},
		{"нет статьи процентов", func(in *credit.RepaymentInput) { in.InterestAccountID = 0 }, store.CodeCreditInterestAccountInvalid},
	}
	for _, tc := range cases {
		in := f.input(money.FromKopecks(10_000_00))
		tc.mutate(&in)
		if _, err := f.service.Preview(ctx, in); err == nil {
			t.Fatalf("%s: ожидалась ошибка", tc.name)
		} else {
			wantCode(t, err, tc.code)
		}
	}

	// У счёта без настроек кредита погашение недоступно.
	noSettings := createAccount(t, f.db, "Без настроек", model.TypeLiability, nil)
	in := f.input(money.FromKopecks(10_000_00))
	in.AccountID = noSettings.ID
	if _, err := f.service.Preview(ctx, in); err == nil {
		t.Fatal("счёт без настроек принят")
	} else {
		wantCode(t, err, store.CodeCreditSettingsMissing)
	}

	// Погашать нечего: долг и проценты равны нулю.
	empty := createAccount(t, f.db, "Пустой кредит", model.TypeLiability, nil)
	if _, err := f.db.SetCreditSettings(ctx, empty.ID, store.CreditSettingsInput{RatePPM: 150_000}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	in = f.input(money.FromKopecks(10_000_00))
	in.AccountID = empty.ID
	if _, err := f.service.Preview(ctx, in); err == nil {
		t.Fatal("пустой кредит принят")
	} else {
		wantCode(t, err, store.CodeCreditNothingToPay)
	}
}

// TestASL_17_Post проверяет составную проводку и идемпотентность.
func TestASL_17_Post(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))
	ctx := context.Background()

	posted, err := f.service.Post(ctx, f.input(money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if posted.Principal != money.FromKopecks(15_000_00) ||
		posted.Interest != money.FromKopecks(5_000_00) ||
		posted.Total != money.FromKopecks(20_000_00) {
		t.Fatalf("разбивка проводки: %+v", posted)
	}
	if posted.Transaction.OpType != model.OpDebtPayment {
		t.Fatalf("тип операции = %q", posted.Transaction.OpType)
	}
	if len(posted.Transaction.Lines) != 3 {
		t.Fatalf("строк в проводке = %d, ожидалось 3", len(posted.Transaction.Lines))
	}

	var debit, creditTotal money.Kop
	sides := map[int64]model.Side{}
	amounts := map[int64]money.Kop{}
	for _, line := range posted.Transaction.Lines {
		sides[line.AccountID] = line.Side
		amounts[line.AccountID] = line.Amount
		if line.Side == model.SideDebit {
			debit += line.Amount
		} else {
			creditTotal += line.Amount
		}
	}
	if debit != creditTotal {
		t.Fatalf("ΣDEBIT = %s, ΣCREDIT = %s", debit, creditTotal)
	}
	if sides[f.credit.ID] != model.SideDebit || amounts[f.credit.ID] != money.FromKopecks(15_000_00) {
		t.Fatalf("тело долга: сторона %s, сумма %s", sides[f.credit.ID], amounts[f.credit.ID])
	}
	if sides[f.interest.ID] != model.SideDebit || amounts[f.interest.ID] != money.FromKopecks(5_000_00) {
		t.Fatalf("проценты: сторона %s, сумма %s", sides[f.interest.ID], amounts[f.interest.ID])
	}
	if sides[f.wallet.ID] != model.SideCredit || amounts[f.wallet.ID] != money.FromKopecks(20_000_00) {
		t.Fatalf("источник: сторона %s, сумма %s", sides[f.wallet.ID], amounts[f.wallet.ID])
	}

	debt, err := f.db.Balance(ctx, f.credit.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if debt != money.FromKopecks(85_000_00) {
		t.Fatalf("долг после платежа = %s", debt)
	}

	// Повтор с тем же uuid идемпотентен и возвращает прежнюю проводку.
	replay, err := f.service.Post(ctx, f.input(money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("повторный Post: %v", err)
	}
	if replay.Transaction.ID != posted.Transaction.ID {
		t.Fatalf("повтор создал новую проводку: %d != %d", replay.Transaction.ID, posted.Transaction.ID)
	}
	if replay.Principal != posted.Principal || replay.Interest != posted.Interest || replay.Total != posted.Total {
		t.Fatalf("повтор вернул другую разбивку: %+v", replay)
	}

	// Повтор после изменения остатка долга не должен падать.
	if _, err := f.service.Post(ctx, f.input(money.FromKopecks(200_000_00))); err != nil {
		t.Fatalf("повтор с изменившимся остатком: %v", err)
	}
}

// TestASL_17_PostGrace проверяет проводку в беспроцентный период.
func TestASL_17_PostGrace(t *testing.T) {
	f := newFixture(t)
	f.addAccrual("accrual-1", money.FromKopecks(5_000_00))
	if _, err := f.db.SetCreditSettings(context.Background(), f.credit.ID, store.CreditSettingsInput{
		RatePPM:      150_000,
		GraceEnabled: true,
		GraceEndDate: f.date.AddDate(0, 1, 0),
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}

	posted, err := f.service.Post(context.Background(), f.input(money.FromKopecks(20_000_00)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if posted.Principal != money.FromKopecks(20_000_00) || posted.Interest != 0 ||
		posted.Total != money.FromKopecks(20_000_00) {
		t.Fatalf("в grace всё должно идти в тело: %+v", posted)
	}
	if len(posted.Transaction.Lines) != 2 {
		t.Fatalf("строк в проводке = %d, ожидалось 2", len(posted.Transaction.Lines))
	}
}

// TestASL_17_PostGeneratesUUID проверяет генерацию uuid при пустом значении.
func TestASL_17_PostGeneratesUUID(t *testing.T) {
	f := newFixture(t)
	in := f.input(money.FromKopecks(10_000_00))
	in.UUID = ""

	posted, err := f.service.Post(context.Background(), in)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if posted.UUID == "" || posted.Transaction.UUID != posted.UUID {
		t.Fatalf("uuid проводки: %+v", posted)
	}
}
