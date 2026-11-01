package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/uuid"
)

// shoppingListSelect — единый запрос списка с агрегатами по позициям.
const shoppingListSelect = `
	SELECT l.id, l.uuid, l.name, l.planned_date, l.status, l.owner_id,
	       l.created_at, l.updated_at, l.archived_at,
	       (SELECT COUNT(*) FROM shopping_items i WHERE i.list_id = l.id),
	       (SELECT COUNT(*) FROM shopping_items i WHERE i.list_id = l.id
	          AND (i.actual_price_kop > 0 OR i.in_cart = 1)),
	       (SELECT COALESCE(SUM(i.actual_price_kop), 0) FROM shopping_items i
	          WHERE i.list_id = l.id AND (i.actual_price_kop > 0 OR i.in_cart = 1))
	  FROM shopping_lists l`

// shoppingItemColumns — единый список колонок позиции.
const shoppingItemColumns = `id, uuid, list_id, name, account_id, assignee_id,
	priority, planned_price_kop, actual_price_kop, in_cart, price_mismatch,
	comment, status, position, version, created_at, updated_at`

// ShoppingListInput — данные для создания списка покупок.
type ShoppingListInput struct {
	// UUID — идентификатор списка; пустое значение означает генерацию.
	UUID string
	// Name — название списка.
	Name string
	// PlannedDate — планируемая дата похода в магазин; нулевое время — не задана.
	PlannedDate time.Time
	// Status — статус списка; пустое значение означает «в работе».
	Status model.ShoppingListStatus
	// OwnerID — владелец списка.
	OwnerID int64
}

// ShoppingListFilter — условия выборки списков покупок.
type ShoppingListFilter struct {
	// Status ограничивает выборку статусом.
	Status *model.ShoppingListStatus
	// OwnerID ограничивает выборку владельцем.
	OwnerID *int64
	// IncludeArchived включает архивные списки.
	IncludeArchived bool
	// Limit и Offset задают страницу; Limit = 0 — без ограничения.
	Limit  int
	Offset int
}

// ShoppingListPatch — частичное изменение списка покупок.
type ShoppingListPatch struct {
	Name        *string
	PlannedDate *time.Time
	Status      *model.ShoppingListStatus
	OwnerID     *int64
}

// ShoppingItemInput — данные для создания позиции списка.
type ShoppingItemInput struct {
	// UUID — идентификатор позиции; пустое значение означает генерацию.
	UUID string
	// Name — наименование товара.
	Name string
	// AccountID — статья расходов; может быть не задана в черновике.
	AccountID *int64
	// AssigneeID — ответственный за покупку.
	AssigneeID *int64
	// Priority — приоритет/группа.
	Priority model.ShoppingPriority
	// PlannedPrice — ориентировочная цена.
	PlannedPrice money.Kop
	// ActualPrice — фактическая цена в магазине.
	ActualPrice money.Kop
	// InCart — товар в корзине (цена может быть неизвестна).
	InCart bool
	// PriceMismatch — зафиксировано расхождение с ценником.
	PriceMismatch bool
	// Comment — комментарий к позиции.
	Comment string
	// Status — статус позиции; пустое значение означает «запланирована».
	Status model.ShoppingItemStatus
	// Position — порядок в списке; 0 — в конец.
	Position int
}

// ShoppingItemPatch — частичное изменение позиции с оптимистичной версией.
type ShoppingItemPatch struct {
	// Version — версия, которую видел клиент; обязательна.
	Version int64
	// Name — новое наименование.
	Name *string
	// AccountID — новая статья расходов.
	AccountID *int64
	// ClearAccount — очистить статью расходов (если AccountID не задан).
	ClearAccount bool
	// AssigneeID — новый ответственный.
	AssigneeID *int64
	// ClearAssignee — очистить ответственного (если AssigneeID не задан).
	ClearAssignee bool
	// Priority — новый приоритет.
	Priority *model.ShoppingPriority
	// PlannedPrice — новая ориентировочная цена.
	PlannedPrice *money.Kop
	// ActualPrice — новая фактическая цена.
	ActualPrice *money.Kop
	// InCart — отметка «в корзине».
	InCart *bool
	// PriceMismatch — отметка расхождения с ценником.
	PriceMismatch *bool
	// Comment — новый комментарий.
	Comment *string
	// Status — новый статус позиции.
	Status *model.ShoppingItemStatus
	// Position — новый порядок в списке.
	Position *int
}

// ShoppingTemplateItemInput — базовая позиция шаблона.
type ShoppingTemplateItemInput struct {
	Name      string
	AccountID *int64
	Priority  model.ShoppingPriority
	Position  int
}

// CreateShoppingList создаёт список покупок.
func (db *DB) CreateShoppingList(ctx context.Context, in ShoppingListInput) (*model.ShoppingList, error) {
	name, err := validateShoppingName(in.Name)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = model.ShoppingListActive
	}
	if !status.Valid() {
		return nil, shoppingValidation(CodeShoppingStatus, "недопустимый статус списка «"+string(status)+"»")
	}

	var id int64
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		if err := validateUserTx(ctx, tx, in.OwnerID); err != nil {
			return err
		}
		var err error
		id, err = insertShoppingListTx(ctx, tx, in.UUID, name, in.PlannedDate, status, in.OwnerID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return db.ShoppingList(ctx, id)
}

