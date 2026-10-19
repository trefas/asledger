package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// pathSeparator — разделитель уровней плана счетов (как в hledger).
const pathSeparator = ":"

// accountColumns — единый список колонок для чтения счёта.
const accountColumns = `id, name, full_path, type, parent_id, is_leaf,
	min_balance, is_archived, is_system, created_at`

// AccountInput — данные для создания счёта.
type AccountInput struct {
	// Name — короткое имя счёта (кириллица, без разделителя «:»).
	Name string
	// Type — тип счёта.
	Type model.AccountType
	// ParentID — родительский счёт; nil для корневого.
	ParentID *int64
	// MinBalance — минимально допустимый остаток (0 или отрицательный).
	MinBalance money.Kop
}

// AccountFilter — условия выборки счетов.
type AccountFilter struct {
	// Prefix ограничивает выборку поддеревом по префиксу пути.
	Prefix string
	// OnlyLeaves возвращает только листовые счета.
	OnlyLeaves bool
	// IncludeArchived включает заархивированные счета.
	IncludeArchived bool
}

// rowScanner объединяет *sql.Row и *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// CreateAccount создаёт счёт, вычисляя полный путь по родителю.
func (db *DB) CreateAccount(ctx context.Context, in AccountInput) (*model.Account, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validateAccountName(in.Name); err != nil {
		return nil, err
	}
	if !in.Type.Valid() {
		return nil, fmt.Errorf("store: недопустимый тип счёта %q", in.Type)
	}

	var created *model.Account
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		fullPath := in.Name
		if in.ParentID != nil {
			parent, err := getAccountTx(ctx, tx, *in.ParentID)
			if err != nil {
				return err
			}
			if parent.IsArchived {
				return fmt.Errorf("%w: %s", ErrAccountArchived, parent.FullPath)
			}
			if parent.Type != in.Type {
				return &ValidationError{
					Code:    CodeAccountTypeMismatch,
					Message: fmt.Sprintf("тип счёта %q не совпадает с типом родителя %q (%s)", in.Type, parent.FullPath, parent.Type),
				}
			}
			fullPath = parent.FullPath + pathSeparator + in.Name
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO accounts(name, full_path, type, parent_id, is_leaf, min_balance, is_archived, is_system, created_at)
			 VALUES(?, ?, ?, ?, 1, ?, 0, 0, ?)`,
			in.Name, fullPath, string(in.Type), nullableID(in.ParentID), int64(in.MinBalance), Now())
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: %s", ErrDuplicatePath, fullPath)
			}
			return fmt.Errorf("store: не удалось создать счёт %q: %w", fullPath, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: не удалось получить id счёта: %w", err)
		}
		if err := db.rebuildPathsTx(ctx, tx); err != nil {
			return err
		}
		if err := appendChangelog(ctx, tx, model.EntityAccount, id, model.OpInsert); err != nil {
			return err
		}
		created = &model.Account{
			ID: id, Name: in.Name, FullPath: fullPath, Type: in.Type,
			ParentID: in.ParentID, IsLeaf: true, MinBalance: in.MinBalance, CreatedAt: fromUnix(Now()),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Account возвращает счёт по идентификатору.
func (db *DB) Account(ctx context.Context, id int64) (*model.Account, error) {
	row := db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id)
	acc, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: счёт %d", ErrNotFound, id)
	}
	return acc, err
}

// AccountByPath возвращает счёт по полному пути.
func (db *DB) AccountByPath(ctx context.Context, fullPath string) (*model.Account, error) {
	row := db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE full_path = ?`, fullPath)
	acc, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: счёт %q", ErrNotFound, fullPath)
	}
	return acc, err
}

// ListAccounts возвращает счета по условиям фильтра в порядке пути.
func (db *DB) ListAccounts(ctx context.Context, f AccountFilter) ([]model.Account, error) {
	query := `SELECT ` + accountColumns + ` FROM accounts WHERE 1 = 1`
	var args []any

	if !f.IncludeArchived {
		query += ` AND is_archived = 0`
	}
	if f.OnlyLeaves {
		query += ` AND is_leaf = 1`
	}
	if f.Prefix != "" {
		escaped := escapeLike(f.Prefix)
		query += ` AND (full_path = ? OR full_path LIKE ? ESCAPE '\')`
		args = append(args, f.Prefix, escaped+pathSeparator+"%")
	}
	query += ` ORDER BY full_path`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать план счетов: %w", err)
	}
	defer rows.Close()

	var accounts []model.Account
	for rows.Next() {
		acc, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать счёт: %w", err)
		}
		accounts = append(accounts, *acc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения плана счетов: %w", err)
	}
	return accounts, nil
}

