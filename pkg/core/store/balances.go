package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// Balance возвращает остаток счёта по природе его типа.
//
// Остаток никогда не хранится: он вычисляется свёрткой журнала по формуле
//
//	актив, расход          → ΣDEBIT − ΣCREDIT
//	пассив, капитал, доход → ΣCREDIT − ΣDEBIT
//
// Отрицательное значение означает остаток, противоположный природе счёта.
func (db *DB) Balance(ctx context.Context, accountID int64) (money.Kop, error) {
	var accountType string
	err := db.QueryRowContext(ctx, `SELECT type FROM accounts WHERE id = ?`, accountID).Scan(&accountType)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: счёт %d", ErrNotFound, accountID)
	}
	if err != nil {
		return 0, fmt.Errorf("store: не удалось прочитать тип счёта: %w", err)
	}

	var debit, credit int64
	if err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN side = 'DEBIT'  THEN amount_kop ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN side = 'CREDIT' THEN amount_kop ELSE 0 END), 0)
		  FROM transaction_lines
		 WHERE account_id = ?`, accountID).Scan(&debit, &credit); err != nil {
		return 0, fmt.Errorf("store: не удалось посчитать остаток счёта: %w", err)
	}
	return balanceFor(model.AccountType(accountType), money.FromKopecks(debit), money.FromKopecks(credit)), nil
}

// Balances возвращает остатки всех счетов, включая нулевые.
func (db *DB) Balances(ctx context.Context) (map[int64]money.Kop, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.type,
			COALESCE(SUM(CASE WHEN l.side = 'DEBIT'  THEN l.amount_kop ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN l.side = 'CREDIT' THEN l.amount_kop ELSE 0 END), 0)
		  FROM accounts a
		  LEFT JOIN transaction_lines l ON l.account_id = a.id
		 GROUP BY a.id, a.type`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось посчитать остатки: %w", err)
	}
	defer rows.Close()

	balances := map[int64]money.Kop{}
	for rows.Next() {
		var (
			id          int64
			accountType string
			debit       int64
			credit      int64
		)
		if err := rows.Scan(&id, &accountType, &debit, &credit); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать остаток: %w", err)
		}
		balances[id] = balanceFor(model.AccountType(accountType),
			money.FromKopecks(debit), money.FromKopecks(credit))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения остатков: %w", err)
	}
	return balances, nil
}

// SubtreeBalance возвращает суммарный остаток поддерева по префиксу пути.
//
// Каждый счёт складывается по природе своего типа, поэтому итог имеет смысл
// для однородных ветвей (например «Активы»).
func (db *DB) SubtreeBalance(ctx context.Context, prefix string) (money.Kop, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.type,
			COALESCE(SUM(CASE WHEN l.side = 'DEBIT'  THEN l.amount_kop ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN l.side = 'CREDIT' THEN l.amount_kop ELSE 0 END), 0)
		  FROM accounts a
		  LEFT JOIN transaction_lines l ON l.account_id = a.id
		 WHERE a.full_path = ? OR a.full_path LIKE ? ESCAPE '\'
		 GROUP BY a.id, a.type`,
		prefix, escapeLike(prefix)+pathSeparator+"%")
	if err != nil {
		return 0, fmt.Errorf("store: не удалось посчитать остаток поддерева: %w", err)
	}
	defer rows.Close()

	var total money.Kop
	for rows.Next() {
		var (
			accountType string
			debit       int64
			credit      int64
		)
		if err := rows.Scan(&accountType, &debit, &credit); err != nil {
			return 0, fmt.Errorf("store: не удалось разобрать остаток поддерева: %w", err)
		}
		total += balanceFor(model.AccountType(accountType),
			money.FromKopecks(debit), money.FromKopecks(credit))
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("store: ошибка чтения остатков поддерева: %w", err)
	}
	return total, nil
}

// balanceFor переводит дебетовый и кредитовый обороты в остаток по природе типа.
func balanceFor(t model.AccountType, debit, credit money.Kop) money.Kop {
	switch t {
	case model.TypeAsset, model.TypeExpense:
		return debit - credit
	default: // пассив, капитал, доход
		return credit - debit
	}
}
