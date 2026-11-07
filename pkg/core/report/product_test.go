package report

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/store/storetest"
)

// productFixture — книга с кредитом и вкладом для отчётов по продуктам.
type productFixture struct {
	db      *store.DB
	asOf    time.Time
	credit  *model.Account
	deposit *model.Account
	cash    *model.Account
}

// newProductFixture собирает кредит с долгом и вкладом с остатком.
func newProductFixture(t *testing.T) *productFixture {
	t.Helper()
	ctx := context.Background()
	db := storetest.OpenTemp(t)
	user, err := db.CreateUser(ctx, "Андрей", "секрет-1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	assets, err := db.AccountByPath(ctx, "Активы")
	if err != nil {
		t.Fatalf("AccountByPath(Активы): %v", err)
	}
	liabilities, err := db.CreateAccount(ctx, store.AccountInput{
		Name: "Обязательства", Type: model.TypeLiability,
	})
	if err != nil {
		t.Fatalf("CreateAccount(Обязательства): %v", err)
	}
	credit, err := db.CreateAccount(ctx, store.AccountInput{
		Name: "Кредит", Type: model.TypeLiability, ParentID: &liabilities.ID,
	})
	if err != nil {
		t.Fatalf("CreateAccount(Кредит): %v", err)
	}
	cash, err := db.CreateAccount(ctx, store.AccountInput{
		Name: "Наличные", Type: model.TypeAsset, ParentID: &assets.ID,
	})
	if err != nil {
		t.Fatalf("CreateAccount(Наличные): %v", err)
	}
	deposit, err := db.CreateAccount(ctx, store.AccountInput{
		Name: "Вклад", Type: model.TypeAsset, ParentID: &assets.ID,
	})
	if err != nil {
		t.Fatalf("CreateAccount(Вклад): %v", err)
	}
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath(Капитал:Начальные): %v", err)
	}

	asOf := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	initial := asOf.AddDate(0, 0, -10)

	if _, err := db.SetCreditSettings(ctx, credit.ID, store.CreditSettingsInput{
		RatePPM:        120_000,
		InterestType:   model.CreditInterestSimple,
		RepaymentOrder: model.RepaymentInterestFirst,
	}); err != nil {
		t.Fatalf("SetCreditSettings: %v", err)
	}
	if _, err := db.SetDepositSettings(ctx, deposit.ID, store.DepositSettingsInput{
		BaseRatePPM:      100_000,
		AccrualFrequency: model.DepositAccrualDaily,
		OpenedAt:         asOf.AddDate(0, 0, -30),
	}); err != nil {
		t.Fatalf("SetDepositSettings: %v", err)
	}

	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID: "product-debt", OpType: model.OpInitial, Date: initial, ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: cash.ID, Amount: money.FromKopecks(1_000_000), Side: model.SideDebit},
			{AccountID: credit.ID, Amount: money.FromKopecks(1_000_000), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(долг): %v", err)
	}
	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID: "product-deposit", OpType: model.OpInitial, Date: initial, ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: deposit.ID, Amount: money.FromKopecks(1_000_000), Side: model.SideDebit},
			{AccountID: capital.ID, Amount: money.FromKopecks(1_000_000), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction(вклад): %v", err)
	}

	accrual := func(uuid string, accountID int64, kind model.InterestAccrualKind, amount money.Kop) {
		t.Helper()
		if _, _, err := db.EnsureInterestAccrual(ctx, store.InterestAccrualInput{
			UUID:       uuid,
			AccountID:  accountID,
			Date:       asOf.AddDate(0, 0, -5),
			Kind:       kind,
			Amount:     amount,
			BaseAmount: money.FromKopecks(1_000_000),
			RatePPM:    120_000,
		}); err != nil {
			t.Fatalf("EnsureInterestAccrual(%s): %v", uuid, err)
		}
	}
	accrual("product-credit-accrual", credit.ID, model.InterestAccrualCredit, money.FromKopecks(50_000))
	accrual("product-deposit-accrual", deposit.ID, model.InterestAccrualDeposit, money.FromKopecks(25_000))

	return &productFixture{db: db, asOf: asOf, credit: credit, deposit: deposit, cash: cash}
}

