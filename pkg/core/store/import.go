package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
)

// AccountSpec — счёт, который нужен импорту.
type AccountSpec struct {
	// Path — полный путь счёта (например, «Расходы:Продукты»).
	Path string
	// Type — тип счёта; у потомка он совпадает с типом родителя.
	Type model.AccountType
}

// EnsureAccounts создаёт недостающие счета плана и возвращает все счёта по пути.
//
// Существующие счета не меняются — импорт дополняет план, но не переписывает
// его. Счета создаются одним действием вместе с пересчётом путей и записью в
// changelog; счета-предки должны быть перечислены в specs (импорт всегда
// передаёт всю цепочку путей).
func (db *DB) EnsureAccounts(ctx context.Context, specs []AccountSpec) (map[string]*model.Account, []model.Account, error) {
	ordered := append([]AccountSpec(nil), specs...)
	sort.Slice(ordered, func(i, j int) bool {
		di, dj := strings.Count(ordered[i].Path, pathSeparator), strings.Count(ordered[j].Path, pathSeparator)
		if di != dj {
			return di < dj
		}
		return ordered[i].Path < ordered[j].Path
	})
	for _, spec := range ordered {
		if strings.TrimSpace(spec.Path) == "" {
			return nil, nil, fmt.Errorf("store: пустой путь счёта в импорте")
		}
		if !spec.Type.Valid() {
			return nil, nil, fmt.Errorf("store: недопустимый тип счёта %q для пути %q", spec.Type, spec.Path)
		}
		if strings.HasSuffix(spec.Path, pathSeparator) || strings.HasPrefix(spec.Path, pathSeparator) ||
			strings.Contains(spec.Path, pathSeparator+pathSeparator) {
			return nil, nil, fmt.Errorf("store: неверный путь счёта %q", spec.Path)
		}
	}

	byPath := make(map[string]*model.Account, len(specs))
	var created []model.Account

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		if err := loadAccountsTx(ctx, tx, byPath); err != nil {
			return err
		}
		madeAny := false
		for _, spec := range ordered {
			if _, ok := byPath[spec.Path]; ok {
				continue
			}

			parentPath, name := parentPathOf(spec.Path)
			var parentID *int64
			if parentPath != "" {
				parent, ok := byPath[parentPath]
				if !ok {
					return fmt.Errorf("store: для счёта %q не найден родитель %q", spec.Path, parentPath)
				}
				if parent.IsArchived {
					return fmt.Errorf("%w: %s", ErrAccountArchived, parent.FullPath)
				}
				if parent.Type != spec.Type {
					return &ValidationError{
						Code:    CodeAccountTypeMismatch,
						Message: fmt.Sprintf("тип счёта %q не совпадает с типом родителя %q (%s)", spec.Type, parent.FullPath, parent.Type),
					}
				}
				id := parent.ID
				parentID = &id
			}

			res, err := tx.ExecContext(ctx,
				`INSERT INTO accounts(name, full_path, type, parent_id, is_leaf, min_balance, is_archived, is_system, created_at)
				 VALUES(?, ?, ?, ?, 1, 0, 0, 0, ?)`,
				name, spec.Path, string(spec.Type), nullableID(parentID), Now())
			if err != nil {
				if isUniqueViolation(err) {
					return fmt.Errorf("%w: %s", ErrDuplicatePath, spec.Path)
				}
				return fmt.Errorf("store: не удалось создать счёт %q: %w", spec.Path, err)
			}
			id, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("store: не удалось получить id счёта: %w", err)
			}
			if err := appendChangelog(ctx, tx, model.EntityAccount, id, model.OpInsert); err != nil {
				return err
			}
			account := &model.Account{
				ID: id, Name: name, FullPath: spec.Path, Type: spec.Type,
				ParentID: parentID, IsLeaf: true, CreatedAt: fromUnix(Now()),
			}
			byPath[spec.Path] = account
			created = append(created, *account)
			madeAny = true
		}
		if madeAny {
			return db.rebuildPathsTx(ctx, tx)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return byPath, created, nil
}

// ImportedHashes возвращает множество хешей содержимого уже импортированных
// проводок указанного источника (идемпотентность повторного импорта).
func (db *DB) ImportedHashes(ctx context.Context, source string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT source_hash FROM transactions
		  WHERE source = ? AND source_hash IS NOT NULL AND source_hash <> ''`, source)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать хеши импорта: %w", err)
	}
	defer rows.Close()

	hashes := make(map[string]bool)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать хеш импорта: %w", err)
		}
		hashes[hash] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения хешей импорта: %w", err)
	}
	return hashes, nil
}

// loadAccountsTx читает все счета в карту «путь → счёт» в рамках транзакции.
func loadAccountsTx(ctx context.Context, tx *sql.Tx, into map[string]*model.Account) error {
	rows, err := tx.QueryContext(ctx, `SELECT `+accountColumns+` FROM accounts`)
	if err != nil {
		return fmt.Errorf("store: не удалось прочитать план счетов: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return fmt.Errorf("store: не удалось разобрать счёт: %w", err)
		}
		into[account.FullPath] = account
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: ошибка чтения плана счетов: %w", err)
	}
	return nil
}

// parentPathOf делит путь на путь родителя и короткое имя счёта.
func parentPathOf(path string) (string, string) {
	if idx := strings.LastIndex(path, pathSeparator); idx >= 0 {
		return path[:idx], path[idx+1:]
	}
	return "", path
}
