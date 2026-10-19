package store

import (
	"context"
	"errors"
	"testing"
)

// TestASL_5_UserCRUD проверяет создание и чтение пользователей семьи.
func TestASL_5_UserCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	created, err := db.CreateUser(ctx, "Андрей", "sha256:андрей")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.ID == 0 || created.Name != "Андрей" {
		t.Fatalf("созданный пользователь: %+v", created)
	}

	byID, err := db.User(ctx, created.ID)
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if byID.Name != "Андрей" || byID.TokenHash != "sha256:андрей" {
		t.Fatalf("пользователь по id: %+v", byID)
	}

	byName, err := db.UserByName(ctx, "Андрей")
	if err != nil {
		t.Fatalf("UserByName: %v", err)
	}
	if byName.ID != created.ID {
		t.Fatalf("UserByName вернул id=%d, ожидался %d", byName.ID, created.ID)
	}

	if _, err := db.CreateUser(ctx, "Андрей", "sha256:дубль"); err == nil {
		t.Fatal("повторное имя пользователя принято")
	}

	if _, err := db.User(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("User(999): err=%v, ожидалась ErrNotFound", err)
	}
}

// TestASL_5_ListUsers проверяет выборку всех пользователей.
func TestASL_5_ListUsers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	mustCreateUser(t, db, "Борис")
	mustCreateUser(t, db, "Анна")

	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("получено %d пользователей, ожидалось 2", len(users))
	}
	if users[0].Name != "Анна" || users[1].Name != "Борис" {
		t.Fatalf("порядок пользователей: %q, %q", users[0].Name, users[1].Name)
	}
}

// TestASL_5_Settings проверяет хранение и перезапись настроек.
func TestASL_5_Settings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, ok, err := db.Setting(ctx, SettingInitialBalances); err != nil || ok {
		t.Fatalf("отсутствующая настройка: ok=%v err=%v", ok, err)
	}

	if err := db.SetSetting(ctx, SettingInitialBalances, "1"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	value, ok, err := db.Setting(ctx, SettingInitialBalances)
	if err != nil || !ok || value != "1" {
		t.Fatalf("Setting: value=%q ok=%v err=%v", value, ok, err)
	}

	if err := db.SetSetting(ctx, SettingInitialBalances, "0"); err != nil {
		t.Fatalf("повторный SetSetting: %v", err)
	}
	value, _, err = db.Setting(ctx, SettingInitialBalances)
	if err != nil || value != "0" {
		t.Fatalf("после перезаписи: value=%q err=%v", value, err)
	}
}