// ShoppingList возвращает список покупок вместе с позициями.
func (db *DB) ShoppingList(ctx context.Context, id int64) (*model.ShoppingList, error) {
	row := db.QueryRowContext(ctx, shoppingListSelect+` WHERE l.id = ?`, id)
	list, err := scanShoppingList(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: список покупок %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	items, err := db.ListShoppingItems(ctx, id)
	if err != nil {
		return nil, err
	}
	list.Items = items
	return list, nil
}

// ListShoppingLists возвращает списки покупок по условиям фильтра.
func (db *DB) ListShoppingLists(ctx context.Context, f ShoppingListFilter) ([]model.ShoppingList, error) {
	query := shoppingListSelect + ` WHERE 1 = 1`
	var args []any

	if f.Status != nil {
		query += ` AND l.status = ?`
		args = append(args, string(*f.Status))
	} else if !f.IncludeArchived {
		query += ` AND l.status <> 'archived'`
	}
	if f.OwnerID != nil {
		query += ` AND l.owner_id = ?`
		args = append(args, *f.OwnerID)
	}
	query += ` ORDER BY l.planned_date DESC, l.id DESC`
	if f.Limit > 0 || f.Offset > 0 {
		limit := f.Limit
		if limit == 0 {
			limit = -1
		}
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, f.Offset)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать списки покупок: %w", err)
	}
	defer rows.Close()

	var lists []model.ShoppingList
	for rows.Next() {
		list, err := scanShoppingList(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать список покупок: %w", err)
		}
		lists = append(lists, *list)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения списков покупок: %w", err)
	}
	return lists, nil
}

// UpdateShoppingList изменяет список покупок.
func (db *DB) UpdateShoppingList(ctx context.Context, id int64, patch ShoppingListPatch) (*model.ShoppingList, error) {
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		list, err := getShoppingListTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if list.Status == model.ShoppingListArchived && (patch.Status == nil || *patch.Status != model.ShoppingListArchived) {
			return shoppingValidation(CodeShoppingListArchived, "список покупок заархивирован")
		}

		name := list.Name
		if patch.Name != nil {
			name, err = validateShoppingName(*patch.Name)
			if err != nil {
				return err
			}
		}
		status := list.Status
		if patch.Status != nil {
			status = *patch.Status
			if !status.Valid() {
				return shoppingValidation(CodeShoppingStatus, "недопустимый статус списка «"+string(status)+"»")
			}
		}
		ownerID := list.OwnerID
		if patch.OwnerID != nil {
			if err := validateUserTx(ctx, tx, *patch.OwnerID); err != nil {
				return err
			}
			ownerID = *patch.OwnerID
		}
		plannedDate := list.PlannedDate
		if patch.PlannedDate != nil {
			plannedDate = *patch.PlannedDate
		}

		var archivedAt any
		if status == model.ShoppingListArchived {
			archivedAt = Now()
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE shopping_lists
			    SET name = ?, planned_date = ?, status = ?, owner_id = ?,
			        updated_at = ?, archived_at = ?
			  WHERE id = ?`,
			name, toUnix(plannedDate), string(status), ownerID, Now(), archivedAt, id); err != nil {
			return fmt.Errorf("store: не удалось изменить список покупок: %w", err)
		}
		op := model.OpUpdate
		if status == model.ShoppingListArchived {
			op = model.OpArchive
		}
		return appendChangelog(ctx, tx, model.EntityShoppingList, id, op)
	})
	if err != nil {
		return nil, err
	}
	return db.ShoppingList(ctx, id)
}

// ArchiveShoppingList переводит список в архив, сохраняя позиции.
func (db *DB) ArchiveShoppingList(ctx context.Context, id int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		list, err := getShoppingListTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if list.Status == model.ShoppingListArchived {
			return nil
		}
		now := Now()
		if _, err := tx.ExecContext(ctx,
			`UPDATE shopping_lists SET status = 'archived', archived_at = ?, updated_at = ? WHERE id = ?`,
			now, now, id); err != nil {
			return fmt.Errorf("store: не удалось заархивировать список покупок: %w", err)
		}
		return appendChangelog(ctx, tx, model.EntityShoppingList, id, model.OpArchive)
	})
}

// ListShoppingItems возвращает позиции списка в порядке отображения.
func (db *DB) ListShoppingItems(ctx context.Context, listID int64) ([]model.ShoppingListItem, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+shoppingItemColumns+` FROM shopping_items
		  WHERE list_id = ? ORDER BY position, id`, listID)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать позиции списка: %w", err)
	}
	defer rows.Close()

	var items []model.ShoppingListItem
	for rows.Next() {
		item, err := scanShoppingItem(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать позицию списка: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения позиций списка: %w", err)
	}
	return items, nil
}

// ListAllShoppingItems возвращает все позиции всех списков.
//
// Используется снапшотом синхронизации: клиент получает списки целиком.
func (db *DB) ListAllShoppingItems(ctx context.Context) ([]model.ShoppingListItem, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+shoppingItemColumns+` FROM shopping_items ORDER BY list_id, position, id`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать все позиции списков: %w", err)
	}
	defer rows.Close()

	var items []model.ShoppingListItem
	for rows.Next() {
		item, err := scanShoppingItem(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать позицию списка: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения позиций списков: %w", err)
	}
	return items, nil
}

// ListShoppingTemplatesFull возвращает шаблоны вместе с позициями.
func (db *DB) ListShoppingTemplatesFull(ctx context.Context) ([]model.ShoppingListTemplate, error) {
	templates, err := db.ListShoppingTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for i := range templates {
		full, err := db.ShoppingTemplate(ctx, templates[i].ID)
		if err != nil {
			return nil, err
		}
		templates[i] = *full
	}
	return templates, nil
}

// ShoppingAccountSuggestion — статья расходов в подсказке списка покупок.
//
// Used — сколько раз счёт встречался в дебете проводок типа «трата»:
// подсказка поднимает привычные статьи выше редких.
type ShoppingAccountSuggestion struct {
	// ID — идентификатор счёта.
	ID int64
	// Path — полный путь счёта.
	Path string
	// Used — число использований в прошлых тратах.
	Used int
}

// SuggestShoppingAccounts подсказывает листовые статьи расходов.
//
// Сортировка: сначала часто используемые в прошлых тратах, затем по пути.
// Пустой запрос возвращает самые употребимые статьи.
func (db *DB) SuggestShoppingAccounts(ctx context.Context, query string, limit int) ([]ShoppingAccountSuggestion, error) {
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.name, a.full_path,
		       (SELECT COUNT(*)
		          FROM transaction_lines l
		          JOIN transactions t ON t.id = l.transaction_id
		         WHERE l.account_id = a.id AND l.side = 'DEBIT' AND t.op_type = 'трата') AS used
		  FROM accounts a
		 WHERE a.is_leaf = 1 AND a.is_archived = 0 AND a.type = 'расход'
		 ORDER BY used DESC, a.full_path`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать статьи расходов: %w", err)
	}
	defer rows.Close()

	q := strings.ToLower(strings.TrimSpace(query))
	var result []ShoppingAccountSuggestion
	for rows.Next() {
		var (
			item ShoppingAccountSuggestion
			name string
		)
		if err := rows.Scan(&item.ID, &name, &item.Path, &item.Used); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать статью расходов: %w", err)
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(name), q) &&
			!strings.Contains(strings.ToLower(item.Path), q) {
			continue
		}
		result = append(result, item)
		if len(result) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения статей расходов: %w", err)
	}
	return result, nil
}

