package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func keyFor(fill byte) []byte {
	key := make([]byte, keyLen)
	for i := range key {
		key[i] = fill
	}
	return key
}

func openAt(t *testing.T, path string, key []byte) *DB {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, path, WithKey(key))
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate(%s): %v", path, err)
	}
	return db
}

func TestExportEncryptedRekey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	targetPath := filepath.Join(dir, "target.db")
	oldKey := keyFor(0x11)
	newKey := keyFor(0x22)

	source := openAt(t, sourcePath, oldKey)
	if _, err := source.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('перенос', 'данные')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if err := source.ExportEncrypted(ctx, targetPath, newKey); err != nil {
		t.Fatalf("ExportEncrypted: %v", err)
	}
	if err := source.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck source: %v", err)
	}

	target, err := Open(ctx, targetPath, WithKey(newKey))
	if err != nil {
		t.Fatalf("Open target: %v", err)
	}
	defer target.Close()
	if !target.Encrypted() {
		t.Fatal("целевая книга не зашифрована")
	}
	if err := target.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck target: %v", err)
	}
	var value string
	if err := target.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'перенос'`).Scan(&value); err != nil {
		t.Fatalf("чтение target: %v", err)
	}
	if value != "данные" {
		t.Fatalf("значение = %q", value)
	}

	if _, err := Open(ctx, targetPath, WithKey(oldKey)); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("старый ключ открыл target: %v", err)
	}
	if _, err := Open(ctx, sourcePath, WithKey(oldKey)); err != nil {
		t.Fatalf("исходная книга перестала открываться старым ключом: %v", err)
	}
}

func TestExportPlaintextToEncrypted(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "plain.db")
	targetPath := filepath.Join(dir, "encrypted.db")
	key := keyFor(0x33)

	source, err := Open(ctx, sourcePath)
	if err != nil {
		t.Fatalf("Open plaintext: %v", err)
	}
	defer source.Close()
	if err := source.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := source.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('миграция', 'ok')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if err := source.ExportEncrypted(ctx, targetPath, key); err != nil {
		t.Fatalf("ExportEncrypted: %v", err)
	}
	if info, err := os.Stat(targetPath); err != nil {
		t.Fatalf("Stat target: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("права target = %o", info.Mode().Perm())
	}

	target, err := Open(ctx, targetPath, WithKey(key))
	if err != nil {
		t.Fatalf("Open target: %v", err)
	}
	defer target.Close()
	if err := target.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck: %v", err)
	}
	var value string
	if err := target.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'миграция'`).Scan(&value); err != nil {
		t.Fatalf("чтение target: %v", err)
	}
	if value != "ok" {
		t.Fatalf("значение = %q", value)
	}
}

func TestExportEncryptedTargetExists(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	targetPath := filepath.Join(dir, "target.db")
	source := openAt(t, sourcePath, keyFor(0x44))
	if err := os.WriteFile(targetPath, []byte("занято"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := source.ExportEncrypted(ctx, targetPath, keyFor(0x55)); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("ExportEncrypted вернул %v, ожидалась ErrTargetExists", err)
	}
}

func TestIntegrityCheckOK(t *testing.T) {
	ctx := context.Background()
	db := openAt(t, filepath.Join(t.TempDir(), "book.db"), keyFor(0x66))
	if err := db.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck: %v", err)
	}
}
