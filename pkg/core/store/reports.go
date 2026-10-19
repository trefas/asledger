package store

import (
	"context"
	"fmt"

	"github.com/trefas/asledger/pkg/core/money"
)

// AccountActorAmount — агрегированная сумма по счёту и автору проводок.
type AccountActorAmount struct {
	// ActorID — автор проводок.
	ActorID int64
	// AccountID — счёт строк.
	AccountID int64
	// Amount — сумма (для расходов: дебет минус кредит).
	Amount money.Kop
}

// ExpensesByActor возвращает расходы по счетам типа «расход» в разрезе авторов.
//
// Расход увеличивается дебетом и уменьшается кредитом (сторно, корректировка),
// поэтому сумма считается как ΣDEBIT − ΣCREDIT. Нуль в from или to означает
// отсутствие соответствующей границы периода.
func (db *DB) ExpensesByActor(ctx context.Context, from, to int64) ([]AccountActorAmount, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.actor_id, l.account_id,
			SUM(CASE WHEN l.side = 'DEBIT' THEN l.amount_kop ELSE -l.amount_kop END) AS amount
		  FROM transaction_lines l
		  JOIN transactions t ON t.id = l.transaction_id
		  JOIN accounts a ON a.id = l.account_id
		 WHERE a.type = 'расход'
		   AND (? = 0 OR t.date >= ?)
		   AND (? = 0 OR t.date <= ?)
		 GROUP BY t.actor_id, l.account_id
		 ORDER BY t.actor_id, l.account_id`,
		from, from, to, to)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось посчитать расходы по авторам: %w", err)
	}
	defer rows.Close()

	var out []AccountActorAmount
	for rows.Next() {
		var row AccountActorAmount
		var amount int64
		if err := rows.Scan(&row.ActorID, &row.AccountID, &amount); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать расходы по авторам: %w", err)
		}
		row.Amount = money.FromKopecks(amount)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения расходов по авторам: %w", err)
	}
	return out, nil
}