// ShoppingItem возвращает позицию списка по идентификатору.
func (db *DB) ShoppingItem(ctx context.Context, id int64) (*model.ShoppingListItem, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+shoppingItemColumns+` FROM shopping_items WHERE id = ?`, id)
	item, err := scanShoppingItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: позиция списка %d", ErrNotFound, id)
	}
	return item, err
}

// AddShoppingItem добавляет позицию в список покупок.
func (db *DB) AddShoppingItem(ctx context.Context, listID int64, in ShoppingItemInput) (*model.ShoppingListItem, error) {
	var id int64
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		list, err := getShoppingListTx(ctx, tx, listID)
		if err != nil {
			return err
		}
		if list.Status == model.ShoppingListArchived {
			return shoppingValidation(CodeShoppingListArchived, "список покупок заархивирован")
		}
		if err := validateShoppingItemTx(ctx, tx, in); err != nil {
			return err
		}
		position := in.Position
		if position <= 0 {
			if err := tx.QueryRowContext(ctx,
				`SELECT COALESCE(MAX(position), 0) + 1 FROM shopping_items WHERE list_id = ?`,
				listID).Scan(&position); err != nil {
				return fmt.Errorf("store: не удалось определить порядок позиции: %w", err)
			}
		}
		id, err = insertShoppingItemTx(ctx, tx, listID, in, position)
		return err
	})
	if err != nil {
		return nil, err
	}
	return db.ShoppingItem(ctx, id)
}

