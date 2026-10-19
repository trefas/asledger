package store

import (
	"context"
	"testing"
)

func TestSchemaContainsTables(t *testing.T) {
	db := openTestDB(t)

	want := []string{
		"accounts", "users", "settings", "transactions",
		"transaction_lines", "changelog", "commits", "schema_migrations",
	}
	for _, table := range want {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("таблица %q не найдена: %v", table, err)
		}
	}
}

func TestSchemaContainsIndexes(t *testing.T) {
	db := openTestDB(t)

	want := []string{
		"idx_accounts_name", "idx_accounts_parent",
		"idx_transactions_date", "idx_transactions_uuid", "idx_transactions_actor",
		"idx_lines_tx", "idx_lines_account", "idx_lines_tx_account_side",
		"idx_changelog_entity", "idx_commits_client",
	}
	for _, index := range want {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&name)
		if err != nil {
			t.Errorf("индекс %q не найден: %v", index, err)
		}
	}
}

// TestMigrateIdempotent проверяет, что повторный вызов Migrate ничего не меняет.
func TestMigrateIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("не найдено ни одной миграции")
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("повторный Migrate вернул ошибку: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("не удалось посчитать миграции: %v", err)
	}
	if count != len(migrations) {
		t.Fatalf("применено миграций %d, ожидалось %d", count, len(migrations))
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"начальная миграция", "0001_init.sql", 1},
		{"двузначный номер", "0012_indexes.sql", 12},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseVersion(tc.in)
			if err != nil {
				t.Fatalf("parseVersion(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseVersion(%q) = %d, ожидалось %d", tc.in, got, tc.want)
			}
		})
	}

	if _, err := parseVersion("init.sql"); err == nil {
		t.Fatal("parseVersion без номера версии не вернул ошибку")
	}
}

func TestSplitStatements(t *testing.T) {
	got := splitStatements("-- комментарий\nCREATE TABLE a (id INTEGER);\n\nCREATE INDEX i ON a(id);\n")
	if len(got) != 2 {
		t.Fatalf("получено %d инструкций, ожидалось 2: %#v", len(got), got)
	}
	if got[0] != "CREATE TABLE a (id INTEGER)" {
		t.Fatalf("первая инструкция = %q", got[0])
	}
}

// TestConstraints проверяет ограничения целостности схемы.
func TestConstraints(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Минимальный пользователь и транзакция нужны для проверки внешних ключей.
	mustExec(t, db, `INSERT INTO users(id, name, token_hash) VALUES(1, 'Андрей', 'sha256:нет')`)
	mustExec(t, db, `INSERT INTO accounts(id, name, full_path, type, is_leaf, min_balance, is_archived, is_system, created_at)
		VALUES(1, 'Наличные', 'Активы:Наличные', 'актив', 1, 0, 0, 0, 0)`)
	mustExec(t, db, `INSERT INTO transactions(id, uuid, op_type, date, actor_id, created_at, commit_id)
		VALUES(1, 'uuid-1', 'трата', 0, 1, 0, 0)`)

	tests := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "недопустимый тип счёта",
			sql:  `INSERT INTO accounts(name, full_path, type, is_leaf, min_balance, is_archived, is_system, created_at) VALUES(?, ?, 'вымысел', 1, 0, 0, 0, 0)`,
			args: []any{"Плохой", "Плохой"},
		},
		{
			name: "дубликат пути счёта",
			sql:  `INSERT INTO accounts(name, full_path, type, is_leaf, min_balance, is_archived, is_system, created_at) VALUES(?, ?, 'актив', 1, 0, 0, 0, 0)`,
			args: []any{"Ещё", "Активы:Наличные"},
		},
		{
			name: "недопустимый тип операции",
			sql:  `INSERT INTO transactions(uuid, op_type, date, actor_id, created_at, commit_id) VALUES(?, 'перевод2', 0, 1, 0, 0)`,
			args: []any{"uuid-2"},
		},
		{
			name: "дубликат uuid транзакции",
			sql:  `INSERT INTO transactions(uuid, op_type, date, actor_id, created_at, commit_id) VALUES(?, 'трата', 0, 1, 0, 0)`,
			args: []any{"uuid-1"},
		},
		{
			name: "несуществующий актор",
			sql:  `INSERT INTO transactions(uuid, op_type, date, actor_id, created_at, commit_id) VALUES(?, 'трата', 0, 999, 0, 0)`,
			args: []any{"uuid-3"},
		},
		{
			name: "нулевая сумма строки",
			sql:  `INSERT INTO transaction_lines(transaction_id, account_id, amount_kop, side) VALUES(1, 1, 0, 'DEBIT')`,
		},
		{
			name: "отрицательная сумма строки",
			sql:  `INSERT INTO transaction_lines(transaction_id, account_id, amount_kop, side) VALUES(1, 1, -100, 'DEBIT')`,
		},
		{
			name: "недопустимая сторона",
			sql:  `INSERT INTO transaction_lines(transaction_id, account_id, amount_kop, side) VALUES(1, 1, 100, 'DEBET')`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, tc.sql, tc.args...); err == nil {
				t.Fatalf("вставка прошла, хотя должна была быть отклонена: %s", tc.name)
			}
		})
	}
}

// mustExec выполняет инструкцию и завершает тест при ошибке.
func mustExec(t *testing.T, db *DB, sqlText string, args ...any) {
	t.Helper()
	if _, err := db.Exec(sqlText, args...); err != nil {
		t.Fatalf("не удалось выполнить %q: %v", sqlText, err)
	}
}
