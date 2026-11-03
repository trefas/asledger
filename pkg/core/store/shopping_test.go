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

	// Изменение шаблона: название и полная замена базовых позиций.
	newName := "Обновлённый шаблон"
	replacement := []ShoppingTemplateItemInput{
		{Name: "Сыр", AccountID: &account.ID, Priority: model.ShoppingPriorityLow},
	}
	updated, err := db.UpdateShoppingTemplate(ctx, saved.ID, ShoppingTemplatePatch{
		Name: &newName, Items: &replacement,
	})
	if err != nil {
		t.Fatalf("UpdateShoppingTemplate: %v", err)
	}
	if updated.Name != newName || len(updated.Items) != 1 || updated.Items[0].Name != "Сыр" {
		t.Fatalf("обновлённый шаблон: %+v", updated)
	}

	// Пустое название списка берётся из шаблона.
	unnamed, err := db.CreateShoppingListFromTemplate(ctx, saved.ID, "  ", userID, time.Time{})
	if err != nil {
		t.Fatalf("CreateShoppingListFromTemplate без имени: %v", err)
	}
	if unnamed.Name != newName || len(unnamed.Items) != 1 {
		t.Fatalf("список без имени: %+v", unnamed)
	}

	// Несуществующий шаблон — ErrNotFound.
	if _, err := db.UpdateShoppingTemplate(ctx, 999999, ShoppingTemplatePatch{Name: &newName}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateShoppingTemplate(нет шаблона) = %v, ожидался ErrNotFound", err)
	}
}

