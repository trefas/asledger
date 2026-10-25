package reconcile

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

// testBook — книга с одним активом, доходом и проводкой на 500,00 ₽.
type testBook struct {
	db       *store.DB
	service  *Service
	actorID  int64
	assetID  int64
	incomeID int64
}

// newTestBook поднимает книгу с активом и доходом и вносит 500,00 ₽.
func newTestBook(t *testing.T) *testBook {
	t.Helper()
	ctx := context.Background()

	db := storetest.OpenTemp(t)
	user, err := db.CreateUser(ctx, "Андрей", "секрет")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	asset, err := db.CreateAccount(ctx, store.AccountInput{Name: "Кошелёк", Type: model.TypeAsset})
	if err != nil {
		t.Fatalf("CreateAccount(актив): %v", err)
	}
	income, err := db.CreateAccount(ctx, store.AccountInput{Name: "Прочие", Type: model.TypeIncome})
	if err != nil {
		t.Fatalf("CreateAccount(доход): %v", err)
	}
	if _, err := db.InsertTransaction(ctx, &model.Transaction{
		UUID: "income-1", OpType: model.OpIncome, Date: time.Now().UTC(), ActorID: user.ID,
		Lines: []model.TransactionLine{
			{AccountID: asset.ID, Amount: money.FromKopecks(50_000), Side: model.SideDebit},
			{AccountID: income.ID, Amount: money.FromKopecks(50_000), Side: model.SideCredit},
		},
	}); err != nil {
		t.Fatalf("InsertTransaction: %v", err)
	}
	return &testBook{
		db:       db,
		service:  New(db),
		actorID:  user.ID,
		assetID:  asset.ID,
		incomeID: income.ID,
	}
}

// TestASL_9_ReconcileGeneratesAdjustment проверяет сверку с корректировкой.
func TestASL_9_ReconcileGeneratesAdjustment(t *testing.T) {
	book := newTestBook(t)
	ctx := context.Background()

	// Фактический остаток на 50,00 ₽ меньше расчётного.
	result, err := book.service.Check(ctx, Request{
		AccountID: book.assetID,
		Fact:      money.FromKopecks(45_000),
		Date:      time.Now().UTC(),
		ActorID:   book.actorID,
		Generate:  true,
		UUID:      "reconcile-1",
		Source:    "web",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Difference != money.FromKopecks(-5_000) {
		t.Errorf("расхождение: %s, ожидалось -50,00 ₽", result.Difference)
	}
	if result.Transaction == nil {
		t.Fatal("корректировка не создана")
	}
	if result.Transaction.OpType != model.OpAdjustment {
		t.Errorf("тип корректировки: %s", result.Transaction.OpType)
	}
	if result.Reconciliation.ID == 0 || result.Reconciliation.TransactionID == nil {
		t.Error("сверка не сохранена или не связана с корректировкой")
	}

	// Балансовое уравнение сохранилось, служебная строка добавлена движком.
	var debit, credit money.Kop
	for _, line := range result.Transaction.Lines {
		if line.Side == model.SideDebit {
			debit += line.Amount
		} else {
			credit += line.Amount
		}
	}
	if debit != credit || len(result.Transaction.Lines) != 2 {
		t.Errorf("строки корректировки: %+v", result.Transaction.Lines)
	}

	// После корректировки остаток равен факту, расхождение обнулилось.
	balance, err := book.db.Balance(ctx, book.assetID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != money.FromKopecks(45_000) {
		t.Errorf("остаток после корректировки: %s, ожидалось 450,00 ₽", balance)
	}
	discrepancies, err := book.service.Discrepancies(ctx)
	if err != nil {
		t.Fatalf("Discrepancies: %v", err)
	}
	if len(discrepancies) != 1 {
		t.Fatalf("расхождений: %d, ожидалось 1", len(discrepancies))
	}
	if discrepancies[0].Difference != 0 {
		t.Errorf("расхождение после корректировки: %s", discrepancies[0].Difference)
	}
	if discrepancies[0].Path == "" || !discrepancies[0].Adjustable {
		t.Errorf("неполное описание расхождения: %+v", discrepancies[0])
	}
}

// TestASL_9_ReconcileWithoutAdjustment проверяет расчёт расхождения без правки.
func TestASL_9_ReconcileWithoutAdjustment(t *testing.T) {
	book := newTestBook(t)
	ctx := context.Background()

	result, err := book.service.Check(ctx, Request{
		AccountID: book.assetID,
		Fact:      money.FromKopecks(48_000),
		Date:      time.Now().UTC(),
		ActorID:   book.actorID,
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Difference != money.FromKopecks(-2_000) || result.Transaction != nil {
		t.Errorf("расхождение %s или создана корректировка %v", result.Difference, result.Transaction)
	}

	transactions, err := book.db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("проводок в книге %d, ожидалась 1 (только доход)", len(transactions))
	}

	// Расхождение показывается до подтверждения.
	discrepancies, err := book.service.Discrepancies(ctx)
	if err != nil {
		t.Fatalf("Discrepancies: %v", err)
	}
	if len(discrepancies) != 1 || discrepancies[0].Difference != money.FromKopecks(-2_000) {
		t.Errorf("расхождения: %+v", discrepancies)
	}

	// История сверок хранится.
	history, err := book.db.ReconciliationHistory(ctx, book.assetID)
	if err != nil {
		t.Fatalf("ReconciliationHistory: %v", err)
	}
	if len(history) != 1 || history[0].Fact != money.FromKopecks(48_000) {
		t.Errorf("история сверок: %+v", history)
	}
}

// TestASL_9_ReconcileNotAdjustable проверяет отказ для счёта дохода.
func TestASL_9_ReconcileNotAdjustable(t *testing.T) {
	book := newTestBook(t)
	ctx := context.Background()

	_, err := book.service.Check(ctx, Request{
		AccountID: book.incomeID,
		Fact:      0,
		Date:      time.Now().UTC(),
		ActorID:   book.actorID,
		Generate:  true,
		UUID:      "reconcile-bad",
	})
	if !errors.Is(err, ErrNotAdjustable) {
		t.Fatalf("ожидалась ошибка ErrNotAdjustable, получено %v", err)
	}
}
