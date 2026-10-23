package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// testKey — фиксированный ключ тестов. В рабочих сценариях ключ выводится из
// пароля и не хранится в исходниках.
var testKey = bytes.Repeat([]byte{0x42}, keyLen)

// openTestDB открывает временную зашифрованную книгу с миграциями.
func openTestDB(tb testing.TB) *DB {
	tb.Helper()
	return openTestDBWithKey(tb, testKey)
}

// openTestDBWithKey открывает временную книгу с заданным ключом.
func openTestDBWithKey(tb testing.TB, key []byte) *DB {
	tb.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(tb.TempDir(), "test.db"), WithKey(key))
	if err != nil {
		tb.Fatalf("Open: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		tb.Fatalf("Migrate: %v", err)
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

// TestOpenEncryptedRoundTrip проверяет создание, чтение и повторное открытие.
func TestOpenEncryptedRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "book.db")

	db, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !db.Encrypted() {
		t.Fatal("книга открыта без признака шифрования")
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('секрет-маркер', 'финансовые-данные')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	var cipherVersion string
	if err := db.QueryRowContext(ctx, "PRAGMA cipher_version").Scan(&cipherVersion); err != nil {
		t.Fatalf("cipher_version: %v", err)
	}
	if cipherVersion == "" {
		t.Fatal("cipher_version пуст — SQLCipher не активен")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.HasPrefix(raw, []byte(sqliteHeader)) {
		t.Fatal("зашифрованная книга начинается с заголовка обычного SQLite")
	}
	if bytes.Contains(raw, []byte("финансовые-данные")) || bytes.Contains(raw, []byte("секрет-маркер")) {
		t.Fatal("в зашифрованной книге найден plaintext-маркер")
	}

	reopened, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("повторное Open: %v", err)
	}
	defer reopened.Close()
	var value string
	if err := reopened.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = 'секрет-маркер'`).Scan(&value); err != nil {
		t.Fatalf("SELECT после повторного открытия: %v", err)
	}
	if value != "финансовые-данные" {
		t.Fatalf("value = %q", value)
	}
}

// TestOpenEncryptedRequiresKey проверяет предсказуемую ошибку без ключа.
func TestOpenEncryptedRequiresKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "book.db")
	db, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := Open(ctx, path); !errors.Is(err, ErrKeyRequired) {
		t.Fatalf("Open без ключа вернул %v, ожидалась ErrKeyRequired", err)
	}
}

// TestOpenWrongKey проверяет неверный ключ и сохранность книги.
func TestOpenWrongKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "book.db")
	db, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	wrong := bytes.Repeat([]byte{0x24}, keyLen)
	if _, err := Open(ctx, path, WithKey(wrong)); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("Open с неверным ключом вернул %v, ожидалась ErrWrongKey", err)
	}

	good, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open с верным ключом после ошибки: %v", err)
	}
	defer good.Close()
	var count int
	if err := good.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("книга недоступна после неверного ключа: %v", err)
	}
	if count == 0 {
		t.Fatal("после неверного ключа пропали миграции")
	}
}

// TestOpenPlaintextWithKey отвергает ключ для незашифрованной книги.
func TestOpenPlaintextWithKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "book.db")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open plaintext: %v", err)
	}
	if db.Encrypted() {
		t.Fatal("незашифрованная книга помечена как зашифрованная")
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := Open(ctx, path, WithKey(testKey)); !errors.Is(err, ErrNotEncrypted) {
		t.Fatalf("Open с ключом для plaintext вернул %v, ожидалась ErrNotEncrypted", err)
	}
}

// TestOpenKeyNotFound проверяет ошибку поставщика ключа.
func TestOpenKeyNotFound(t *testing.T) {
	_, err := Open(context.Background(), filepath.Join(t.TempDir(), "book.db"),
		WithKeyProvider(KeyFunc(func(context.Context) ([]byte, error) {
			return nil, ErrKeyNotFound
		})))
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Open вернул %v, ожидалась ErrKeyNotFound", err)
	}
}

// TestOpenRejectsShortKey проверяет длину raw-ключа.
func TestOpenRejectsShortKey(t *testing.T) {
	_, err := Open(context.Background(), filepath.Join(t.TempDir(), "book.db"),
		WithKey([]byte("слишком короткий")))
	if err == nil || !strings.Contains(err.Error(), "32 байт") {
		t.Fatalf("Open с коротким ключом вернул %v", err)
	}
}

// TestOpenEmptyFileWithKey создаёт зашифрованную книгу в пустом файле.
func TestOpenEmptyFileWithKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "empty.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	db, err := Open(ctx, path, WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

// TestOpenEncryptedConcurrentReads проверяет пул соединений на зашифрованной
// книге: каждое соединение должно получать ключ и читать данные.
func TestOpenEncryptedConcurrentReads(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES('конкурентность', 'ok')`); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var value string
			if err := db.QueryRowContext(ctx,
				`SELECT value FROM settings WHERE key = 'конкурентность'`).Scan(&value); err != nil {
				errs <- err
				return
			}
			if value != "ok" {
				errs <- errors.New("неверное значение на параллельном чтении")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("параллельное чтение: %v", err)
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