// TestASL_15_ShoppingAccountSuggest проверяет подсказку статей расходов
// с учётом истории трат.
func TestASL_15_ShoppingAccountSuggest(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")
	products := mustCreateAccount(t, db, "Продукты", model.TypeExpense, nil)
	household := mustCreateAccount(t, db, "Хозяйство", model.TypeExpense, nil)
	asset := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)

	// Продукты уже использовались в тратах — подсказка поднимает их выше.
	mustInsertTransaction(t, db, &model.Transaction{
		UUID:    "11111111-1111-4111-8111-111111111111",
		OpType:  model.OpExpense,
		Date:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		ActorID: userID,
		Lines: []model.TransactionLine{
			{AccountID: products.ID, Amount: money.FromKopecks(15000), Side: model.SideDebit},
			{AccountID: asset.ID, Amount: money.FromKopecks(15000), Side: model.SideCredit},
		},
	})

	suggestions, err := db.SuggestShoppingAccounts(ctx, "", 10)
	if err != nil {
		t.Fatalf("SuggestShoppingAccounts: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("подсказок = %d, ожидалось 2: %+v", len(suggestions), suggestions)
	}
	if suggestions[0].ID != products.ID || suggestions[0].Used != 1 {
		t.Fatalf("первая подсказка = %+v, ожидались продукты с одним использованием", suggestions[0])
	}
	if suggestions[1].ID != household.ID || suggestions[1].Used != 0 {
		t.Fatalf("вторая подсказка = %+v", suggestions[1])
	}

	// Фильтр по запросу ищет по имени и пути.
	filtered, err := db.SuggestShoppingAccounts(ctx, "хозяй", 10)
	if err != nil {
		t.Fatalf("SuggestShoppingAccounts фильтр: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != household.ID {
		t.Fatalf("фильтр по запросу: %+v", filtered)
	}

	// Счета других типов в подсказку не попадают.
	for _, item := range suggestions {
		if item.ID == asset.ID {
			t.Fatal("счёт актива попал в подсказку статей расходов")
		}
	}
}

// shoppingPurchaseTransaction собирает сбалансированную проводку покупки:
// дебет статей расхода и кредит источников оплаты.
func shoppingPurchaseTransaction(uuid string, actor int64, debits, credits map[int64]money.Kop) *model.Transaction {
	tr := &model.Transaction{
		UUID:        uuid,
		OpType:      model.OpExpense,
		Date:        time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Description: "покупка по списку",
		ActorID:     actor,
	}
	for account, amount := range debits {
		tr.Lines = append(tr.Lines, model.TransactionLine{
			AccountID: account, Amount: amount, Side: model.SideDebit,
		})
	}
	for account, amount := range credits {
		tr.Lines = append(tr.Lines, model.TransactionLine{
			AccountID: account, Amount: amount, Side: model.SideCredit,
		})
	}
	return tr
}

// TestASL_15_ShoppingPurchase проверяет проведение покупки по списку:
// одну и несколько позиций, несколько источников, откат при ошибке,
// идемпотентный повтор и завершение списка.
func TestASL_15_ShoppingPurchase(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")
	products := mustCreateAccount(t, db, "Продукты", model.TypeExpense, nil)
	household := mustCreateAccount(t, db, "Хозяйство", model.TypeExpense, nil)
	cash := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)
	card := mustCreateAccount(t, db, "Карта", model.TypeAsset, nil)

	list, err := db.CreateShoppingList(ctx, ShoppingListInput{
		Name:    "На выходные",
		OwnerID: userID,
	})
	if err != nil {
		t.Fatalf("CreateShoppingList: %v", err)
	}
	bread, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Хлеб", AccountID: &products.ID, ActualPrice: money.FromKopecks(5000),
		Status: model.ShoppingItemConfirmed,
	})
	if err != nil {
		t.Fatalf("AddShoppingItem(Хлеб): %v", err)
	}
	milk, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Молоко", AccountID: &household.ID, ActualPrice: money.FromKopecks(8000),
		InCart: true, Status: model.ShoppingItemConfirmed,
	})
	if err != nil {
		t.Fatalf("AddShoppingItem(Молоко): %v", err)
	}
	cheese, err := db.AddShoppingItem(ctx, list.ID, ShoppingItemInput{
		Name: "Сыр", AccountID: &products.ID, PlannedPrice: money.FromKopecks(3000),
	})
	if err != nil {
		t.Fatalf("AddShoppingItem(Сыр): %v", err)
	}

	purchase := shoppingPurchaseTransaction("purchase-1", userID,
		map[int64]money.Kop{products.ID: money.FromKopecks(5000), household.ID: money.FromKopecks(8000)},
		map[int64]money.Kop{cash.ID: money.FromKopecks(10000), card.ID: money.FromKopecks(3000)},
	)

	// Неподтверждённая позиция откатывает всё: проводки нет, статусы прежние.
	_, _, err = db.PostShoppingPurchase(ctx, ShoppingPurchaseInput{
		ListID: list.ID, ItemIDs: []int64{bread.ID, cheese.ID}, Transaction: purchase,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Code != CodeShoppingItemNotConfirmed {
		t.Fatalf("ошибка = %v, ожидался код %s", err, CodeShoppingItemNotConfirmed)
	}
	if count, err := db.CountTransactions(ctx, TransactionFilter{}); err != nil || count != 0 {
		t.Fatalf("после отката транзакций = %d (err %v), ожидалось 0", count, err)
	}
	unchanged, err := db.ShoppingItem(ctx, bread.ID)
	if err != nil {
		t.Fatalf("ShoppingItem: %v", err)
	}
	if unchanged.Status != model.ShoppingItemConfirmed || unchanged.Version != bread.Version {
		t.Fatalf("позиция изменилась при откате: статус %q, версия %d", unchanged.Status, unchanged.Version)
	}

	// Несколько позиций и несколько источников: одна сбалансированная проводка.
	tr, created, err := db.PostShoppingPurchase(ctx, ShoppingPurchaseInput{
		ListID: list.ID, ItemIDs: []int64{bread.ID, milk.ID}, Transaction: purchase,
	})
	if err != nil {
		t.Fatalf("PostShoppingPurchase: %v", err)
	}
	if !created || tr.UUID != "purchase-1" || len(tr.Lines) != 4 {
		t.Fatalf("проводка = %+v, created = %v", tr, created)
	}
	var debit, credit money.Kop
	for _, line := range tr.Lines {
		switch line.Side {
		case model.SideDebit:
			debit += line.Amount
		case model.SideCredit:
			credit += line.Amount
		}
	}
	if debit != credit || debit != money.FromKopecks(13000) {
		t.Fatalf("ΣDEBIT = %s, ΣCREDIT = %s", debit, credit)
	}
	for _, id := range []int64{bread.ID, milk.ID} {
		item, err := db.ShoppingItem(ctx, id)
		if err != nil {
			t.Fatalf("ShoppingItem(%d): %v", id, err)
		}
		if item.Status != model.ShoppingItemPurchased {
			t.Fatalf("позиция %d не отмечена купленной: %q", id, item.Status)
		}
	}

	// Сыр остался запланированным, поэтому список ещё не завершён.
	got, err := db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList: %v", err)
	}
	if got.Status == model.ShoppingListCompleted {
		t.Fatalf("список завершён, хотя осталась незакупленная позиция")
	}

	// Повтор с тем же uuid не создаёт дубль и не двигает версии.
	repeat, created, err := db.PostShoppingPurchase(ctx, ShoppingPurchaseInput{
		ListID: list.ID, ItemIDs: []int64{bread.ID, milk.ID}, Transaction: purchase,
	})
	if err != nil {
		t.Fatalf("повтор PostShoppingPurchase: %v", err)
	}
	if created || repeat.ID != tr.ID {
		t.Fatalf("повтор создал новую проводку: id %d против %d, created %v", repeat.ID, tr.ID, created)
	}
	if count, err := db.CountTransactions(ctx, TransactionFilter{}); err != nil || count != 1 {
		t.Fatalf("после повтора транзакций = %d (err %v), ожидалась 1", count, err)
	}
	afterRepeat, err := db.ShoppingItem(ctx, bread.ID)
	if err != nil {
		t.Fatalf("ShoppingItem after repeat: %v", err)
	}
	if afterRepeat.Version != unchanged.Version+1 {
		t.Fatalf("версия после повтора = %d, ожидалась %d", afterRepeat.Version, unchanged.Version+1)
	}

	// Покупка уже купленной позиции с новым uuid отклоняется.
	_, _, err = db.PostShoppingPurchase(ctx, ShoppingPurchaseInput{
		ListID: list.ID, ItemIDs: []int64{bread.ID},
		Transaction: shoppingPurchaseTransaction("purchase-2", userID,
			map[int64]money.Kop{products.ID: money.FromKopecks(5000)},
			map[int64]money.Kop{cash.ID: money.FromKopecks(5000)}),
	})
	if !errors.As(err, &ve) || ve.Code != CodeShoppingItemPurchased {
		t.Fatalf("ошибка повторной покупки = %v, ожидался код %s", err, CodeShoppingItemPurchased)
	}

	// Позиция из чужого списка не принимается.
	other, err := db.CreateShoppingList(ctx, ShoppingListInput{Name: "Другой", OwnerID: userID})
	if err != nil {
		t.Fatalf("CreateShoppingList(other): %v", err)
	}
	foreign, err := db.AddShoppingItem(ctx, other.ID, ShoppingItemInput{
		Name: "Чужой", AccountID: &products.ID, ActualPrice: money.FromKopecks(100),
		Status: model.ShoppingItemConfirmed,
	})
	if err != nil {
		t.Fatalf("AddShoppingItem(foreign): %v", err)
	}
	_, _, err = db.PostShoppingPurchase(ctx, ShoppingPurchaseInput{
		ListID: list.ID, ItemIDs: []int64{foreign.ID},
		Transaction: shoppingPurchaseTransaction("purchase-3", userID,
			map[int64]money.Kop{products.ID: money.FromKopecks(100)},
			map[int64]money.Kop{cash.ID: money.FromKopecks(100)}),
	})
	if !errors.As(err, &ve) || ve.Code != CodeShoppingItemNotFound {
		t.Fatalf("ошибка чужой позиции = %v, ожидался код %s", err, CodeShoppingItemNotFound)
	}

	// После покупки последней позиции список завершается.
	price := money.FromKopecks(3000)
	if _, err := db.UpdateShoppingItem(ctx, cheese.ID, ShoppingItemPatch{
		Version: cheese.Version, ActualPrice: &price, Status: statusPtr(model.ShoppingItemConfirmed),
	}); err != nil {
		t.Fatalf("UpdateShoppingItem(Сыр): %v", err)
	}
	if _, _, err := db.PostShoppingPurchase(ctx, ShoppingPurchaseInput{
		ListID: list.ID, ItemIDs: []int64{cheese.ID},
		Transaction: shoppingPurchaseTransaction("purchase-4", userID,
			map[int64]money.Kop{products.ID: money.FromKopecks(3000)},
			map[int64]money.Kop{cash.ID: money.FromKopecks(3000)}),
	}); err != nil {
		t.Fatalf("PostShoppingPurchase(Сыр): %v", err)
	}
	completed, err := db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList completed: %v", err)
	}
	if completed.Status != model.ShoppingListCompleted {
		t.Fatalf("список не завершён: статус %q", completed.Status)
	}
}

func statusPtr(v model.ShoppingItemStatus) *model.ShoppingItemStatus { return &v }
func strPtr(v string) *string                                        { return &v }
