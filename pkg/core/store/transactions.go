package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// transactionColumns — единый список колонок для чтения транзакции.
const transactionColumns = `id, uuid, op_type, date, description, actor_id,
	reverses_id, source, source_hash, created_at, commit_id`

// TransactionFilter — условия выборки транзакций.
type TransactionFilter struct {
	// ActorID ограничивает выборку актором.
	ActorID *int64
	// AccountID ограничивает выборку проводками по счёту.
	AccountID *int64
	// Source ограничивает выборку источником ('hledger', 'web', 'api:…').
	Source string
	// From и To задают период по дате (включительно); нуль — без ограничения.
	From int64
	To   int64
	// Limit ограничивает число записей (0 — без ограничения).
	Limit int
}

// InsertTransaction сохраняет транзакцию вместе со строками.
//
// Проверяются сквозные инварианты: положительные суммы, допустимые стороны
// и равенство ΣDEBIT = ΣCREDIT. Повтор с тем же uuid отклоняется ошибкой
// ErrDuplicateUUID (защита от повторной отправки коммита).
func (db *DB) InsertTransaction(ctx context.Context, tr *model.Transaction) (int64, error) {
	if err := validateTransaction(tr); err != nil {
		return 0, err
	}

	var id int64
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		var existing int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM transactions WHERE uuid = ?`, tr.UUID).Scan(&existing)
		if err == nil {
			return fmt.Errorf("%w: %s", ErrDuplicateUUID, tr.UUID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: не удалось проверить uuid транзакции: %w", err)
		}
		if err := validateLineAccountsTx(ctx, tx, tr.Lines); err != nil {
			return err
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO transactions(uuid, op_type, date, description, actor_id, reverses_id, source, source_hash, created_at, commit_id)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tr.UUID, string(tr.OpType), toUnix(tr.Date), tr.Description, tr.ActorID,
			nullableID(tr.ReversesID), nullString(tr.Source), nullString(tr.SourceHash),
			Now(), tr.CommitID)
		if err != nil {
			return fmt.Errorf("store: не удалось создать транзакцию: %w", err)
		}
		id, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: не удалось получить id транзакции: %w", err)
		}

		for _, line := range tr.Lines {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO transaction_lines(transaction_id, account_id, amount_kop, side) VALUES(?, ?, ?, ?)`,
				id, line.AccountID, int64(line.Amount), string(line.Side)); err != nil {
				return fmt.Errorf("store: не удалось создать строку проводки: %w", err)
			}
		}
		return appendChangelog(ctx, tx, model.EntityTransaction, id, model.OpInsert)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// Transaction возвращает транзакцию со строками по идентификатору.