// UpdateShoppingItem изменяет позицию с проверкой версии.
func (db *DB) UpdateShoppingItem(ctx context.Context, id int64, patch ShoppingItemPatch) (*model.ShoppingListItem, error) {
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		item, err := getShoppingItemTx(ctx, tx, id)
		if err != nil {
			return err
		}
		list, err := getShoppingListTx(ctx, tx, item.ListID)
		if err != nil {
			return err
		}
		if list.Status == model.ShoppingListArchived {
			return shoppingValidation(CodeShoppingListArchived, "список покупок заархивирован")
		}
		if patch.Version != item.Version {
			return shoppingValidation(CodeShoppingVersionConflict,
				fmt.Sprintf("позиция изменена другим клиентом: ожидалась версия %d, получена %d", item.Version, patch.Version))
		}

		applyShoppingItemPatch(item, patch)
		if err := validateShoppingItemTx(ctx, tx, ShoppingItemInput{
			Name:          item.Name,
			AccountID:     item.AccountID,
			AssigneeID:    item.AssigneeID,
			Priority:      item.Priority,
			PlannedPrice:  item.PlannedPrice,
			ActualPrice:   item.ActualPrice,
			InCart:        item.InCart,
			PriceMismatch: item.PriceMismatch,
			Comment:       item.Comment,
			Status:        item.Status,
		}); err != nil {
			return err
		}
		item.Version++
		item.UpdatedAt = fromUnix(Now())
		if _, err := tx.ExecContext(ctx,
			`UPDATE shopping_items
			    SET name = ?, account_id = ?, assignee_id = ?, priority = ?,
			        planned_price_kop = ?, actual_price_kop = ?, in_cart = ?,
			        price_mismatch = ?, comment = ?, status = ?, position = ?,
			        version = ?, updated_at = ?
			  WHERE id = ?`,
			item.Name, nullableID(item.AccountID), nullableID(item.AssigneeID),
			string(item.Priority), int64(item.PlannedPrice), int64(item.ActualPrice),
			boolInt(item.InCart), boolInt(item.PriceMismatch), item.Comment,
			string(item.Status), item.Position, item.Version, toUnix(item.UpdatedAt), id); err != nil {
			return fmt.Errorf("store: не удалось изменить позицию списка: %w", err)
		}
		return appendChangelog(ctx, tx, model.EntityShoppingItem, id, model.OpUpdate)
	})
	if err != nil {
		return nil, err
	}
	return db.ShoppingItem(ctx, id)
}

// DeleteShoppingItem удаляет позицию, если она ещё не куплена.
func (db *DB) DeleteShoppingItem(ctx context.Context, id int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		item, err := getShoppingItemTx(ctx, tx, id)
		if err != nil {
			return err
		}
		list, err := getShoppingListTx(ctx, tx, item.ListID)
		if err != nil {
			return err
		}
		if list.Status == model.ShoppingListArchived {
			return shoppingValidation(CodeShoppingListArchived, "список покупок заархивирован")
		}
		if item.Status == model.ShoppingItemPurchased {
			return shoppingValidation(CodeShoppingItemPurchased, "купленная позиция защищена от удаления")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM shopping_items WHERE id = ?`, id); err != nil {
			return fmt.Errorf("store: не удалось удалить позицию списка: %w", err)
		}
		return appendChangelog(ctx, tx, model.EntityShoppingItem, id, model.OpDelete)
	})
}

// shoppingPostMu сериализует проведения покупок в процессе демона: повторная
// отправка с тем же uuid не должна создать дубль даже при одновременных
// запросах. Демон — единственная точка записи, поэтому мьютекса процесса
// достаточно.
var shoppingPostMu sync.Mutex

// ShoppingPurchaseInput — данные проведения покупки по списку.
type ShoppingPurchaseInput struct {
	// ListID — список покупок.
	ListID int64
	// ItemIDs — позиции, которые нужно отметить купленными.
	ItemIDs []int64
	// Transaction — подготовленная проводка расхода.
	Transaction *model.Transaction
}

// PostShoppingPurchase сохраняет проводку покупки и отмечает позиции купленными
// одной транзакцией БД.
//
// Идемпотентность: если проводка с таким uuid уже есть, возвращается она же и
// created = false; позиции повторно не меняются. Если хотя бы одна позиция не
// принадлежит списку, не подтверждена или уже куплена, откатывается всё.
func (db *DB) PostShoppingPurchase(ctx context.Context, in ShoppingPurchaseInput) (*model.Transaction, bool, error) {
	if in.Transaction == nil {
		return nil, false, shoppingValidation(CodeShoppingStatus, "не задана проводка покупки")
	}
	if in.ListID <= 0 || len(in.ItemIDs) == 0 {
		return nil, false, shoppingValidation(CodeShoppingStatus, "не задан список или позиции покупки")
	}

	shoppingPostMu.Lock()
	defer shoppingPostMu.Unlock()

	var (
		id      int64
		created bool
	)
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		var existing int64
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM transactions WHERE uuid = ?`, in.Transaction.UUID).Scan(&existing)
		switch {
		case err == nil:
			id = existing
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("store: не удалось проверить uuid покупки: %w", err)
		}

		list, err := getShoppingListTx(ctx, tx, in.ListID)
		if err != nil {
			return err
		}
		if list.Status == model.ShoppingListArchived {
			return shoppingValidation(CodeShoppingListArchived, "список покупок заархивирован")
		}

		seen := make(map[int64]bool, len(in.ItemIDs))
		for _, itemID := range in.ItemIDs {
			if seen[itemID] {
				continue
			}
			seen[itemID] = true
			item, err := getShoppingItemTx(ctx, tx, itemID)
			if err != nil {
				return shoppingValidation(CodeShoppingItemNotFound, "позиция покупки не найдена")
			}
			if item.ListID != in.ListID {
				return shoppingValidation(CodeShoppingItemNotFound, "позиция не принадлежит списку покупок")
			}
			if item.Status == model.ShoppingItemPurchased {
				return shoppingValidation(CodeShoppingItemPurchased, "позиция уже куплена")
			}
			if !item.Confirmed() {
				return shoppingValidation(CodeShoppingItemNotConfirmed, "позиция не подтверждена к покупке")
			}
		}

		id, err = insertTransactionTx(ctx, tx, in.Transaction)
		if err != nil {
			return err
		}
		created = true

		now := Now()
		for itemID := range seen {
			if _, err := tx.ExecContext(ctx,
				`UPDATE shopping_items SET status = ?, version = version + 1, updated_at = ? WHERE id = ?`,
				string(model.ShoppingItemPurchased), now, itemID); err != nil {
				return fmt.Errorf("store: не удалось отметить позицию купленной: %w", err)
			}
			if err := appendChangelog(ctx, tx, model.EntityShoppingItem, itemID, model.OpUpdate); err != nil {
				return err
			}
		}

		// Если незакупленных позиций не осталось, список завершён.
		var remaining int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM shopping_items WHERE list_id = ? AND status NOT IN (?, ?)`,
			in.ListID, string(model.ShoppingItemPurchased), string(model.ShoppingItemSkipped)).Scan(&remaining); err != nil {
			return fmt.Errorf("store: не удалось проверить остаток позиций: %w", err)
		}
		if remaining == 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE shopping_lists SET status = ?, updated_at = ? WHERE id = ? AND status != ?`,
				string(model.ShoppingListCompleted), now, in.ListID, string(model.ShoppingListArchived)); err != nil {
				return fmt.Errorf("store: не удалось завершить список покупок: %w", err)
			}
			if err := appendChangelog(ctx, tx, model.EntityShoppingList, in.ListID, model.OpUpdate); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	tr, err := db.Transaction(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return tr, created, nil
}