// TestASL_18_CreditSummary проверяет долг, проценты и предстоящие начисления.
func TestASL_18_CreditSummary(t *testing.T) {
	fx := newProductFixture(t)
	rep, err := CreditSummary(context.Background(), fx.db, ProductOptions{
		AsOf: fx.asOf, HorizonDays: 30,
	})
	if err != nil {
		t.Fatalf("CreditSummary: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("строк в отчёте: %d", len(rep.Rows))
	}
	row := rep.Rows[0]
	if row.AccountID != fx.credit.ID || row.Path != "Обязательства:Кредит" {
		t.Fatalf("счёт в отчёте: %+v", row)
	}
	if row.Debt != money.FromKopecks(1_000_000) {
		t.Errorf("долг: %s", row.Debt)
	}
	if row.AccruedInterest != money.FromKopecks(50_000) {
		t.Errorf("накопленные проценты: %s", row.AccruedInterest)
	}
	if row.TotalDebt != money.FromKopecks(1_050_000) {
		t.Errorf("долг с процентами: %s", row.TotalDebt)
	}
	if len(row.Upcoming) != 1 {
		t.Fatalf("предстоящих начислений: %d", len(row.Upcoming))
	}
	upcoming := row.Upcoming[0]
	if upcoming.Days != 30 || upcoming.Amount != money.FromKopecks(9_863) {
		t.Errorf("прогноз начисления: %+v", upcoming)
	}
	if row.UpcomingInterest != money.FromKopecks(9_863) {
		t.Errorf("итог прогноза: %s", row.UpcomingInterest)
	}
	if rep.Debt != row.Debt || rep.AccruedInterest != row.AccruedInterest ||
		rep.TotalDebt != row.TotalDebt || rep.UpcomingInterest != row.UpcomingInterest {
		t.Errorf("итоги отчёта: %+v", rep)
	}
}

// TestASL_18_DepositSummary проверяет остаток, проценты и прогноз вклада.
func TestASL_18_DepositSummary(t *testing.T) {
	fx := newProductFixture(t)
	rep, err := DepositSummary(context.Background(), fx.db, ProductOptions{
		AsOf: fx.asOf, HorizonDays: 10,
	})
	if err != nil {
		t.Fatalf("DepositSummary: %v", err)
	}
	if len(rep.Rows) != 1 {
		t.Fatalf("строк в отчёте: %d", len(rep.Rows))
	}
	row := rep.Rows[0]
	if row.AccountID != fx.deposit.ID || row.Path != "Активы:Вклад" {
		t.Fatalf("счёт в отчёте: %+v", row)
	}
	if row.Balance != money.FromKopecks(1_000_000) {
		t.Errorf("остаток: %s", row.Balance)
	}
	if row.AccruedInterest != money.FromKopecks(25_000) {
		t.Errorf("накопленные проценты: %s", row.AccruedInterest)
	}
	if row.Total != money.FromKopecks(1_025_000) {
		t.Errorf("остаток с процентами: %s", row.Total)
	}
	if len(row.Upcoming) != 10 {
		t.Fatalf("предстоящих начислений: %d", len(row.Upcoming))
	}
	if row.UpcomingInterest != money.FromKopecks(2_740) {
		t.Errorf("прогноз процентов: %s", row.UpcomingInterest)
	}
	if row.UpcomingTotal != row.UpcomingInterest+row.UpcomingBonus {
		t.Errorf("итог прогноза: %+v", row)
	}
}

// TestASL_18_CreditForecast проверяет изменение долга и процентов после платежа.
func TestASL_18_CreditForecast(t *testing.T) {
	fx := newProductFixture(t)
	forecast, err := CreditForecast(context.Background(), fx.db, CreditForecastOptions{
		AccountID:   fx.credit.ID,
		Amount:      money.FromKopecks(200_000),
		Date:        fx.asOf,
		HorizonDays: 30,
	})
	if err != nil {
		t.Fatalf("CreditForecast: %v", err)
	}
	if forecast.Principal != money.FromKopecks(150_000) ||
		forecast.Interest != money.FromKopecks(50_000) ||
		forecast.Total != money.FromKopecks(200_000) {
		t.Errorf("разбивка платежа: %+v", forecast)
	}
	if forecast.DebtBefore != money.FromKopecks(1_000_000) ||
		forecast.AccruedInterestBefore != money.FromKopecks(50_000) ||
		forecast.TotalDebtBefore != money.FromKopecks(1_050_000) {
		t.Errorf("состояние до платежа: %+v", forecast)
	}
	if forecast.DebtAfter != money.FromKopecks(850_000) ||
		forecast.AccruedInterestAfter != 0 ||
		forecast.TotalDebtAfter != money.FromKopecks(850_000) {
		t.Errorf("состояние после платежа: %+v", forecast)
	}
	if forecast.DebtChange != money.FromKopecks(-150_000) ||
		forecast.InterestChange != money.FromKopecks(-50_000) {
		t.Errorf("изменение: долг %s, проценты %s", forecast.DebtChange, forecast.InterestChange)
	}
	if len(forecast.Upcoming) != 1 || forecast.Upcoming[0].Amount != money.FromKopecks(8_384) {
		t.Errorf("прогноз после платежа: %+v", forecast.Upcoming)
	}

	_, err = CreditForecast(context.Background(), fx.db, CreditForecastOptions{
		AccountID: fx.credit.ID, Amount: 0, Date: fx.asOf,
	})
	var ve *store.ValidationError
	if !errors.As(err, &ve) || ve.Code != store.CodeCreditAmount {
		t.Errorf("ошибка нулевой суммы: %v", err)
	}

	_, err = CreditSummary(context.Background(), fx.db, ProductOptions{
		AccountID: fx.cash.ID, AsOf: fx.asOf,
	})
	if !errors.As(err, &ve) || ve.Code != store.CodeCreditAccountInvalid {
		t.Errorf("ошибка счёта сводки: %v", err)
	}
}
