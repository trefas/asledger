package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/trefas/asledger/internal/key"
	"github.com/trefas/asledger/pkg/core/store"
)

func createPlaintextBook(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open plaintext: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('тест', 'до миграции')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func createEncryptedBook(t *testing.T, dbPath, passwordPath string) {
	t.Helper()
	provider, err := key.NewProvider(dbPath, key.Source{PasswordFile: passwordPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := context.Background()
	rawKey, err := provider.Key(ctx)
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	defer key.Zero(rawKey)
	db, err := store.Open(ctx, dbPath, store.WithKey(rawKey))
	if err != nil {
		t.Fatalf("Open encrypted: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('тест', 'до смены пароля')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func openWithPassword(t *testing.T, dbPath, passwordPath string) *store.DB {
	t.Helper()
	provider, err := key.NewProvider(dbPath, key.Source{PasswordFile: passwordPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	db, err := store.Open(context.Background(), dbPath, store.WithKeyProvider(provider))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigrateEncrypt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	backupPath := filepath.Join(dir, "backup.db")
	writeFile(t, passwordPath, "новый пароль\n")
	createPlaintextBook(t, dbPath)

	if code := run([]string{
		"migrate-encrypt", "-db", dbPath,
		"-password-file", passwordPath,
		"-backup", backupPath,
	}); code != 0 {
		t.Fatalf("migrate-encrypt: код %d", code)
	}
	if _, err := os.Stat(key.ParamsPath(dbPath)); err != nil {
		t.Fatalf("sidecar не создан: %v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("резервная копия не создана: %v", err)
	}

	if _, err := store.Open(context.Background(), dbPath); !errors.Is(err, store.ErrKeyRequired) {
		t.Fatalf("plaintext Open вернул %v, ожидалась ErrKeyRequired", err)
	}
	db := openWithPassword(t, dbPath, passwordPath)
	if !db.Encrypted() {
		t.Fatal("книга после миграции не зашифрована")
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'тест'`).Scan(&value); err != nil {
		t.Fatalf("чтение после миграции: %v", err)
	}
	if value != "до миграции" {
		t.Fatalf("значение = %q", value)
	}
	backup := openWithPassword(t, backupPath, passwordPath)
	var backupValue string
	if err := backup.QueryRow(`SELECT value FROM settings WHERE key = 'тест'`).Scan(&backupValue); err != nil {
		t.Fatalf("чтение резервной копии: %v", err)
	}
	if backupValue != value {
		t.Fatalf("резервная копия: значение = %q", backupValue)
	}
}

func TestMigrateEncryptMissingBook(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing.db")
	passwordPath := filepath.Join(dir, "password")
	writeFile(t, passwordPath, "пароль\n")

	if code := run([]string{
		"migrate-encrypt", "-db", dbPath, "-password-file", passwordPath,
	}); code != 1 {
		t.Fatalf("migrate-encrypt без книги: код %d, ожидался 1", code)
	}
	if _, err := os.Stat(key.ParamsPath(dbPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sidecar создан для отсутствующей книги: %v", err)
	}
}

func TestMigrateEncryptDryRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	writeFile(t, passwordPath, "пароль\n")
	createPlaintextBook(t, dbPath)

	if code := run([]string{
		"migrate-encrypt", "-db", dbPath, "-password-file", passwordPath, "-dry-run",
	}); code != 0 {
		t.Fatalf("dry-run: код %d", code)
	}
	if _, err := os.Stat(key.ParamsPath(dbPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run создал sidecar: %v", err)
	}
	if _, err := store.Open(context.Background(), dbPath); err != nil {
		t.Fatalf("dry-run изменил книгу: %v", err)
	}
}

func TestMigrateEncryptAlreadyEncrypted(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	writeFile(t, passwordPath, "пароль\n")
	createEncryptedBook(t, dbPath, passwordPath)

	if code := run([]string{
		"migrate-encrypt", "-db", dbPath, "-password-file", passwordPath,
	}); code != 1 {
		t.Fatalf("migrate-encrypt на зашифрованной книге: код %d, ожидался 1", code)
	}
}

func TestPasswd(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	oldPassword := filepath.Join(dir, "old-password")
	newPassword := filepath.Join(dir, "new-password")
	backupPath := filepath.Join(dir, "old-book.db")
	writeFile(t, oldPassword, "старый пароль\n")
	writeFile(t, newPassword, "новый пароль\n")
	createEncryptedBook(t, dbPath, oldPassword)

	if code := run([]string{
		"passwd", "-db", dbPath,
		"-password-file", oldPassword,
		"-new-password-file", newPassword,
		"-backup", backupPath,
	}); code != 0 {
		t.Fatalf("passwd: код %d", code)
	}

	// Старый пароль больше не открывает книгу.
	oldProvider, err := key.NewProvider(dbPath, key.Source{PasswordFile: oldPassword})
	if err != nil {
		t.Fatalf("NewProvider old: %v", err)
	}
	if _, err := store.Open(context.Background(), dbPath, store.WithKeyProvider(oldProvider)); !errors.Is(err, store.ErrWrongKey) {
		t.Fatalf("старый пароль открыл книгу: %v", err)
	}

	db := openWithPassword(t, dbPath, newPassword)
	if !db.Encrypted() {
		t.Fatal("книга после смены пароля не зашифрована")
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'тест'`).Scan(&value); err != nil {
		t.Fatalf("чтение после смены пароля: %v", err)
	}
	if value != "до смены пароля" {
		t.Fatalf("значение = %q", value)
	}

	// Резервная копия открывается старым паролем и содержит данные.
	backup := openWithPassword(t, backupPath, oldPassword)
	var backupValue string
	if err := backup.QueryRow(`SELECT value FROM settings WHERE key = 'тест'`).Scan(&backupValue); err != nil {
		t.Fatalf("чтение резервной копии: %v", err)
	}
	if backupValue != value {
		t.Fatalf("резервная копия: значение = %q", backupValue)
	}
}

func TestPasswdMissingBook(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing.db")
	passwordPath := filepath.Join(dir, "password")
	newPasswordPath := filepath.Join(dir, "new-password")
	writeFile(t, passwordPath, "старый пароль\n")
	writeFile(t, newPasswordPath, "новый пароль\n")

	if code := run([]string{
		"passwd", "-db", dbPath,
		"-password-file", passwordPath,
		"-new-password-file", newPasswordPath,
	}); code != 1 {
		t.Fatalf("passwd без книги: код %d, ожидался 1", code)
	}
}

func TestPasswdWrongOldPassword(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	oldPassword := filepath.Join(dir, "old-password")
	wrongPassword := filepath.Join(dir, "wrong-password")
	newPassword := filepath.Join(dir, "new-password")
	writeFile(t, oldPassword, "старый пароль\n")
	writeFile(t, wrongPassword, "неверный пароль\n")
	writeFile(t, newPassword, "новый пароль\n")
	createEncryptedBook(t, dbPath, oldPassword)

	if code := run([]string{
		"passwd", "-db", dbPath,
		"-password-file", wrongPassword,
		"-new-password-file", newPassword,
	}); code != 1 {
		t.Fatalf("passwd с неверным паролем: код %d, ожидался 1", code)
	}
	// Исходная книга остаётся доступной старым паролем.
	db := openWithPassword(t, dbPath, oldPassword)
	var value string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'тест'`).Scan(&value); err != nil {
		t.Fatalf("книга повреждена после неверного пароля: %v", err)
	}
}

func TestBackupEncrypted(t *testing.T) {
	t.Setenv("ASLEDGER_ALLOW_PLAINTEXT", "")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	backupPath := filepath.Join(dir, "backup.db")
	writeFile(t, passwordPath, "пароль\n")
	createEncryptedBook(t, dbPath, passwordPath)

	if code := run([]string{
		"backup", "-db", dbPath, "-password-file", passwordPath, "-out", backupPath,
	}); code != 0 {
		t.Fatalf("backup: код %d", code)
	}
	if _, err := os.Stat(key.ParamsPath(backupPath)); err != nil {
		t.Fatalf("sidecar резервной копии не создан: %v", err)
	}
	backup := openWithPassword(t, backupPath, passwordPath)
	if !backup.Encrypted() {
		t.Fatal("резервная копия не зашифрована")
	}
	var value string
	if err := backup.QueryRow(`SELECT value FROM settings WHERE key = 'тест'`).Scan(&value); err != nil {
		t.Fatalf("чтение резервной копии: %v", err)
	}
	if value != "до смены пароля" {
		t.Fatalf("значение = %q", value)
	}
}