func (db *DB) Transaction(ctx context.Context, id int64) (*model.Transaction, error) {
	tr, err := scanTransaction(db.QueryRowContext(ctx,
		`SELECT `+transactionColumns+` FROM transactions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: транзакция %d", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	if err := db.loadLines(ctx, tr); err != nil {
		return nil, err
	}
	return tr, nil
}

// TransactionByUUID возвращает транзакцию по uuid (идемпотентность клиента).
func (db *DB) TransactionByUUID(ctx context.Context, uuid string) (*model.Transaction, error) {
	tr, err := scanTransaction(db.QueryRowContext(ctx,
		`SELECT `+transactionColumns+` FROM transactions WHERE uuid = ?`, uuid))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: транзакция %s", ErrNotFound, uuid)
	}
	if err != nil {
		return nil, err
	}
	if err := db.loadLines(ctx, tr); err != nil {
		return nil, err
	}
	return tr, nil
}

// ListTransactions возвращает транзакции по условиям фильтра (свежие первыми).
func (db *DB) ListTransactions(ctx context.Context, f TransactionFilter) ([]model.Transaction, error) {
	query := `SELECT ` + transactionColumns + ` FROM transactions t WHERE 1 = 1`
	var args []any

	if f.ActorID != nil {
		query += ` AND t.actor_id = ?`
		args = append(args, *f.ActorID)
	}
	if f.AccountID != nil {
		query += ` AND EXISTS (SELECT 1 FROM transaction_lines l WHERE l.transaction_id = t.id AND l.account_id = ?)`
		args = append(args, *f.AccountID)
	}
	if f.Source != "" {
		query += ` AND t.source = ?`
		args = append(args, f.Source)
	}
	if f.From > 0 {
		query += ` AND t.date >= ?`
		args = append(args, f.From)
	}
	if f.To > 0 {
		query += ` AND t.date <= ?`
		args = append(args, f.To)
	}
	query += ` ORDER BY t.date DESC, t.id DESC`
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать транзакции: %w", err)
	}
	defer rows.Close()

	var transactions []model.Transaction
	for rows.Next() {
		tr, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать транзакцию: %w", err)
		}
		transactions = append(transactions, *tr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения транзакций: %w", err)
	}
	for i := range transactions {
		if err := db.loadLines(ctx, &transactions[i]); err != nil {
			return nil, err
		}
	}
	return transactions, nil
}

// loadLines дополняет транзакцию строками проводки.
func (db *DB) loadLines(ctx context.Context, tr *model.Transaction) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, transaction_id, account_id, amount_kop, side
		   FROM transaction_lines WHERE transaction_id = ? ORDER BY id`, tr.ID)
	if err != nil {
		return fmt.Errorf("store: не удалось прочитать строки проводки: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			line   model.TransactionLine
			amount int64
			side   string
		)
		if err := rows.Scan(&line.ID, &line.TransactionID, &line.AccountID, &amount, &side); err != nil {
			return fmt.Errorf("store: не удалось разобрать строку проводки: %w", err)
		}
		line.Amount = money.FromKopecks(amount)
		line.Side = model.Side(side)
		tr.Lines = append(tr.Lines, line)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: ошибка чтения строк проводки: %w", err)
	}
	return nil
}

// validateTransaction проверяет сквозные инварианты проводки.
func validateTransaction(tr *model.Transaction) error {
	if tr.UUID == "" {
		return &ValidationError{Code: "ERR_UUID_EMPTY", Message: "не задан uuid транзакции"}
	}
	if !tr.OpType.Valid() {
		return &ValidationError{Code: "ERR_OP_TYPE_UNKNOWN", Message: fmt.Sprintf("недопустимый тип операции %q", tr.OpType)}
	}
	if len(tr.Lines) == 0 {
		return &ValidationError{Code: CodeNoLines, Message: "в проводке нет строк"}
	}

	var debit, credit money.Kop
	for i, line := range tr.Lines {
		if line.Amount <= 0 {
			return &ValidationError{
				Code:    CodeAmountNotPositive,
				Message: fmt.Sprintf("строка %d: сумма %s должна быть положительной", i+1, line.Amount),
			}
		}
		if !line.Side.Valid() {
			return &ValidationError{
				Code:    CodeSideInvalid,
				Message: fmt.Sprintf("строка %d: недопустимая сторона %q", i+1, line.Side),
			}
		}
		if line.Side == model.SideDebit {
			debit += line.Amount
		} else {
			credit += line.Amount
		}
	}
	if debit != credit {
		return &ValidationError{
			Code:    CodeUnbalanced,
			Message: fmt.Sprintf("дебет %s не равен кредиту %s", debit, credit),
		}
	}
	return nil
}

// validateLineAccountsTx проверяет сквозной инвариант 2: счета строк —
// только существующие, листовые и не заархивированные.
func validateLineAccountsTx(ctx context.Context, tx *sql.Tx, lines []model.TransactionLine) error {
	for i, line := range lines {
		acc, err := getAccountTx(ctx, tx, line.AccountID)
		if errors.Is(err, ErrNotFound) {
			return &ValidationError{
				Code:    CodeAccountNotFound,
				Message: fmt.Sprintf("строка %d: счёт %d не существует", i+1, line.AccountID),
			}
		}
		if err != nil {
			return err
		}
		if acc.IsArchived {
			return &ValidationError{
				Code:    CodeAccountArchived,
				Message: fmt.Sprintf("строка %d: счёт %q заархивирован", i+1, acc.FullPath),
			}
		}
		if !acc.IsLeaf {
			return &ValidationError{
				Code:    CodeAccountNotLeaf,
				Message: fmt.Sprintf("строка %d: счёт %q — папка, выберите листовой счёт", i+1, acc.FullPath),
			}
		}
	}
	return nil
}

// scanTransaction разбирает строку выборки транзакции.
func scanTransaction(s rowScanner) (*model.Transaction, error) {
	var (
		tr          model.Transaction
		opType      string
		date        int64
		description sql.NullString
		reverses    sql.NullInt64
		source      sql.NullString
		sourceHash  sql.NullString
		createdAt   int64
	)
	if err := s.Scan(&tr.ID, &tr.UUID, &opType, &date, &description, &tr.ActorID,
		&reverses, &source, &sourceHash, &createdAt, &tr.CommitID); err != nil {
		return nil, err
	}
	tr.OpType = model.OpType(opType)
	tr.Date = fromUnix(date)
	tr.Description = description.String
	if reverses.Valid {
		id := reverses.Int64
		tr.ReversesID = &id
	}
	tr.Source = source.String
	tr.SourceHash = sourceHash.String
	tr.CreatedAt = fromUnix(createdAt)
	return &tr, nil
}

// nullString превращает пустую строку в NULL.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
