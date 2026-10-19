package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// initialTransaction собирает сбалансированную проводку начальных остатков:
// дебет счёта-актива и кредит счёта капитала на одинаковую сумму.
func initialTransaction(uuid string, actor, debitAccount, creditAccount int64) *model.Transaction {
	return &model.Transaction{
		UUID:        uuid,
		OpType:      model.OpInitial,
		Date:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Description: "начальные остатки",
		ActorID:     actor,
		Lines: []model.TransactionLine{
			{AccountID: debitAccount, Amount: money.FromKopecks(10000), Side: model.SideDebit},
			{AccountID: creditAccount, Amount: money.FromKopecks(10000), Side: model.SideCredit},
		},
	}
}

// TestASL_5_InsertTransaction проверяет сохранение проводки со строками.
func TestASL_5_InsertTransaction(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")

	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	id := mustInsertTransaction(t, db, initialTransaction("tx-1", actor, wallet.ID, capital.ID))

	tr, err := db.Transaction(ctx, id)
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if tr.UUID != "tx-1" || tr.OpType != model.OpInitial || tr.ActorID != actor {
		t.Fatalf("заголовок проводки: %+v", tr)
	}
	if len(tr.Lines) != 2 {
		t.Fatalf("строк %d, ожидалось 2", len(tr.Lines))
	}
	if !tr.Date.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("дата проводки: %v", tr.Date)
	}

	// Журнал изменений получил запись о транзакции.
	entries, err := db.Changelog(ctx, 0)
	if err != nil {
		t.Fatalf("Changelog: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Entity == model.EntityTransaction && e.EntityID == id && e.Op == model.OpInsert {
			found = true
		}
	}
	if !found {
		t.Fatalf("в журнале нет записи о транзакции %d: %+v", id, entries)
	}
	head, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	if head == 0 {
		t.Fatal("HeadSeq вернул 0 после записи в журнал")
	}
}

// TestASL_5_InsertTransactionUnbalanced проверяет инвариант ΣDEBIT = ΣCREDIT.
func TestASL_5_InsertTransactionUnbalanced(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")
	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	tr := initialTransaction("tx-unbalanced", actor, wallet.ID, capital.ID)
	tr.Lines[1].Amount = money.FromKopecks(9999) // кредит меньше дебета

	_, err = db.InsertTransaction(ctx, tr)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Code != CodeUnbalanced {
		t.Fatalf("err=%v, ожидалась ValidationError с кодом %s", err, CodeUnbalanced)
	}

	// Несбалансированная проводка не сохранилась.
	if _, err := db.TransactionByUUID(ctx, "tx-unbalanced"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("TransactionByUUID: err=%v, ожидалась ErrNotFound", err)
	}
}

// TestASL_5_InsertTransactionAmounts проверяет положительность сумм и стороны.
func TestASL_5_InsertTransactionAmounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")
	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	cases := []struct {
		name     string
		mutate   func(*model.Transaction)
		wantCode string
	}{
		{
			name:     "нулевая сумма",
			mutate:   func(tr *model.Transaction) { tr.Lines[0].Amount = 0 },
			wantCode: CodeAmountNotPositive,
		},
		{
			name:     "отрицательная сумма",
			mutate:   func(tr *model.Transaction) { tr.Lines[0].Amount = -1 },
			wantCode: CodeAmountNotPositive,
		},
		{
			name:     "недопустимая сторона",
			mutate:   func(tr *model.Transaction) { tr.Lines[0].Side = "DEBET" },
			wantCode: CodeSideInvalid,
		},
		{
			name:     "нет строк",
			mutate:   func(tr *model.Transaction) { tr.Lines = nil },
			wantCode: CodeNoLines,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := initialTransaction("tx-"+tc.name, actor, wallet.ID, capital.ID)
			tc.mutate(tr)
			_, err := db.InsertTransaction(ctx, tr)
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Code != tc.wantCode {
				t.Fatalf("err=%v, ожидалась ValidationError с кодом %s", err, tc.wantCode)
			}
		})
	}
}

// TestASL_5_InsertTransactionDuplicateUUID проверяет идемпотентность по uuid.
func TestASL_5_InsertTransactionDuplicateUUID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")
	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	mustInsertTransaction(t, db, initialTransaction("tx-dup", actor, wallet.ID, capital.ID))

	_, err = db.InsertTransaction(ctx, initialTransaction("tx-dup", actor, wallet.ID, capital.ID))
	if !errors.Is(err, ErrDuplicateUUID) {
		t.Fatalf("err=%v, ожидалась ErrDuplicateUUID", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE uuid = 'tx-dup'`).Scan(&count); err != nil {
		t.Fatalf("подсчёт транзакций: %v", err)
	}
	if count != 1 {
		t.Fatalf("транзакций с uuid tx-dup: %d, ожидалась 1", count)
	}
}

// TestASL_5_ListTransactions проверяет фильтры выборки транзакций.
func TestASL_5_ListTransactions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")
	other := mustCreateUser(t, db, "Борис")
	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	mustInsertTransaction(t, db, initialTransaction("tx-a", actor, wallet.ID, capital.ID))
	mustInsertTransaction(t, db, initialTransaction("tx-b", other, wallet.ID, capital.ID))

	byActor, err := db.ListTransactions(ctx, TransactionFilter{ActorID: &actor})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(byActor) != 1 || byActor[0].UUID != "tx-a" {
		t.Fatalf("по актору: %+v", byActor)
	}

	byAccount, err := db.ListTransactions(ctx, TransactionFilter{AccountID: &wallet.ID})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(byAccount) != 2 {
		t.Fatalf("по счёту: %d транзакций, ожидалось 2", len(byAccount))
	}

	limited, err := db.ListTransactions(ctx, TransactionFilter{Limit: 1})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("с лимитом: %d транзакций, ожидалась 1", len(limited))
	}
}

// TestASL_5_TransactionByUUID проверяет чтение проводки по uuid.
func TestASL_5_TransactionByUUID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")
	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	id := mustInsertTransaction(t, db, initialTransaction("tx-read", actor, wallet.ID, capital.ID))

	tr, err := db.TransactionByUUID(ctx, "tx-read")
	if err != nil {
		t.Fatalf("TransactionByUUID: %v", err)
	}
	if tr.ID != id || len(tr.Lines) != 2 {
		t.Fatalf("проводка по uuid: id=%d строк=%d", tr.ID, len(tr.Lines))
	}
}
