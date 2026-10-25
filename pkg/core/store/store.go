// Package store реализует доступ к книге asledger в SQLite/SQLCipher.
//
// Книга открывается в режиме WAL с включёнными внешними ключами и
// busy_timeout для всех подключений. Демон является единственной точкой
// записи, поэтому пул соединений ограничен.
//
// Незашифрованная книга открывается без KeyProvider — это режим совместимости
// с v1.2.0 и миграции. В v1.3.0 рабочие команды обязаны передавать ключ.
package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	// Драйвер SQLite с SQLCipher. Требует cgo; сборка описана в
	// docs/security.md.
	_ "github.com/mutecomm/go-sqlcipher/v4"
)

// Параметры подключения.
const (
	driverName = "sqlite3"
	// busyTimeoutMS — время ожидания снятия блокировки БД, мс.
	busyTimeoutMS = 5000
	// maxOpenConns — ограничение пула соединений (сервер — единственный писатель).
	maxOpenConns = 4
	// keyLen — длина raw-ключа SQLCipher.
	keyLen = 32
	// sqliteHeader — начало незашифрованной книги SQLite.
	sqliteHeader = "SQLite format 3\x00"
)

// Ошибки открытия книги.
var (
	// ErrKeyRequired — книга зашифрована, но ключ не передан.
	ErrKeyRequired = errors.New("store: книга зашифрована, нужен ключ")
	// ErrKeyNotFound — поставщик ключа не нашёл ключ.
	ErrKeyNotFound = errors.New("store: ключ не найден")
	// ErrWrongKey — ключ не подошёл или книга повреждена.
	ErrWrongKey = errors.New("store: неверный ключ или повреждённая книга")
	// ErrCorrupt — книга повреждена.
	ErrCorrupt = errors.New("store: книга повреждена")
	// ErrNotEncrypted — передан ключ, но книга не зашифрована.
	ErrNotEncrypted = errors.New("store: книга не зашифрована")
)

// KeyProvider возвращает raw-ключ SQLCipher.
//
// Поставщик вызывается один раз при открытии книги. Он не должен логировать
// ключ и обязан вернуть копию, которой владеет store.
type KeyProvider interface {
	Key(ctx context.Context) ([]byte, error)
}

// KeyFunc адаптирует функцию к KeyProvider.
type KeyFunc func(ctx context.Context) ([]byte, error)

// Key реализует KeyProvider.
func (f KeyFunc) Key(ctx context.Context) ([]byte, error) { return f(ctx) }

// openOptions — параметры Open.
type openOptions struct {
	provider KeyProvider
	key      []byte
}

// Option настраивает открытие книги.
type Option func(*openOptions)

// WithKeyProvider задаёт источник ключа.
func WithKeyProvider(provider KeyProvider) Option {
	return func(o *openOptions) { o.provider = provider }
}

// WithKey задаёт готовый raw-ключ. Используется тестами и вызывающим кодом,
// который уже получил ключ из системного хранилища. Ключ копируется.
func WithKey(key []byte) Option {
	return func(o *openOptions) {
		o.key = append([]byte(nil), key...)
	}
}

// DB — книга asledger в SQLite.
type DB struct {
	*sql.DB
	path      string
	encrypted bool
	// commitMu сериализует создание коммитов: хэш-цепочка и head_seq должны
	// оставаться согласованными при одновременных push и импортах.
	commitMu sync.Mutex
}

// Open открывает книгу по указанному пути.
//
// Без KeyProvider/WithKey открывается незашифрованная книга (совместимость с
// v1.2.0). С ключом открывается SQLCipher-книга; если файл уже существует и
// не зашифрован, возвращается ErrNotEncrypted. Зашифрованная книга без ключа
// даёт ErrKeyRequired и не изменяется.
//
// Включаются режим WAL, busy_timeout, внешние ключи и synchronous=NORMAL.
func Open(ctx context.Context, path string, opts ...Option) (*DB, error) {
	var options openOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	state, err := inspectBook(path)
	if err != nil {
		return nil, err
	}
	key, err := resolveKey(ctx, &options)
	if err != nil {
		return nil, err
	}
	if len(key) == 0 {
		if state.exists && !state.empty && !state.plaintext {
			return nil, fmt.Errorf("%w: %s", ErrKeyRequired, path)
		}
		return openSQLite(ctx, path, nil, false)
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("store: ключ должен быть %d байт, получено %d", keyLen, len(key))
	}
	if state.exists && !state.empty && state.plaintext {
		return nil, fmt.Errorf("%w: %s", ErrNotEncrypted, path)
	}
	return openSQLite(ctx, path, key, true)
}

// Path возвращает путь к файлу книги.
func (db *DB) Path() string { return db.path }

// Encrypted сообщает, открыта ли книга с ключом.
func (db *DB) Encrypted() bool { return db.encrypted }

