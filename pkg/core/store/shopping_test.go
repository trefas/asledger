package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_15_ShoppingMigration проверяет применение миграции списков покупок.
func TestASL_15_ShoppingMigration(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	tables := []string{
		"shopping_lists", "shopping_items",
		"shopping_list_templates", "shopping_template_items",
	}
	for _, table := range tables {
		var name string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("таблица %q не создана: %v", table, err)
		}
	}

	// Повторная миграция идемпотентна.
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("повторная Migrate: %v", err)
	}
}

// TestASL_15_ShoppingMigrationFromV133 проверяет инкрементальную миграцию
// книги версии 1.3.3: схема 0001–0005 уже есть, 0006 добавляется Migrate.
func TestASL_15_ShoppingMigrationFromV133(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "v133.db"), WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Воспроизводим книгу v1.3.3: применяем 0001–0005 вручную, не вызывая
	// Migrate, иначе он применил бы и 0006.
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migrations {
		if m.version > 5 {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			t.Fatalf("миграция %s: %v", m.name, err)
		}
	}

	// В книге v1.3.3 есть данные, которые миграция обязана сохранить.
	userID := mustCreateUser(t, db, "Андрей")
	account := mustCreateAccount(t, db, "Продукты", model.TypeExpense, nil)

	var version int
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("версия схемы: %v", err)
	}
	if version != 5 {
		t.Fatalf("версия схемы = %d, ожидалась 5", version)
	}
	if tableExists(t, db, "shopping_lists") {
		t.Fatal("таблица shopping_lists уже существует до миграции 0006")
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{
		"shopping_lists", "shopping_items",
		"shopping_list_templates", "shopping_template_items",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("таблица %q не создана миграцией 0006", table)
		}
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("версия схемы после миграции: %v", err)
	}
	if version != 6 {
		t.Fatalf("версия схемы = %d, ожидалась 6", version)
	}

	// Данные книги не пострадали, списки создаются на существующей книге.
	var userName string
	if err := db.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, userID).Scan(&userName); err != nil {
		t.Fatalf("пользователь потерян: %v", err)
	}
	if userName != "Андрей" {
		t.Fatalf("имя пользователя = %q", userName)
	}
	list, err := db.CreateShoppingList(ctx, ShoppingListInput{
		Name: "После обновления", OwnerID: userID,
	})
	if err != nil {
		t.Fatalf("CreateShoppingList после миграции: %v", err)
	}
	if _, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Хлеб", AccountID: &account.ID,
	}); err != nil {
		t.Fatalf("AddShoppingItem после миграции: %v", err)
	}
}

// tableExists сообщает, существует ли таблица в книге.
func tableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("проверка таблицы %q: %v", name, err)
	}
	return true
}

// TestASL_15_ShoppingConstraints проверяет ограничения схемы и store.
func TestASL_15_ShoppingConstraints(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")
	account := mustCreateAccount(t, db, "Продукты", model.TypeExpense, nil)

	list, err := db.CreateShoppingList(ctx, ShoppingListInput{
		Name: "Продукты", OwnerID: userID,
	})
	if err != nil {
		t.Fatalf("CreateShoppingList: %v", err)
	}

	// Отрицательная цена отклоняется store.
	if _, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Хлеб", AccountID: &account.ID, ActualPrice: money.FromKopecks(-1),
	}); err == nil {
		t.Fatal("ожидалась ошибка отрицательной цены")
	}

	// Отрицательная цена отклоняется и на уровне SQL.
	_, err = db.ExecContext(ctx,
		`INSERT INTO shopping_items(uuid, list_id, name, actual_price_kop, created_at, updated_at)
		 VALUES('bad-price', ?, 'Хлеб', -1, 0, 0)`, list.ID)
	if err == nil {
		t.Fatal("ожидалось нарушение CHECK actual_price_kop >= 0")
	}

	// Статья расходов должна быть листовым счётом типа «расход».
	asset := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)
	_, err = db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Хлеб", AccountID: &asset.ID,
	})
	if err == nil {
		t.Fatal("ожидалась ошибка статьи расходов")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Code != CodeShoppingAccountInvalid {
		t.Fatalf("код ошибки = %v, ожидался %s", err, CodeShoppingAccountInvalid)
	}
}

