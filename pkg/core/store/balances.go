package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	var (
		accountType   string
		debit, credit int64
	)
	// Тип счёта и обороты читаются одним запросом: для пустого счёта LEFT JOIN
	// даёт одну строку с нулевыми суммами, а отсутствие счёта — sql.ErrNoRows.
	err := db.QueryRowContext(ctx, `
		SELECT a.type,
			COALESCE(SUM(CASE WHEN l.side = 'DEBIT'  THEN l.amount_kop ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN l.side = 'CREDIT' THEN l.amount_kop ELSE 0 END), 0)
		  FROM accounts a
		  LEFT JOIN transaction_lines l ON l.account_id = a.id
		 WHERE a.id = ?
		 GROUP BY a.id, a.type`, accountID).Scan(&accountType, &debit, &credit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: счёт %d", ErrNotFound, accountID)
	}
	if err != nil {
		return 0, fmt.Errorf("store: не удалось посчитать остаток счёта: %w", err)
	}
	return balanceFor(model.AccountType(accountType), money.FromKopecks(debit), money.FromKopecks(credit)), nil
}

// BalanceAt возвращает остаток счёта на начало дня date.
//
// Учитываются проводки с датой строго раньше date: движение в сам день date
// относится к расчётному периоду, начинающемуся с этой даты.
func (db *DB) BalanceAt(ctx context.Context, accountID int64, date time.Time) (money.Kop, error) {
	if date.IsZero() {
		return db.Balance(ctx, accountID)
	}
	var (
		accountType   string
		debit, credit int64
	)
	err := db.QueryRowContext(ctx, `
		SELECT a.type,
			COALESCE((SELECT SUM(l.amount_kop)
			            FROM transaction_lines l
			            JOIN transactions t ON t.id = l.transaction_id
			           WHERE l.account_id = a.id AND l.side = 'DEBIT' AND t.date < ?), 0),
			COALESCE((SELECT SUM(l.amount_kop)
			            FROM transaction_lines l
			            JOIN transactions t ON t.id = l.transaction_id
			           WHERE l.account_id = a.id AND l.side = 'CREDIT' AND t.date < ?), 0)
		  FROM accounts a
		 WHERE a.id = ?`, toUnix(date), toUnix(date), accountID).Scan(&accountType, &debit, &credit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: счёт %d", ErrNotFound, accountID)
	}
	if err != nil {
		return 0, fmt.Errorf("store: не удалось посчитать остаток на дату: %w", err)
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

// BalanceFor переводит дебетовый и кредитовый обороты в остаток по природе типа.
//
// Формула: актив и расход — ΣDEBIT − ΣCREDIT, пассив, капитал и доход —
// ΣCREDIT − ΣDEBIT.
func BalanceFor(t model.AccountType, debit, credit money.Kop) money.Kop {
	switch t {
	case model.TypeAsset, model.TypeExpense:
		return debit - credit
	default: // пассив, капитал, доход
		return credit - debit
	}
}

// Turnover — обороты и остатки счёта за период.
type Turnover struct {
	// AccountID — счёт.
	AccountID int64
	// OpeningDebit и OpeningCredit — обороты до начала периода.
	OpeningDebit  money.Kop
	OpeningCredit money.Kop
	// PeriodDebit и PeriodCredit — обороты внутри периода.
	PeriodDebit  money.Kop
	PeriodCredit money.Kop
}

// Turnovers возвращает обороты всех счетов за период [from, to] включительно.
//
// Нуль в from или to означает отсутствие границы. Остатки на начало и конец
// периода вычисляются вызывающим кодом через BalanceFor: хранимых остатков в
// книге нет.
func (db *DB) Turnovers(ctx context.Context, from, to int64) (map[int64]Turnover, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id,
			COALESCE(SUM(CASE WHEN t.date < ? THEN CASE WHEN l.side = 'DEBIT'  THEN l.amount_kop ELSE 0 END ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN t.date < ? THEN CASE WHEN l.side = 'CREDIT' THEN l.amount_kop ELSE 0 END ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (? = 0 OR t.date >= ?) AND (? = 0 OR t.date <= ?)
				THEN CASE WHEN l.side = 'DEBIT'  THEN l.amount_kop ELSE 0 END ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN (? = 0 OR t.date >= ?) AND (? = 0 OR t.date <= ?)
				THEN CASE WHEN l.side = 'CREDIT' THEN l.amount_kop ELSE 0 END ELSE 0 END), 0)
		  FROM accounts a
		  LEFT JOIN transaction_lines l ON l.account_id = a.id
		  LEFT JOIN transactions t ON t.id = l.transaction_id
		 GROUP BY a.id`,
		from, from, from, from, to, to, from, from, to, to)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось посчитать обороты: %w", err)
	}
	defer rows.Close()

	turnovers := map[int64]Turnover{}
	for rows.Next() {
		var (
			id                          int64
			openingDebit, openingCredit int64
			periodDebit, periodCredit   int64
		)
		if err := rows.Scan(&id, &openingDebit, &openingCredit, &periodDebit, &periodCredit); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать обороты: %w", err)
		}
		turnovers[id] = Turnover{
			AccountID:     id,
			OpeningDebit:  money.FromKopecks(openingDebit),
			OpeningCredit: money.FromKopecks(openingCredit),
			PeriodDebit:   money.FromKopecks(periodDebit),
			PeriodCredit:  money.FromKopecks(periodCredit),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения оборотов: %w", err)
	}
	return turnovers, nil
}

// balanceFor переводит дебетовый и кредитовый обороты в остаток по природе типа.
func balanceFor(t model.AccountType, debit, credit money.Kop) money.Kop {
	return BalanceFor(t, debit, credit)
}