// CreateShoppingTemplate создаёт шаблон списка из базовых позиций.
func (db *DB) CreateShoppingTemplate(ctx context.Context, name string, items []ShoppingTemplateItemInput) (*model.ShoppingListTemplate, error) {
	name, err := validateShoppingName(name)
	if err != nil {
		return nil, err
	}
	var id int64
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		uuidStr, err := newShoppingUUID()
		if err != nil {
			return err
		}
		now := Now()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO shopping_list_templates(uuid, name, created_at, updated_at) VALUES(?, ?, ?, ?)`,
			uuidStr, name, now, now)
		if err != nil {
			return fmt.Errorf("store: не удалось создать шаблон списка: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: не удалось получить id шаблона: %w", err)
		}
		for i, item := range items {
			if err := insertShoppingTemplateItemTx(ctx, tx, id, item, i+1); err != nil {
				return err
			}
		}
		return appendChangelog(ctx, tx, model.EntityShoppingTemplate, id, model.OpInsert)
	})
	if err != nil {
		return nil, err
	}
	return db.ShoppingTemplate(ctx, id)
}

// ShoppingTemplate возвращает шаблон вместе с позициями.
func (db *DB) ShoppingTemplate(ctx context.Context, id int64) (*model.ShoppingListTemplate, error) {
	var (
		tpl     model.ShoppingListTemplate
		created int64
		updated int64
	)
	err := db.QueryRowContext(ctx,
		`SELECT id, uuid, name, created_at, updated_at FROM shopping_list_templates WHERE id = ?`, id).
		Scan(&tpl.ID, &tpl.UUID, &tpl.Name, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: шаблон списка %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать шаблон списка: %w", err)
	}
	tpl.CreatedAt = fromUnix(created)
	tpl.UpdatedAt = fromUnix(updated)

	rows, err := db.QueryContext(ctx,
		`SELECT id, template_id, name, account_id, priority, position
		   FROM shopping_template_items WHERE template_id = ? ORDER BY position, id`, id)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать позиции шаблона: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			item    model.ShoppingTemplateItem
			account sql.NullInt64
			prio    string
		)
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.Name, &account, &prio, &item.Position); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать позицию шаблона: %w", err)
		}
		if account.Valid {
			v := account.Int64
			item.AccountID = &v
		}
		item.Priority = model.ShoppingPriority(prio)
		tpl.Items = append(tpl.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения позиций шаблона: %w", err)
	}
	return &tpl, nil
}

// ListShoppingTemplates возвращает все шаблоны списков.
func (db *DB) ListShoppingTemplates(ctx context.Context) ([]model.ShoppingListTemplate, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, uuid, name, created_at, updated_at FROM shopping_list_templates ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать шаблоны списков: %w", err)
	}
	defer rows.Close()

	var templates []model.ShoppingListTemplate
	for rows.Next() {
		var (
			tpl     model.ShoppingListTemplate
			created int64
			updated int64
		)
		if err := rows.Scan(&tpl.ID, &tpl.UUID, &tpl.Name, &created, &updated); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать шаблон списка: %w", err)
		}
		tpl.CreatedAt = fromUnix(created)
		tpl.UpdatedAt = fromUnix(updated)
		templates = append(templates, tpl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения шаблонов списков: %w", err)
	}
	return templates, nil
}

// DeleteShoppingTemplate удаляет шаблон и его позиции.
func (db *DB) DeleteShoppingTemplate(ctx context.Context, id int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM shopping_list_templates WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("store: не удалось удалить шаблон списка: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: не удалось удалить шаблон списка: %w", err)
		}
		if affected == 0 {
			return fmt.Errorf("%w: шаблон списка %d", ErrNotFound, id)
		}
		return appendChangelog(ctx, tx, model.EntityShoppingTemplate, id, model.OpDelete)
	})
}

// CreateShoppingListFromTemplate создаёт список из шаблона.
//
// В новый список копируются только базовые позиции: наименование, статья
// расходов и приоритет. Цены, ответственные и статусы не копируются.
func (db *DB) CreateShoppingListFromTemplate(ctx context.Context, templateID int64, name string, ownerID int64, plannedDate time.Time) (*model.ShoppingList, error) {
	name, err := validateShoppingName(name)
	if err != nil {
		return nil, err
	}
	var id int64
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		if err := validateUserTx(ctx, tx, ownerID); err != nil {
			return err
		}
		var tplName string
		if err := tx.QueryRowContext(ctx,
			`SELECT name FROM shopping_list_templates WHERE id = ?`, templateID).Scan(&tplName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: шаблон списка %d", ErrNotFound, templateID)
			}
			return fmt.Errorf("store: не удалось прочитать шаблон списка: %w", err)
		}
		var err error
		id, err = insertShoppingListTx(ctx, tx, "", name, plannedDate, model.ShoppingListActive, ownerID)
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx,
			`SELECT name, account_id, priority, position
			   FROM shopping_template_items WHERE template_id = ? ORDER BY position, id`, templateID)
		if err != nil {
			return fmt.Errorf("store: не удалось прочитать позиции шаблона: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				itemName string
				account  sql.NullInt64
				prio     string
				position int
			)
			if err := rows.Scan(&itemName, &account, &prio, &position); err != nil {
				return fmt.Errorf("store: не удалось разобрать позицию шаблона: %w", err)
			}
			var accountID *int64
			if account.Valid {
				v := account.Int64
				accountID = &v
			}
			item := ShoppingItemInput{
				Name:      itemName,
				AccountID: accountID,
				Priority:  model.ShoppingPriority(prio),
				Status:    model.ShoppingItemPlanned,
			}
			if err := validateShoppingItemTx(ctx, tx, item); err != nil {
				return err
			}
			if _, err := insertShoppingItemTx(ctx, tx, id, item, position); err != nil {
				return err
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("store: ошибка чтения позиций шаблона: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return db.ShoppingList(ctx, id)
}

// SaveShoppingListAsTemplate сохраняет базовые позиции списка как шаблон.
func (db *DB) SaveShoppingListAsTemplate(ctx context.Context, listID int64, name string) (*model.ShoppingListTemplate, error) {
	list, err := db.ShoppingList(ctx, listID)
	if err != nil {
		return nil, err
	}
	items := make([]ShoppingTemplateItemInput, 0, len(list.Items))
	for i, item := range list.Items {
		items = append(items, ShoppingTemplateItemInput{
			Name:      item.Name,
			AccountID: item.AccountID,
			Priority:  item.Priority,
			Position:  i + 1,
		})
	}
	return db.CreateShoppingTemplate(ctx, name, items)
}

// insertShoppingListTx создаёт список внутри транзакции.
func insertShoppingListTx(ctx context.Context, tx *sql.Tx, id string, name string, plannedDate time.Time, status model.ShoppingListStatus, ownerID int64) (int64, error) {
	uuidStr, err := shoppingUUID(id)
	if err != nil {
		return 0, err
	}
	now := Now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO shopping_lists(uuid, name, planned_date, status, owner_id, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		uuidStr, name, toUnix(plannedDate), string(status), ownerID, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, fmt.Errorf("store: список покупок с uuid %q уже существует", uuidStr)
		}
		return 0, fmt.Errorf("store: не удалось создать список покупок: %w", err)
	}
	listID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: не удалось получить id списка покупок: %w", err)
	}
	if err := appendChangelog(ctx, tx, model.EntityShoppingList, listID, model.OpInsert); err != nil {
		return 0, err
	}
	return listID, nil
}

// insertShoppingItemTx создаёт позицию внутри транзакции.
func insertShoppingItemTx(ctx context.Context, tx *sql.Tx, listID int64, in ShoppingItemInput, position int) (int64, error) {
	uuidStr, err := shoppingUUID(in.UUID)
	if err != nil {
		return 0, err
	}
	status := in.Status
	if status == "" {
		status = model.ShoppingItemPlanned
	}
	priority := in.Priority
	if priority == "" {
		priority = model.ShoppingPriorityNormal
	}
	now := Now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO shopping_items(uuid, list_id, name, account_id, assignee_id,
		    priority, planned_price_kop, actual_price_kop, in_cart, price_mismatch,
		    comment, status, position, version, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		uuidStr, listID, in.Name, nullableID(in.AccountID), nullableID(in.AssigneeID),
		string(priority), int64(in.PlannedPrice), int64(in.ActualPrice),
		boolInt(in.InCart), boolInt(in.PriceMismatch), in.Comment, string(status),
		position, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, fmt.Errorf("store: позиция списка с uuid %q уже существует", uuidStr)
		}
		return 0, fmt.Errorf("store: не удалось создать позицию списка: %w", err)
	}
	itemID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: не удалось получить id позиции списка: %w", err)
	}
	if err := appendChangelog(ctx, tx, model.EntityShoppingItem, itemID, model.OpInsert); err != nil {
		return 0, err
	}
	return itemID, nil
}

// insertShoppingTemplateItemTx создаёт базовую позицию шаблона.
func insertShoppingTemplateItemTx(ctx context.Context, tx *sql.Tx, templateID int64, in ShoppingTemplateItemInput, position int) error {
	name, err := validateShoppingName(in.Name)
	if err != nil {
		return err
	}
	if err := validateShoppingAccountTx(ctx, tx, in.AccountID); err != nil {
		return err
	}
	priority := in.Priority
	if priority == "" {
		priority = model.ShoppingPriorityNormal
	}
	if !priority.Valid() {
		return shoppingValidation(CodeShoppingPriority, "недопустимый приоритет «"+string(priority)+"»")
	}
	if position <= 0 {
		position = in.Position
	}
	if position <= 0 {
		position = 1
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO shopping_template_items(template_id, name, account_id, priority, position)
		 VALUES(?, ?, ?, ?, ?)`,
		templateID, name, nullableID(in.AccountID), string(priority), position); err != nil {
		return fmt.Errorf("store: не удалось создать позицию шаблона: %w", err)
	}
	return nil
}