// TestASL_15_ShoppingCRUD проверяет CRUD, версии и агрегаты списка.
func TestASL_15_ShoppingCRUD(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")
	secondUserID := mustCreateUser(t, db, "Мария")
	account := mustCreateAccount(t, db, "Продукты", model.TypeExpense, nil)

	list, err := db.CreateShoppingList(ctx, ShoppingListInput{
		Name:        "На выходные",
		PlannedDate: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		OwnerID:     userID,
	})
	if err != nil {
		t.Fatalf("CreateShoppingList: %v", err)
	}
	if list.Status != model.ShoppingListActive {
		t.Fatalf("статус = %q", list.Status)
	}

	first, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Хлеб", AccountID: &account.ID, AssigneeID: &userID,
		Priority: model.ShoppingPriorityHigh, PlannedPrice: money.FromKopecks(5000),
	})
	if err != nil {
		t.Fatalf("AddShoppingItem: %v", err)
	}
	second, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Молоко", AccountID: &account.ID, AssigneeID: &secondUserID,
	})
	if err != nil {
		t.Fatalf("AddShoppingItem: %v", err)
	}
	if first.Position != 1 || second.Position != 2 {
		t.Fatalf("порядок позиций = %d, %d", first.Position, second.Position)
	}

	// Подтверждение цены и отметка «в корзине».
	actual := money.FromKopecks(6200)
	confirmed, err := db.UpdateShoppingItem(ctx, first.ID, ShoppingItemPatch{
		Version:     first.Version,
		ActualPrice: &actual,
		Status:      statusPtr(model.ShoppingItemConfirmed),
	})
	if err != nil {
		t.Fatalf("UpdateShoppingItem: %v", err)
	}
	if confirmed.Version != first.Version+1 {
		t.Fatalf("версия = %d, ожидалась %d", confirmed.Version, first.Version+1)
	}

	// Устаревшая версия даёт конфликт.
	_, err = db.UpdateShoppingItem(ctx, first.ID, ShoppingItemPatch{
		Version: first.Version, Comment: strPtr("устаревшая версия"),
	})
	if err == nil {
		t.Fatal("ожидался конфликт версий")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Code != CodeShoppingVersionConflict {
		t.Fatalf("код ошибки = %v, ожидался %s", err, CodeShoppingVersionConflict)
	}

	// Второй товар — «в корзине», без цены.
	inCart := true
	if _, err := db.UpdateShoppingItem(ctx, second.ID, ShoppingItemPatch{
		Version: second.Version, InCart: &inCart,
	}); err != nil {
		t.Fatalf("UpdateShoppingItem in_cart: %v", err)
	}

	got, err := db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList: %v", err)
	}
	if got.ItemCount != 2 || got.ConfirmedCount != 2 {
		t.Fatalf("агрегаты = %d/%d, ожидались 2/2", got.ItemCount, got.ConfirmedCount)
	}
	if got.ConfirmedTotal != money.FromKopecks(6200) {
		t.Fatalf("сумма подтверждённых = %s", got.ConfirmedTotal)
	}
	if len(got.Items) != 2 {
		t.Fatalf("позиций = %d", len(got.Items))
	}

	// Купленную позицию нельзя удалить.
	purchased, err := db.UpdateShoppingItem(ctx, first.ID, ShoppingItemPatch{
		Version: confirmed.Version, Status: statusPtr(model.ShoppingItemPurchased),
	})
	if err != nil {
		t.Fatalf("UpdateShoppingItem purchased: %v", err)
	}
	if err := db.DeleteShoppingItem(ctx, purchased.ID); err == nil {
		t.Fatal("ожидалась защита купленной позиции")
	}

	// Архивация сохраняет позиции.
	if err := db.ArchiveShoppingList(ctx, list.ID); err != nil {
		t.Fatalf("ArchiveShoppingList: %v", err)
	}
	archived, err := db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList archived: %v", err)
	}
	if archived.Status != model.ShoppingListArchived || archived.ArchivedAt == nil {
		t.Fatalf("список не заархивирован: %+v", archived)
	}
	if len(archived.Items) != 2 {
		t.Fatalf("позиции потеряны при архивации: %d", len(archived.Items))
	}

	// В журнале изменений есть новые сущности.
	entries, err := db.Changelog(ctx, 0)
	if err != nil {
		t.Fatalf("Changelog: %v", err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Entity] = true
	}
	if !seen[model.EntityShoppingList] || !seen[model.EntityShoppingItem] {
		t.Fatalf("журнал изменений не содержит списки: %+v", seen)
	}
}

// TestASL_15_ShoppingTemplates проверяет шаблоны и создание списка из шаблона.
func TestASL_15_ShoppingTemplates(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")
	account := mustCreateAccount(t, db, "Продукты", model.TypeExpense, nil)

	tpl, err := db.CreateShoppingTemplate(ctx, "Еженедельные продукты", []ShoppingTemplateItemInput{
		{Name: "Хлеб", AccountID: &account.ID, Priority: model.ShoppingPriorityHigh},
		{Name: "Молоко", AccountID: &account.ID},
	})
	if err != nil {
		t.Fatalf("CreateShoppingTemplate: %v", err)
	}
	if len(tpl.Items) != 2 {
		t.Fatalf("позиций шаблона = %d", len(tpl.Items))
	}

	list, err := db.CreateShoppingListFromTemplate(ctx, tpl.ID, "Неделя 44", userID, time.Time{})
	if err != nil {
		t.Fatalf("CreateShoppingListFromTemplate: %v", err)
	}
	if len(list.Items) != 2 {
		t.Fatalf("позиций списка = %d", len(list.Items))
	}
	if list.Items[0].ActualPrice != 0 || list.Items[0].InCart {
		t.Fatalf("в список попали цены/отметки шаблона: %+v", list.Items[0])
	}

	// Повторное создание даёт независимый список.
	second, err := db.CreateShoppingListFromTemplate(ctx, tpl.ID, "Неделя 45", userID, time.Time{})
	if err != nil {
		t.Fatalf("CreateShoppingListFromTemplate second: %v", err)
	}
	if second.ID == list.ID {
		t.Fatal("создан тот же список")
	}

	// Сохранение списка как шаблона.
	saved, err := db.SaveShoppingListAsTemplate(ctx, list.ID, "Шаблон из списка")
	if err != nil {
		t.Fatalf("SaveShoppingListAsTemplate: %v", err)
	}
	if len(saved.Items) != 2 {
		t.Fatalf("позиций сохранённого шаблона = %d", len(saved.Items))
	}
}

func statusPtr(v model.ShoppingItemStatus) *model.ShoppingItemStatus { return &v }
func strPtr(v string) *string                                        { return &v }