// RenameAccount переименовывает счёт и пересчитывает пути потомков.
func (db *DB) RenameAccount(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if err := validateAccountName(name); err != nil {
		return err
	}
	return db.Tx(ctx, func(tx *sql.Tx) error {
		acc, err := getAccountTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if acc.IsSystem {
			return fmt.Errorf("%w: %s", ErrSystemAccount, acc.FullPath)
		}
		if acc.Name == name {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET name = ? WHERE id = ?`, name, id); err != nil {
			return fmt.Errorf("store: не удалось переименовать счёт: %w", err)
		}
		if err := db.rebuildPathsTx(ctx, tx); err != nil {
			return err
		}
		return appendChangelog(ctx, tx, model.EntityAccount, id, model.OpUpdate)
	})
}

// MoveAccount переносит счёт к другому родителю (nil — в корень).
func (db *DB) MoveAccount(ctx context.Context, id int64, parentID *int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		acc, err := getAccountTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if acc.IsSystem {
			return fmt.Errorf("%w: %s", ErrSystemAccount, acc.FullPath)
		}
		if parentID != nil {
			if *parentID == id {
				return ErrMoveIntoDescendant
			}
			target, err := getAccountTx(ctx, tx, *parentID)
			if err != nil {
				return err
			}
			if target.IsArchived {
				return fmt.Errorf("%w: %s", ErrAccountArchived, target.FullPath)
			}
			if isDescendantPath(acc.FullPath, target.FullPath) {
				return fmt.Errorf("%w: %s → %s", ErrMoveIntoDescendant, acc.FullPath, target.FullPath)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET parent_id = ? WHERE id = ?`, nullableID(parentID), id); err != nil {
			return fmt.Errorf("store: не удалось переместить счёт: %w", err)
		}
		if err := db.rebuildPathsTx(ctx, tx); err != nil {
			return err
		}
		return appendChangelog(ctx, tx, model.EntityAccount, id, model.OpUpdate)
	})
}

// SetMinBalance задаёт минимально допустимый остаток счёта.
func (db *DB) SetMinBalance(ctx context.Context, id int64, minBalance money.Kop) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		acc, err := getAccountTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if acc.IsSystem {
			return fmt.Errorf("%w: %s", ErrSystemAccount, acc.FullPath)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET min_balance = ? WHERE id = ?`, int64(minBalance), id); err != nil {
			return fmt.Errorf("store: не удалось изменить min_balance: %w", err)
		}
		return appendChangelog(ctx, tx, model.EntityAccount, id, model.OpUpdate)
	})
}

// ArchiveAccount архивирует счёт (без удаления) вместе с проверками.
func (db *DB) ArchiveAccount(ctx context.Context, id int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		acc, err := getAccountTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if acc.IsSystem {
			return fmt.Errorf("%w: %s", ErrSystemAccount, acc.FullPath)
		}

		var children int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM accounts WHERE parent_id = ? AND is_archived = 0`, id).Scan(&children); err != nil {
			return fmt.Errorf("store: не удалось проверить потомков счёта: %w", err)
		}
		if children > 0 {
			return fmt.Errorf("%w: %s", ErrAccountHasChildren, acc.FullPath)
		}

		if err := hasOperationsTx(ctx, tx, acc.FullPath); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE accounts SET is_archived = 1 WHERE id = ?`, id); err != nil {
			return fmt.Errorf("store: не удалось заархивировать счёт: %w", err)
		}
		return appendChangelog(ctx, tx, model.EntityAccount, id, model.OpArchive)
	})
}

// hasOperationsTx сообщает ошибкой, есть ли проводки по поддереву счёта.
func hasOperationsTx(ctx context.Context, tx *sql.Tx, fullPath string) error {
	var count int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction_lines l
		   JOIN accounts a ON a.id = l.account_id
		  WHERE a.full_path = ? OR a.full_path LIKE ? ESCAPE '\'`,
		fullPath, escapeLike(fullPath)+pathSeparator+"%").Scan(&count)
	if err != nil {
		return fmt.Errorf("store: не удалось проверить проводки счёта: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: %s", ErrAccountHasOperations, fullPath)
	}
	return nil
}

// rebuildPaths пересчитывает пути и признак листа для всего плана счетов.
func (db *DB) rebuildPaths(ctx context.Context) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		return db.rebuildPathsTx(ctx, tx)
	})
}