// Tx выполняет fn в транзакции. При ошибке транзакция откатывается, при
// успехе — фиксируется.
func (db *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: не удалось начать транзакцию: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: не удалось зафиксировать транзакцию: %w", err)
	}
	return nil
}

// Now возвращает текущее время в unix-секундах для колонок *_at.
func Now() int64 { return time.Now().Unix() }

// fromUnix переводит unix-секунды в time.Time (UTC).
func fromUnix(v int64) time.Time { return time.Unix(v, 0).UTC() }

// toUnix переводит время в unix-секунды; нулевое время даёт 0.
func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// bookState — результат предварительного осмотра файла.
type bookState struct {
	exists    bool
	empty     bool
	plaintext bool
}

// inspectBook читает заголовок книги, не открывая её драйвером.
func inspectBook(path string) (bookState, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return bookState{}, nil
	}
	if err != nil {
		return bookState{}, fmt.Errorf("store: не удалось прочитать книгу %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return bookState{}, fmt.Errorf("store: не удалось получить размер книги %q: %w", path, err)
	}
	if info.Size() == 0 {
		return bookState{exists: true, empty: true}, nil
	}

	header := make([]byte, len(sqliteHeader))
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return bookState{}, fmt.Errorf("store: не удалось прочитать заголовок книги %q: %w", path, err)
	}
	return bookState{
		exists:    true,
		plaintext: n == len(header) && bytes.Equal(header, []byte(sqliteHeader)),
	}, nil
}

// resolveKey получает ключ из опций и возвращает его копию.
func resolveKey(ctx context.Context, options *openOptions) ([]byte, error) {
	if options.provider != nil && options.key != nil {
		return nil, errors.New("store: нельзя одновременно задать KeyProvider и готовый ключ")
	}
	if options.provider != nil {
		key, err := options.provider.Key(ctx)
		if err != nil {
			if errors.Is(err, ErrKeyNotFound) {
				return nil, err
			}
			return nil, fmt.Errorf("store: не удалось получить ключ: %w", err)
		}
		if len(key) == 0 {
			return nil, ErrKeyNotFound
		}
		return append([]byte(nil), key...), nil
	}
	if options.key != nil {
		return append([]byte(nil), options.key...), nil
	}
	return nil, nil
}

// openSQLite открывает книгу драйвером SQLCipher.
func openSQLite(ctx context.Context, path string, key []byte, encrypted bool) (*DB, error) {
	dsn := sqliteDSN(path, key)
	// Ключ больше не нужен: DSN содержит его hex-представление, а исходный
	// срез можно затереть. Строка DSN живёт до открытия соединения.
	for i := range key {
		key[i] = 0
	}

	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось открыть книгу %q: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, classifyOpenError(path, err, encrypted)
	}
	// Ping у SQLCipher не всегда читает страницу: проверяем схему, чтобы
	// неверный ключ проявился сразу.
	var objects int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&objects); err != nil {
		_ = sqlDB.Close()
		return nil, classifyOpenError(path, err, encrypted)
	}
	// Книга и sidecar должны быть доступны только владельцу. На Windows
	// os.Chmod меняет только read-only и не мешает открытию.
	if err := os.Chmod(path, 0o600); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: не удалось выставить права на книгу %q: %w", path, err)
	}
	return &DB{DB: sqlDB, path: path, encrypted: encrypted}, nil
}

// sqliteDSN собирает DSN для драйвера SQLCipher.
//
// Ключ передаётся только через _pragma_key, а не через argv/env. DSN с
// ключом нельзя логировать.
func sqliteDSN(path string, key []byte) string {
	p := path
	if abs, err := filepath.Abs(path); err == nil {
		p = abs
	}
	p = filepath.ToSlash(p)
	if runtime.GOOS == "windows" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	query := url.Values{}
	query.Set("_journal_mode", "WAL")
	query.Set("_busy_timeout", strconv.Itoa(busyTimeoutMS))
	query.Set("_foreign_keys", "on")
	query.Set("_synchronous", "NORMAL")
	if key != nil {
		query.Set("_pragma_key", "x'"+hex.EncodeToString(key)+"'")
		query.Set("_pragma_cipher_page_size", "4096")
	}
	u.RawQuery = query.Encode()
	return u.String()
}

// classifyOpenError переводит ошибки драйвера в понятные ошибки store.
func classifyOpenError(path string, err error, encrypted bool) error {
	msg := strings.ToLower(err.Error())
	if encrypted {
		switch {
		case strings.Contains(msg, "file is not a database"):
			return fmt.Errorf("%w: %s", ErrWrongKey, path)
		case strings.Contains(msg, "malformed"), strings.Contains(msg, "corrupt"):
			return fmt.Errorf("%w: %s: %s", ErrCorrupt, path, err.Error())
		}
	}
	return fmt.Errorf("store: книга %q недоступна: %w", path, err)
}
