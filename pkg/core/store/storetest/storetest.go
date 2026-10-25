// Package storetest содержит общие помощники для тестов книги.
//
// Пакет используется только тестами: он открывает зашифрованную книгу с
// фиксированным ключом и применяет миграции. В рабочем коде ключ выводится из
// пароля и не хранится в исходниках.
package storetest

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// Key возвращает копию тестового raw-ключа SQLCipher.
func Key() []byte {
	return bytes.Repeat([]byte{0x5a}, 32)
}

// Open открывает временную зашифрованную книгу и применяет миграции.
//
// Книга закрывается автоматически после завершения теста.
func Open(tb testing.TB, path string) *store.DB {
	tb.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, path, store.WithKey(Key()))
	if err != nil {
		tb.Fatalf("store.Open: %v", err)
	}
	tb.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		tb.Fatalf("store.Migrate: %v", err)
	}
	return db
}

// OpenTemp открывает книгу во временном каталоге теста.
func OpenTemp(tb testing.TB) *store.DB {
	tb.Helper()
	return Open(tb, filepath.Join(tb.TempDir(), "book.db"))
}