// rebuildPathsTx пересчитывает пути в рамках уже открытой транзакции.
func (db *DB) rebuildPathsTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, parent_id FROM accounts`)
	if err != nil {
		return fmt.Errorf("store: не удалось прочитать дерево счетов: %w", err)
	}
	defer rows.Close()

	type node struct {
		name   string
		parent *int64
	}
	nodes := map[int64]node{}
	children := map[int64][]int64{}
	var roots []int64

	for rows.Next() {
		var id int64
		var name string
		var parent sql.NullInt64
		if err := rows.Scan(&id, &name, &parent); err != nil {
			return fmt.Errorf("store: не удалось разобрать дерево счетов: %w", err)
		}
		n := node{name: name}
		if parent.Valid {
			pid := parent.Int64
			n.parent = &pid
			children[pid] = append(children[pid], id)
		} else {
			roots = append(roots, id)
		}
		nodes[id] = n
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: ошибка чтения дерева счетов: %w", err)
	}

	paths := make(map[int64]string, len(nodes))
	var walk func(id int64, parentPath string)
	walk = func(id int64, parentPath string) {
		fullPath := nodes[id].name
		if parentPath != "" {
			fullPath = parentPath + pathSeparator + nodes[id].name
		}
		paths[id] = fullPath
		for _, child := range children[id] {
			walk(child, fullPath)
		}
	}
	for _, root := range roots {
		walk(root, "")
	}

	for id := range nodes {
		fullPath, ok := paths[id]
		if !ok {
			continue // узел вне дерева (цикл) — не трогаем
		}
		isLeaf := 0
		if len(children[id]) == 0 {
			isLeaf = 1
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE accounts SET full_path = ?, is_leaf = ? WHERE id = ?`, fullPath, isLeaf, id); err != nil {
			return fmt.Errorf("store: не удалось обновить путь счёта %d: %w", id, err)
		}
	}
	return nil
}

// isDescendantPath сообщает, лежит ли path внутри base.
func isDescendantPath(base, path string) bool {
	return path == base || strings.HasPrefix(path, base+pathSeparator)
}

// validateAccountName проверяет короткое имя счёта.
func validateAccountName(name string) error {
	switch {
	case name == "":
		return errors.New("store: имя счёта не может быть пустым")
	case strings.Contains(name, pathSeparator):
		return fmt.Errorf("store: имя счёта не может содержать %q", pathSeparator)
	default:
		return nil
	}
}

// getAccountTx читает счёт по идентификатору в рамках транзакции.
func getAccountTx(ctx context.Context, tx *sql.Tx, id int64) (*model.Account, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = ?`, id)
	acc, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: счёт %d", ErrNotFound, id)
	}
	return acc, err
}

// scanAccount разбирает строку выборки счёта.
func scanAccount(s rowScanner) (*model.Account, error) {
	var (
		acc         model.Account
		typ         string
		parent      sql.NullInt64
		isLeaf      int
		minBalance  int64
		isArchived  int
		isSystem    int
		createdUnix int64
	)
	if err := s.Scan(&acc.ID, &acc.Name, &acc.FullPath, &typ, &parent,
		&isLeaf, &minBalance, &isArchived, &isSystem, &createdUnix); err != nil {
		return nil, err
	}
	if parent.Valid {
		pid := parent.Int64
		acc.ParentID = &pid
	}
	acc.Type = model.AccountType(typ)
	acc.IsLeaf = isLeaf == 1
	acc.MinBalance = money.FromKopecks(minBalance)
	acc.IsArchived = isArchived == 1
	acc.IsSystem = isSystem == 1
	acc.CreatedAt = fromUnix(createdUnix)
	return &acc, nil
}

// SuggestAccounts ищет счета по частичному вводу без учёта регистра.
//
// Сравнение выполняется в Go (strings.ToLower), потому что SQLite NOCASE не
// сворачивает регистр кириллицы. Сначала идут листовые счета (пригодные для
// проводки), затем папки; внутри группы — совпадения по имени, затем по пути.
// Пустой запрос возвращает листовые счета по алфавиту.
func (db *DB) SuggestAccounts(ctx context.Context, query string, limit int) ([]model.Account, error) {
	if limit <= 0 {
		limit = 20
	}
	accounts, err := db.ListAccounts(ctx, AccountFilter{})
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(strings.TrimSpace(query))
	type ranked struct {
		acc  model.Account
		leaf int
		kind int
	}
	matches := make([]ranked, 0, len(accounts))
	for _, acc := range accounts {
		leaf := 1
		if acc.IsLeaf {
			leaf = 0
		}
		kind := 0
		if q != "" {
			name := strings.ToLower(acc.Name)
			path := strings.ToLower(acc.FullPath)
			switch {
			case strings.HasPrefix(name, q):
				kind = 0
			case strings.Contains(name, q):
				kind = 1
			case strings.Contains(path, q):
				kind = 2
			default:
				continue
			}
		}
		matches = append(matches, ranked{acc: acc, leaf: leaf, kind: kind})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].leaf != matches[j].leaf {
			return matches[i].leaf < matches[j].leaf
		}
		if matches[i].kind != matches[j].kind {
			return matches[i].kind < matches[j].kind
		}
		if len(matches[i].acc.FullPath) != len(matches[j].acc.FullPath) {
			return len(matches[i].acc.FullPath) < len(matches[j].acc.FullPath)
		}
		return matches[i].acc.FullPath < matches[j].acc.FullPath
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]model.Account, 0, len(matches))
	for _, m := range matches {
		result = append(result, m.acc)
	}
	return result, nil
}
