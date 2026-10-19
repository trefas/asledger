package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestASL_5_UserCRUD проверяет создание и чтение пользователей семьи.
func TestASL_5_UserCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	created, err := db.CreateUser(ctx, "Андрей", "секрет-андрея")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.ID == 0 || created.Name != "Андрей" {
		t.Fatalf("созданный пользователь: %+v", created)
	}
	if created.TokenHash == "секрет-андрея" || !strings.HasPrefix(created.TokenHash, "pbkdf2_sha256$") {
		t.Fatalf("секрет сохранён неверно: %q", created.TokenHash)
	}

	byID, err := db.User(ctx, created.ID)
	if err != nil {
		t.Fatalf("User: %v", err)
	}
	if byID.Name != "Андрей" || byID.TokenHash != created.TokenHash {
		t.Fatalf("пользователь по id: %+v", byID)
	}

	byName, err := db.UserByName(ctx, "Андрей")
	if err != nil {
		t.Fatalf("UserByName: %v", err)
	}
	if byName.ID != created.ID {
		t.Fatalf("UserByName вернул id=%d, ожидался %d", byName.ID, created.ID)
	}

	if _, err := db.CreateUser(ctx, "Андрей", "секрет-дубля"); err == nil {
		t.Fatal("повторное имя пользователя принято")
	}

	if _, err := db.User(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("User(999): err=%v, ожидалась ErrNotFound", err)
	}
}

// TestASL_5_Authenticate проверяет проверку секрета пользователя.
func TestASL_5_Authenticate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.CreateUser(ctx, "Андрей", "верный-секрет"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := db.Authenticate(ctx, "Андрей", "верный-секрет")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user.Name != "Андрей" {
		t.Fatalf("Authenticate вернул пользователя %q", user.Name)
	}

	if _, err := db.Authenticate(ctx, "Андрей", "неверный-секрет"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("неверный секрет: err=%v, ожидалась ErrAuthFailed", err)
	}
	if _, err := db.Authenticate(ctx, "Никого", "секрет"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("неизвестный пользователь: err=%v, ожидалась ErrNotFound", err)
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

	if err := db.DeleteSetting(ctx, SettingInitialBalances); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}
	if _, ok, err := db.Setting(ctx, SettingInitialBalances); err != nil || ok {
		t.Fatalf("после удаления: ok=%v err=%v", ok, err)
	}
	// Повторное удаление отсутствующего ключа не ошибка.
	if err := db.DeleteSetting(ctx, SettingInitialBalances); err != nil {
		t.Fatalf("повторный DeleteSetting: %v", err)
	}
}
