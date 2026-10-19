package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// openTestDB открывает временную книгу с применёнными миграциями.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestOpenUsesWAL(t *testing.T) {
	db := openTestDB(t)
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("не удалось прочитать journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, ожидалось wal", mode)
	}
}

func TestOpenEnablesForeignKeys(t *testing.T) {
	db := openTestDB(t)
	var enabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("не удалось прочитать foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, ожидалось 1", enabled)
	}
}

func TestOpenSetsBusyTimeout(t *testing.T) {
	db := openTestDB(t)
	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("не удалось прочитать busy_timeout: %v", err)
	}
	if timeout != busyTimeoutMS {
		t.Fatalf("busy_timeout = %d, ожидалось %d", timeout, busyTimeoutMS)
	}
}

func TestOpenRejectsUnknownDirectory(t *testing.T) {
	if _, err := Open(context.Background(), filepath.Join(t.TempDir(), "нет-каталога", "book.db")); err == nil {
		t.Fatal("Open с недоступным путём не вернул ошибку")
	}
}

// TestTxRollbackOnError проверяет атомарность: ошибка внутри Tx откатывает всё.
func TestTxRollbackOnError(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	sentinel := errors.New("искусственная ошибка")
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO settings(key, value) VALUES('проверка', '1')`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Tx вернул %v, ожидалась искусственная ошибка", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&count); err != nil {
		t.Fatalf("не удалось посчитать настройки: %v", err)
	}
	if count != 0 {
		t.Fatalf("транзакция не откатилась: записей %d", count)
	}
}

// TestTxCommit проверяет фиксацию успешной транзакции.
func TestTxCommit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES('флаг', '1')`)
		return err
	}); err != nil {
		t.Fatalf("Tx вернул ошибку: %v", err)
	}

	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'флаг'`).Scan(&value); err != nil {
		t.Fatalf("запись не найдена: %v", err)
	}
	if value != "1" {
		t.Fatalf("value = %q, ожидалось 1", value)
	}
}

func TestNowReturnsUnixSeconds(t *testing.T) {
	if Now() < 1_700_000_000 {
		t.Fatal("Now() вернул подозрительно маленькое значение")
	}
}