// getShoppingListTx читает список внутри транзакции (без позиций).
func getShoppingListTx(ctx context.Context, tx *sql.Tx, id int64) (*model.ShoppingList, error) {
	row := tx.QueryRowContext(ctx, shoppingListSelect+` WHERE l.id = ?`, id)
	list, err := scanShoppingList(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: список покупок %d", ErrNotFound, id)
	}
	return list, err
}

// getShoppingItemTx читает позицию внутри транзакции.
func getShoppingItemTx(ctx context.Context, tx *sql.Tx, id int64) (*model.ShoppingListItem, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+shoppingItemColumns+` FROM shopping_items WHERE id = ?`, id)
	item, err := scanShoppingItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: позиция списка %d", ErrNotFound, id)
	}
	return item, err
}

// scanShoppingList разбирает строку выборки списка.
func scanShoppingList(s rowScanner) (*model.ShoppingList, error) {
	var (
		list           model.ShoppingList
		status         string
		planned        int64
		created        int64
		updated        int64
		archived       sql.NullInt64
		confirmedTotal int64
	)
	if err := s.Scan(&list.ID, &list.UUID, &list.Name, &planned, &status, &list.OwnerID,
		&created, &updated, &archived, &list.ItemCount, &list.ConfirmedCount, &confirmedTotal); err != nil {
		return nil, err
	}
	list.Status = model.ShoppingListStatus(status)
	list.PlannedDate = fromUnixOptional(planned)
	list.CreatedAt = fromUnix(created)
	list.UpdatedAt = fromUnix(updated)
	list.ConfirmedTotal = money.FromKopecks(confirmedTotal)
	if archived.Valid {
		t := fromUnix(archived.Int64)
		list.ArchivedAt = &t
	}
	return &list, nil
}

