package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_5_BalanceByNature проверяет расчёт остатка по природе счёта.
func TestASL_5_BalanceByNature(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")

	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	mustInsertTransaction(t, db, initialTransaction("tx-bal", actor, wallet.ID, capital.ID))

	walletBalance, err := db.Balance(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("Balance(актив): %v", err)
	}
	if walletBalance != money.FromKopecks(10000) {
		t.Fatalf("остаток актива = %s, ожидалось 100,00 ₽", walletBalance)
	}

	capitalBalance, err := db.Balance(ctx, capital.ID)
	if err != nil {
		t.Fatalf("Balance(капитал): %v", err)
	}
	if capitalBalance != money.FromKopecks(10000) {
		t.Fatalf("остаток капитала = %s, ожидалось 100,00 ₽", capitalBalance)
	}

	// Остаток никогда не хранится: удаление строк меняет результат свёртки.
	if _, err := db.ExecContext(ctx, `DELETE FROM transaction_lines WHERE transaction_id IS NOT NULL`); err != nil {
		t.Fatalf("удаление строк: %v", err)
	}
	after, err := db.Balance(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("Balance после удаления: %v", err)
	}
	if after != 0 {
		t.Fatalf("остаток после удаления строк = %s, ожидался 0", after)
	}
}

// TestASL_5_BalancesAll проверяет сводную карту остатков всех счетов.
func TestASL_5_BalancesAll(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")

	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	mustInsertTransaction(t, db, initialTransaction("tx-all", actor, wallet.ID, capital.ID))

	balances, err := db.Balances(ctx)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	if balances[wallet.ID] != money.FromKopecks(10000) {
		t.Fatalf("остаток кошелька = %s", balances[wallet.ID])
	}
	if balances[capital.ID] != money.FromKopecks(10000) {
		t.Fatalf("остаток капитала = %s", balances[capital.ID])
	}
	// Служебные счета без проводок присутствуют с нулём.
	debitorka, err := db.AccountByPath(ctx, "Активы:Дебиторка")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if bal, ok := balances[debitorka.ID]; !ok || bal != 0 {
		t.Fatalf("остаток дебиторки = %s (наличие=%v)", bal, ok)
	}
}

// TestASL_5_SubtreeBalance проверяет суммарный остаток поддерева.
func TestASL_5_SubtreeBalance(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")

	wallet := mustCreateAccount(t, db, "Кошелёк", model.TypeAsset, nil)
	card := mustCreateAccount(t, db, "Карта", model.TypeAsset, nil)
	capital, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	mustInsertTransaction(t, db, initialTransaction("tx-1", actor, wallet.ID, capital.ID))
	mustInsertTransaction(t, db, initialTransaction("tx-2", actor, card.ID, capital.ID))

	total, err := db.SubtreeBalance(ctx, "Капитал")
	if err != nil {
		t.Fatalf("SubtreeBalance: %v", err)
	}
	if total != money.FromKopecks(20000) {
		t.Fatalf("остаток капитала = %s, ожидалось 200,00 ₽", total)
	}

	// Пустое поддерево даёт ноль.
	empty, err := db.SubtreeBalance(ctx, "НетТакогоСчёта")
	if err != nil {
		t.Fatalf("SubtreeBalance(пусто): %v", err)
	}
	if empty != 0 {
		t.Fatalf("остаток пустого поддерева = %s, ожидался 0", empty)
	}
}

// BenchmarkBalance фиксирует производительность расчёта остатка на журнале из
// 10 000 проводок (20 000 строк).
func BenchmarkBalance(b *testing.B) {
	db := openTestDB(b)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, "Андрей", "секрет")
	if err != nil {
		b.Fatalf("CreateUser: %v", err)
	}
	asset, err := db.CreateAccount(ctx, AccountInput{Name: "Кошелёк", Type: model.TypeAsset})
	if err != nil {
		b.Fatalf("CreateAccount(актив): %v", err)
	}
	expense, err := db.CreateAccount(ctx, AccountInput{Name: "Продукты", Type: model.TypeExpense})
	if err != nil {
		b.Fatalf("CreateAccount(расход): %v", err)
	}

	const volume = 10_000
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatalf("BeginTx: %v", err)
	}
	for i := range volume {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO transactions(uuid, op_type, date, actor_id, created_at, commit_id)
			 VALUES(?, 'трата', 0, ?, 0, 0)`, fmt.Sprintf("bench-%d", i), user.ID)
		if err != nil {
			b.Fatalf("insert transactions: %v", err)
		}
		txID, err := res.LastInsertId()
		if err != nil {
			b.Fatalf("LastInsertId: %v", err)
		}
		for _, line := range []struct {
			account int64
			side    string
		}{{expense.ID, "DEBIT"}, {asset.ID, "CREDIT"}} {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO transaction_lines(transaction_id, account_id, amount_kop, side)
				 VALUES(?, ?, 100, ?)`, txID, line.account, line.side); err != nil {
				b.Fatalf("insert lines: %v", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("Commit: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := db.Balance(ctx, expense.ID); err != nil {
			b.Fatalf("Balance: %v", err)
		}
	}
}
