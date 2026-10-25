package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenBookFromSQLCipher442 проверяет обратную совместимость с книгой,
// созданной драйвером mutecomm/go-sqlcipher v4.4.2 (SQLCipher 4.4.2).
//
// Фикстура зашифрована тестовым ключом testKey и содержит только таблицу
// legacy_marker. После открытия текущий драйвер должен прочитать данные,
// применить миграции, выполнить integrity_check и экспортировать книгу.
func TestOpenBookFromSQLCipher442(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join("testdata", "sqlcipher-4.4.2.db")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", source, err)
	}
	if bytes.HasPrefix(data, []byte(sqliteHeader)) {
		t.Fatal("фикстура не зашифрована: найден заголовок обычного SQLite")
	}

	path := filepath.Join(t.TempDir(), "book.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	db, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var marker string
	if err := db.QueryRowContext(ctx,
		`SELECT value FROM legacy_marker WHERE id = 1`).Scan(&marker); err != nil {
		t.Fatalf("чтение legacy_marker: %v", err)
	}
	if marker != "created-by-sqlcipher-4.4.2" {
		t.Fatalf("legacy_marker = %q, ожидалось created-by-sqlcipher-4.4.2", marker)
	}
	if err := db.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('после-обновления', 'ok')`); err != nil {
		t.Fatalf("INSERT settings: %v", err)
	}

	target := filepath.Join(t.TempDir(), "export.db")
	if err := db.ExportEncrypted(ctx, target, testKey); err != nil {
		t.Fatalf("ExportEncrypted: %v", err)
	}
	exported, err := Open(ctx, target, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open export: %v", err)
	}
	defer func() { _ = exported.Close() }()
	var setting string
	if err := exported.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'после-обновления'`).Scan(&setting); err != nil {
		t.Fatalf("чтение settings из экспорта: %v", err)
	}
	if setting != "ok" {
		t.Fatalf("settings = %q, ожидалось ok", setting)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("повторное Open: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if err := reopened.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'после-обновления'`).Scan(&setting); err != nil {
		t.Fatalf("чтение settings после повторного открытия: %v", err)
	}
	if setting != "ok" {
		t.Fatalf("settings после повторного открытия = %q, ожидалось ok", setting)
	}
}