// scanShoppingItem разбирает строку выборки позиции.
func scanShoppingItem(s rowScanner) (*model.ShoppingListItem, error) {
	var (
		item     model.ShoppingListItem
		account  sql.NullInt64
		assignee sql.NullInt64
		priority string
		status   string
		inCart   int
		mismatch int
		planned  int64
		actual   int64
		created  int64
		updated  int64
	)
	if err := s.Scan(&item.ID, &item.UUID, &item.ListID, &item.Name, &account, &assignee,
		&priority, &planned, &actual, &inCart, &mismatch, &item.Comment, &status,
		&item.Position, &item.Version, &created, &updated); err != nil {
		return nil, err
	}
	if account.Valid {
		v := account.Int64
		item.AccountID = &v
	}
	if assignee.Valid {
		v := assignee.Int64
		item.AssigneeID = &v
	}
	item.Priority = model.ShoppingPriority(priority)
	item.PlannedPrice = money.FromKopecks(planned)
	item.ActualPrice = money.FromKopecks(actual)
	item.InCart = inCart != 0
	item.PriceMismatch = mismatch != 0
	item.Status = model.ShoppingItemStatus(status)
	item.CreatedAt = fromUnix(created)
	item.UpdatedAt = fromUnix(updated)
	return &item, nil
}

