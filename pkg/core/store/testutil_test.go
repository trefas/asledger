package store

import (
	"context"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// mustCreateUser создаёт пользователя и завершает тест при ошибке.
func mustCreateUser(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	user, err := db.CreateUser(context.Background(), name, "sha256:тест")
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", name, err)
	}
	return user.ID
}

// mustCreateAccount создаёт счёт и завершает тест при ошибке.
func mustCreateAccount(t *testing.T, db *DB, name string, typ model.AccountType, parent *int64) *model.Account {
	t.Helper()
	acc, err := db.CreateAccount(context.Background(), AccountInput{
		Name: name, Type: typ, ParentID: parent,
	})
	if err != nil {
		t.Fatalf("CreateAccount(%q): %v", name, err)
	}
	return acc
}

// mustInsertTransaction сохраняет проводку и завершает тест при ошибке.
func mustInsertTransaction(t *testing.T, db *DB, tr *model.Transaction) int64 {
	t.Helper()
	id, err := db.InsertTransaction(context.Background(), tr)
	if err != nil {
		t.Fatalf("InsertTransaction(%s): %v", tr.UUID, err)
	}
	return id
}
