package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
)

// systemAccount описывает служебный счёт, создаваемый при первом запуске.
type systemAccount struct {
	path string
	typ  model.AccountType
}

// Служебные счета из раздела 6 эпика: строка капитала генерируется движком,
// ветка «Активы:Дебиторка» используется операцией выдачи займа.
var systemAccounts = []systemAccount{
	{path: "Капитал:Начальные", typ: model.TypeCapital},
	{path: "Капитал:Корректировка", typ: model.TypeCapital},
	{path: "Капитал:Переоценка", typ: model.TypeCapital},
	{path: "Активы:Дебиторка", typ: model.TypeAsset},
}

// EnsureSystemAccounts создаёт служебные счета и их родительские ветки,
// если они ещё не созданы. Вызывается при первом запуске.
func (db *DB) EnsureSystemAccounts(ctx context.Context) error {
	for _, sa := range systemAccounts {
		if err := db.ensureSystemPath(ctx, sa.path, sa.typ); err != nil {
			return err
		}
	}
	return db.rebuildPaths(ctx)
}

// ensureSystemPath создаёт недостающие узлы пути, помечая их служебными.
func (db *DB) ensureSystemPath(ctx context.Context, path string, typ model.AccountType) error {
	segments := strings.Split(path, pathSeparator)
	var parentID *int64
	current := ""

	for _, segment := range segments {
		if current == "" {
			current = segment
		} else {
			current = current + pathSeparator + segment
		}

		id, err := db.accountIDByPath(ctx, current)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err == nil {
			parentID = &id
			continue
		}

		res, err := db.ExecContext(ctx,
			`INSERT INTO accounts(name, full_path, type, parent_id, is_leaf, min_balance, is_archived, is_system, created_at)
			 VALUES(?, ?, ?, ?, 1, 0, 0, 1, ?)`,
			segment, current, string(typ), nullableID(parentID), Now())
		if err != nil {
			return fmt.Errorf("store: не удалось создать служебный счёт %q: %w", current, err)
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: не удалось получить id служебного счёта %q: %w", current, err)
		}
		parentID = &newID
	}
	return nil
}

// accountIDByPath возвращает идентификатор счёта по полному пути.
func (db *DB) accountIDByPath(ctx context.Context, fullPath string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM accounts WHERE full_path = ?`, fullPath).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: не удалось найти счёт %q: %w", fullPath, err)
	}
	return id, nil
}