// applyShoppingItemPatch применяет частичное изменение к позиции.
func applyShoppingItemPatch(item *model.ShoppingListItem, patch ShoppingItemPatch) {
	if patch.Name != nil {
		item.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.ClearAccount {
		item.AccountID = nil
	} else if patch.AccountID != nil {
		v := *patch.AccountID
		item.AccountID = &v
	}
	if patch.ClearAssignee {
		item.AssigneeID = nil
	} else if patch.AssigneeID != nil {
		v := *patch.AssigneeID
		item.AssigneeID = &v
	}
	if patch.Priority != nil {
		item.Priority = *patch.Priority
	}
	if patch.PlannedPrice != nil {
		item.PlannedPrice = *patch.PlannedPrice
	}
	if patch.ActualPrice != nil {
		item.ActualPrice = *patch.ActualPrice
	}
	if patch.InCart != nil {
		item.InCart = *patch.InCart
	}
	if patch.PriceMismatch != nil {
		item.PriceMismatch = *patch.PriceMismatch
	}
	if patch.Comment != nil {
		item.Comment = strings.TrimSpace(*patch.Comment)
	}
	if patch.Status != nil {
		item.Status = *patch.Status
	}
	if patch.Position != nil {
		item.Position = *patch.Position
	}
}

// validateShoppingItemTx проверяет поля позиции и её ссылки.
func validateShoppingItemTx(ctx context.Context, tx *sql.Tx, in ShoppingItemInput) error {
	if _, err := validateShoppingName(in.Name); err != nil {
		return err
	}
	if in.PlannedPrice.IsNegative() || in.ActualPrice.IsNegative() {
		return shoppingValidation(CodeShoppingPriceInvalid, "цена позиции не может быть отрицательной")
	}
	priority := in.Priority
	if priority == "" {
		priority = model.ShoppingPriorityNormal
	}
	if !priority.Valid() {
		return shoppingValidation(CodeShoppingPriority, "недопустимый приоритет «"+string(priority)+"»")
	}
	status := in.Status
	if status == "" {
		status = model.ShoppingItemPlanned
	}
	if !status.Valid() {
		return shoppingValidation(CodeShoppingStatus, "недопустимый статус позиции «"+string(status)+"»")
	}
	if err := validateShoppingAccountTx(ctx, tx, in.AccountID); err != nil {
		return err
	}
	if err := validateShoppingAssigneeTx(ctx, tx, in.AssigneeID); err != nil {
		return err
	}
	return nil
}

// validateShoppingAccountTx проверяет, что статья расходов — листовой «расход».
func validateShoppingAccountTx(ctx context.Context, tx *sql.Tx, accountID *int64) error {
	if accountID == nil {
		return nil
	}
	acc, err := getAccountTx(ctx, tx, *accountID)
	if err != nil {
		return shoppingValidation(CodeShoppingAccountInvalid, "статья расходов не найдена")
	}
	if acc.IsArchived || !acc.IsLeaf || acc.Type != model.TypeExpense {
		return shoppingValidation(CodeShoppingAccountInvalid,
			"статья расходов должна быть листовым счётом типа «расход»: "+acc.FullPath)
	}
	return nil
}

// validateShoppingAssigneeTx проверяет существование ответственного.
func validateShoppingAssigneeTx(ctx context.Context, tx *sql.Tx, userID *int64) error {
	if userID == nil {
		return nil
	}
	if err := validateUserTx(ctx, tx, *userID); err != nil {
		return shoppingValidation(CodeShoppingAssigneeInvalid, "ответственный не найден")
	}
	return nil
}

// validateUserTx проверяет существование пользователя.
func validateUserTx(ctx context.Context, tx *sql.Tx, userID int64) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&count); err != nil {
		return fmt.Errorf("store: не удалось проверить пользователя: %w", err)
	}
	if count == 0 {
		return shoppingValidation(CodeShoppingAssigneeInvalid, "пользователь не найден")
	}
	return nil
}

// validateShoppingName проверяет наименование списка или позиции.
func validateShoppingName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", shoppingValidation(CodeShoppingName, "наименование не может быть пустым")
	}
	if utf8.RuneCountInString(name) > 200 {
		return "", shoppingValidation(CodeShoppingName, "наименование длиннее 200 символов")
	}
	return name, nil
}

// shoppingValidation создаёт ошибку валидации списка покупок.
func shoppingValidation(code, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}

// shoppingUUID возвращает заданный uuid или генерирует новый.
func shoppingUUID(given string) (string, error) {
	if strings.TrimSpace(given) != "" {
		return strings.TrimSpace(given), nil
	}
	return newShoppingUUID()
}

// newShoppingUUID генерирует новый uuid.
func newShoppingUUID() (string, error) {
	id, err := uuid.New()
	if err != nil {
		return "", fmt.Errorf("store: не удалось создать uuid: %w", err)
	}
	return id, nil
}

// boolInt переводит bool в целое для SQLite.
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// fromUnixOptional переводит unix-секунды в time.Time; 0 — нулевое время.
func fromUnixOptional(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return fromUnix(v)
}
