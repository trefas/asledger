// Package store реализует доступ к книге asledger в SQLite.
//
// Книга открывается в режиме WAL с включёнными внешними ключами и
// busy_timeout для всех подключений. Демон является единственной точкой
// записи, поэтому пул соединений ограничен.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	// Драйвер SQLite на чистом Go — не требует cgo и одинаково работает
	// на Linux и Windows.
	_ "modernc.org/sqlite"
)

// Параметры подключения.
const (
	driverName = "sqlite"
	// busyTimeoutMS — время ожидания снятия блокировки БД, мс.
	busyTimeoutMS = 5000
	// maxOpenConns — ограничение пула соединений (сервер — единственный писатель).
	maxOpenConns = 4
)

// DB — книга asledger в SQLite.
type DB struct {
	*sql.DB
	path string
	// commitMu сериализует создание коммитов: хэш-цепочка и head_seq должны
	// оставаться согласованными при одновременных push и импортах.
	commitMu sync.Mutex
}

// Open открывает книгу по указанному пути.
//
// Включаются режим WAL, busy_timeout, внешние ключи и synchronous=NORMAL.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)",
		path, busyTimeoutMS,
	)
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось открыть книгу %q: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: книга %q недоступна: %w", path, err)
	}
	return &DB{DB: sqlDB, path: path}, nil
}

// Path возвращает путь к файлу книги.
func (db *DB) Path() string { return db.path }

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
