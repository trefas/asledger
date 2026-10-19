package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// reconciliationColumns — единый список колонок для чтения сверки.
const reconciliationColumns = `id, account_id, fact_kop, expected_kop, checked_at, transaction_id`

// RecordReconciliation сохраняет результат сверки счёта.
func (db *DB) RecordReconciliation(ctx context.Context, rec *model.Reconciliation) (int64, error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO reconciliations(account_id, fact_kop, expected_kop, checked_at, transaction_id)
		 VALUES(?, ?, ?, ?, ?)`,
		rec.AccountID, int64(rec.Fact), int64(rec.Expected), toUnix(rec.CheckedAt), nullableID(rec.TransactionID))
	if err != nil {
		return 0, fmt.Errorf("store: не удалось сохранить сверку: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: не удалось получить id сверки: %w", err)
	}
	return id, nil
}

// LatestReconciliations возвращает последнюю сверку по каждому счёту.
func (db *DB) LatestReconciliations(ctx context.Context) ([]model.Reconciliation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+reconciliationColumns+`
		  FROM reconciliations r
		 WHERE r.id = (SELECT MAX(r2.id) FROM reconciliations r2 WHERE r2.account_id = r.account_id)
		 ORDER BY r.account_id`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать сверки: %w", err)
	}
	defer rows.Close()

	var out []model.Reconciliation
	for rows.Next() {
		rec, err := scanReconciliation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать сверку: %w", err)
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения сверок: %w", err)
	}
	return out, nil
}

// ReconciliationHistory возвращает историю сверок счёта (свежие первыми).
func (db *DB) ReconciliationHistory(ctx context.Context, accountID int64) ([]model.Reconciliation, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+reconciliationColumns+` FROM reconciliations WHERE account_id = ? ORDER BY id DESC`,
		accountID)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать историю сверок: %w", err)
	}
	defer rows.Close()

	var out []model.Reconciliation
	for rows.Next() {
		rec, err := scanReconciliation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать сверку: %w", err)
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения истории сверок: %w", err)
	}
	return out, nil
}

// scanReconciliation разбирает строку выборки сверки.
func scanReconciliation(s rowScanner) (*model.Reconciliation, error) {
	var (
		rec         model.Reconciliation
		fact        int64
		expected    int64
		checkedAt   int64
		transaction sql.NullInt64
	)
	if err := s.Scan(&rec.ID, &rec.AccountID, &fact, &expected, &checkedAt, &transaction); err != nil {
		return nil, err
	}
	rec.Fact = money.FromKopecks(fact)
	rec.Expected = money.FromKopecks(expected)
	rec.CheckedAt = fromUnix(checkedAt)
	if transaction.Valid {
		id := transaction.Int64
		rec.TransactionID = &id
	}
	return &rec, nil
}
