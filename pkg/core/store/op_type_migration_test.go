package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// TestASL_17_InterestOpTypeMigration проверяет миграцию 0008: таблица
// transactions пересоздаётся с новым типом операции без потери данных и
// внешних ключей.
func TestASL_17_InterestOpTypeMigration(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "v7.db"), WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migrations {
		if m.version > 7 {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			t.Fatalf("миграция %s: %v", m.name, err)
		}
	}

	userID := mustCreateUser(t, db, "Андрей")
	wallet := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)
	credit := mustCreateAccount(t, db, "Кредит", model.TypeLiability, nil)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO credit_settings(account_id, rate_ppm, created_at, updated_at)
		 VALUES(?, 150000, 1, 1)`, credit.ID); err != nil {
		t.Fatalf("credit_settings: %v", err)
	}

	// Две старые проводки: вторая сторнирует первую.
	tx1 := insertLegacyTransaction(t, db, userID, wallet.ID, "tx-old-1", "трата", nil)
	tx2 := insertLegacyTransaction(t, db, userID, wallet.ID, "tx-old-2", "корректировка", &tx1)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
		                              status, transaction_id, created_at, updated_at)
		 VALUES('accrual-old', ?, 1700000000, 'credit_interest', 100, 'accrued', ?, 1, 1)`,
		credit.ID, tx1); err != nil {
		t.Fatalf("interest_accruals: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var version int
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("версия схемы: %v", err)
	}
	if version != 8 {
		t.Fatalf("версия схемы = %d, ожидалась 8", version)
	}

	// Проводки и самоссылка сторно сохранены.
	var (
		opType   string
		reverses any
	)
	if err := db.QueryRowContext(ctx,
		`SELECT op_type, reverses_id FROM transactions WHERE id = ?`, tx2).Scan(&opType, &reverses); err != nil {
		t.Fatalf("чтение проводки: %v", err)
	}
	if opType != "корректировка" {
		t.Fatalf("op_type = %q", opType)
	}
	if reverses == nil || reverses.(int64) != tx1 {
		t.Fatalf("reverses_id = %v, ожидалось %d", reverses, tx1)
	}

	// Индексы пересозданы.
	for _, index := range []string{
		"idx_transactions_date", "idx_transactions_uuid",
		"idx_transactions_actor", "idx_transactions_source_hash",
	} {
		var name string
		if err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&name); err != nil {
			t.Fatalf("индекс %q не найден: %v", index, err)
		}
	}

	// Новый тип операции принимается, неизвестный — нет.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO transactions(uuid, op_type, date, actor_id, created_at)
		 VALUES('tx-interest', 'начисление_процентов', 1700000000, ?, 1700000000)`,
		userID); err != nil {
		t.Fatalf("новый тип операции отклонён: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO transactions(uuid, op_type, date, actor_id, created_at)
		 VALUES('tx-unknown', 'вымысел', 1700000000, ?, 1700000000)`,
		userID); err == nil {
		t.Fatal("неизвестный тип операции принят")
	}

	// Внешние ключи дочерних таблиц указывают на новую transactions.
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("PRAGMA foreign_key_check нашёл нарушение")
	}

	// Удаление проводки каскадно чистит строки и обнуляет ссылку начисления.
	if _, err := db.ExecContext(ctx, `DELETE FROM transactions WHERE id = ?`, tx2); err != nil {
		t.Fatalf("удаление сторно: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM transactions WHERE id = ?`, tx1); err != nil {
		t.Fatalf("удаление проводки: %v", err)
	}
	var linked any
	if err := db.QueryRowContext(ctx,
		`SELECT transaction_id FROM interest_accruals WHERE uuid = 'accrual-old'`).Scan(&linked); err != nil {
		t.Fatalf("чтение начисления: %v", err)
	}
	if linked != nil {
		t.Fatalf("transaction_id = %v, ожидался NULL", linked)
	}
	var lines int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transaction_lines`).Scan(&lines); err != nil {
		t.Fatalf("подсчёт строк: %v", err)
	}
	if lines != 0 {
		t.Fatalf("строки проводок не удалены каскадом: %d", lines)
	}
}

// insertLegacyTransaction вставляет проводку со строкой и возвращает её id.
func insertLegacyTransaction(t *testing.T, db *DB, actorID, accountID int64,
	uuid, opType string, reverses *int64) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO transactions(uuid, op_type, date, actor_id, reverses_id, created_at)
		 VALUES(?, ?, 1700000000, ?, ?, 1700000000)`,
		uuid, opType, actorID, reverses)
	if err != nil {
		t.Fatalf("вставка проводки %s: %v", uuid, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("id проводки %s: %v", uuid, err)
	}
	if _, err := db.Exec(
		`INSERT INTO transaction_lines(transaction_id, account_id, amount_kop, side)
		 VALUES(?, ?, 100, 'CREDIT')`, id, accountID); err != nil {
		t.Fatalf("строка проводки %s: %v", uuid, err)
	}
	return id
}
