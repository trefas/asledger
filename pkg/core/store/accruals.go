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

// interestAccrualColumns — единый список колонок начисления.
const interestAccrualColumns = `id, uuid, account_id, date, kind, amount_kop,
	base_amount_kop, rate_ppm, period_from, period_to, status, transaction_id,
	created_at, updated_at`

// InterestAccrualInput — данные для создания начисления.
type InterestAccrualInput struct {
	// UUID — идентификатор начисления; повтор с тем же uuid идемпотентен.
	UUID string
	// AccountID — счёт кредита или вклада.
	AccountID int64
	// Date — дата начисления.
	Date time.Time
	// Kind — вид начисления.
	Kind model.InterestAccrualKind
	// Amount — сумма начисления; должна быть положительной.
	Amount money.Kop
	// BaseAmount и RatePPM — база и ставка расчёта.
	BaseAmount money.Kop
	RatePPM    int64
	// PeriodFrom и PeriodTo — период начисления; нули означают ручное.
	PeriodFrom time.Time
	PeriodTo   time.Time
	// Status — статус начисления; пусто означает «accrued».
	Status model.InterestAccrualStatus
	// TransactionID — проводка, которой проведено начисление.
	TransactionID *int64
}

// EnsureInterestAccrual создаёт начисление или возвращает уже существующее.
//
// Идемпотентность двойная: по uuid и по периоду (уникальный индекс книги).
// Для ручных начислений с нулевым периодом период не проверяется.
func (db *DB) EnsureInterestAccrual(ctx context.Context, in InterestAccrualInput) (*model.InterestAccrual, bool, error) {
	if err := validateInterestAccrualInput(in); err != nil {
		return nil, false, err
	}
	if in.Status == "" {
		in.Status = model.InterestAccrued
	}

	if existing, err := db.InterestAccrualByUUID(ctx, in.UUID); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	if !in.PeriodFrom.IsZero() || !in.PeriodTo.IsZero() {
		existing, err := db.interestAccrualByPeriod(ctx, in.AccountID, in.Kind, in.PeriodFrom, in.PeriodTo)
		if err == nil {
			return existing, false, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, false, err
		}
	}

	var created *model.InterestAccrual
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		id, err := insertInterestAccrualTx(ctx, tx, in)
		if err != nil {
			return err
		}
		created, err = interestAccrualTx(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// InterestAccrualByUUID возвращает начисление по uuid.
func (db *DB) InterestAccrualByUUID(ctx context.Context, uuid string) (*model.InterestAccrual, error) {
	accrual, err := scanInterestAccrual(db.QueryRowContext(ctx,
		`SELECT `+interestAccrualColumns+` FROM interest_accruals WHERE uuid = ?`, uuid))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: начисление %s", ErrNotFound, uuid)
	}
	if err != nil {
		return nil, err
	}
	return accrual, nil
}

// InterestAccruals возвращает начисления по счёту, виду и статусу.
//
// Пустые значения фильтров означают «любой»; accountID == 0 — все счета.
func (db *DB) InterestAccruals(ctx context.Context, accountID int64,
	kind model.InterestAccrualKind, status model.InterestAccrualStatus) ([]model.InterestAccrual, error) {
	query := `SELECT ` + interestAccrualColumns + ` FROM interest_accruals WHERE 1 = 1`
	var args []any
	if accountID != 0 {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, string(kind))
	}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, string(status))
	}
	query += ` ORDER BY date, id`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать начисления: %w", err)
	}
	defer rows.Close()

	var accruals []model.InterestAccrual
	for rows.Next() {
		accrual, err := scanInterestAccrual(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать начисление: %w", err)
		}
		accruals = append(accruals, *accrual)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения начислений: %w", err)
	}
	return accruals, nil
}

// PayInterestAccruals отмечает начисления выплаченными на сумму amount.
//
// Начисления выбираются по возрастанию даты. Частичная выплата одного
// начисления не поддерживается: если остатка не хватает на целую запись,
// она остаётся в статусе «accrued». Нулевая дата asOf не ограничивает
// выборку; иначе учитываются начисления не позже указанной даты.
// Возвращается фактически закрытая сумма.
func (db *DB) PayInterestAccruals(ctx context.Context, accountID int64,
	kinds []model.InterestAccrualKind, amount money.Kop, transactionID int64,
	asOf time.Time) (money.Kop, error) {
	if amount <= 0 {
		return 0, nil
	}
	var paid money.Kop
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		query := `SELECT id, amount_kop FROM interest_accruals
		           WHERE account_id = ? AND status = ?`
		args := []any{accountID, string(model.InterestAccrued)}
		if len(kinds) > 0 {
			query += ` AND kind IN (` + placeholders(len(kinds)) + `)`
			for _, kind := range kinds {
				args = append(args, string(kind))
			}
		}
		if !asOf.IsZero() {
			query += ` AND date <= ?`
			args = append(args, toUnix(asOf))
		}
		query += ` ORDER BY date, id`

		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("store: не удалось прочитать начисления к выплате: %w", err)
		}
		defer rows.Close()

		remaining := amount
		for rows.Next() {
			var (
				id  int64
				kop int64
			)
			if err := rows.Scan(&id, &kop); err != nil {
				return fmt.Errorf("store: не удалось разобрать начисление: %w", err)
			}
			value := money.FromKopecks(kop)
			if value > remaining {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE interest_accruals
				   SET status = ?, transaction_id = ?, updated_at = ?
				 WHERE id = ?`,
				string(model.InterestPaid), transactionID, Now(), id); err != nil {
				return fmt.Errorf("store: не удалось отметить начисление выплаченным: %w", err)
			}
			remaining -= value
			paid += value
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("store: ошибка чтения начислений: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return paid, nil
}

// PostInterestAccrual сохраняет начисление и его проводку одной транзакцией.
//
// Если проводка не нужна (начисление только в субледжере), используйте
// EnsureInterestAccrual. После записи начисление получает transaction_id.
func (db *DB) PostInterestAccrual(ctx context.Context, in InterestAccrualInput,
	tr *model.Transaction) (*model.InterestAccrual, error) {
	if err := validateInterestAccrualInput(in); err != nil {
		return nil, err
	}
	if err := validateTransaction(tr); err != nil {
		return nil, err
	}
	if in.Status == "" {
		in.Status = model.InterestAccrued
	}

	var created *model.InterestAccrual
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		trID, err := insertTransactionTx(ctx, tx, tr)
		if err != nil {
			return err
		}
		in.TransactionID = &trID
		id, err := insertInterestAccrualTx(ctx, tx, in)
		if err != nil {
			return err
		}
		created, err = interestAccrualTx(ctx, tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// InterestAccrualByPeriod возвращает начисление за период или ErrNotFound.
//
// Период сравнивается по датам без времени: так повторный расчёт за тот же
// календарный период находит уже сохранённое начисление.
func (db *DB) InterestAccrualByPeriod(ctx context.Context, accountID int64,
	kind model.InterestAccrualKind, from, to time.Time) (*model.InterestAccrual, error) {
	return db.interestAccrualByPeriod(ctx, accountID, kind, from, to)
}

// interestAccrualByPeriod ищет начисление за период.
func (db *DB) interestAccrualByPeriod(ctx context.Context, accountID int64,
	kind model.InterestAccrualKind, from, to time.Time) (*model.InterestAccrual, error) {
	accrual, err := scanInterestAccrual(db.QueryRowContext(ctx,
		`SELECT `+interestAccrualColumns+` FROM interest_accruals
		  WHERE account_id = ? AND kind = ? AND period_from = ? AND period_to = ?`,
		accountID, string(kind), unixOrZero(from), unixOrZero(to)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: начисление за период", ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return accrual, nil
}

// interestAccrualTx читает начисление внутри транзакции БД.
func interestAccrualTx(ctx context.Context, tx *sql.Tx, id int64) (*model.InterestAccrual, error) {
	accrual, err := scanInterestAccrual(tx.QueryRowContext(ctx,
		`SELECT `+interestAccrualColumns+` FROM interest_accruals WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать начисление: %w", err)
	}
	return accrual, nil
}

// insertInterestAccrualTx вставляет начисление в готовую транзакцию БД.
func insertInterestAccrualTx(ctx context.Context, tx *sql.Tx, in InterestAccrualInput) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
		                              base_amount_kop, rate_ppm, period_from, period_to,
		                              status, transaction_id, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.UUID, in.AccountID, toUnix(in.Date), string(in.Kind), int64(in.Amount),
		int64(in.BaseAmount), in.RatePPM, unixOrZero(in.PeriodFrom), unixOrZero(in.PeriodTo),
		string(in.Status), nullableID(in.TransactionID), Now(), Now())
	if err != nil {
		return 0, fmt.Errorf("store: не удалось создать начисление: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: не удалось получить id начисления: %w", err)
	}
	if err := appendChangelog(ctx, tx, model.EntityInterestAccrual, id, model.OpInsert); err != nil {
		return 0, err
	}
	return id, nil
}

// scanInterestAccrual разбирает строку начисления.
func scanInterestAccrual(row rowScanner) (*model.InterestAccrual, error) {
	var (
		accrual       model.InterestAccrual
		date          int64
		kind, status  string
		periodFrom    int64
		periodTo      int64
		transactionID sql.NullInt64
		createdAt     int64
		updatedAt     int64
	)
	if err := row.Scan(&accrual.ID, &accrual.UUID, &accrual.AccountID, &date, &kind,
		&accrual.Amount, &accrual.BaseAmount, &accrual.RatePPM, &periodFrom, &periodTo,
		&status, &transactionID, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	accrual.Date = fromUnix(date)
	accrual.Kind = model.InterestAccrualKind(kind)
	accrual.Status = model.InterestAccrualStatus(status)
	if periodFrom > 0 {
		accrual.PeriodFrom = fromUnix(periodFrom)
	}
	if periodTo > 0 {
		accrual.PeriodTo = fromUnix(periodTo)
	}
	if transactionID.Valid {
		id := transactionID.Int64
		accrual.TransactionID = &id
	}
	accrual.CreatedAt = fromUnix(createdAt)
	accrual.UpdatedAt = fromUnix(updatedAt)
	return &accrual, nil
}

// validateInterestAccrualInput проверяет данные начисления.
func validateInterestAccrualInput(in InterestAccrualInput) error {
	if in.UUID == "" {
		return &ValidationError{Code: CodeInterestUUID, Message: "не задан uuid начисления"}
	}
	if in.AccountID <= 0 {
		return &ValidationError{Code: CodeInterestAccount, Message: "не задан счёт начисления"}
	}
	if in.Date.IsZero() {
		return &ValidationError{Code: CodeInterestDate, Message: "не задана дата начисления"}
	}
	if !in.Kind.Valid() {
		return &ValidationError{Code: CodeInterestKind, Message: "недопустимый вид начисления"}
	}
	if in.Amount <= 0 {
		return &ValidationError{Code: CodeInterestAmount, Message: "сумма начисления должна быть положительной"}
	}
	if in.BaseAmount < 0 {
		return &ValidationError{Code: CodeInterestAmount, Message: "база начисления не может быть отрицательной"}
	}
	if in.RatePPM < 0 || in.RatePPM > maxRatePPM {
		return &ValidationError{Code: CodeInterestRate, Message: "ставка начисления вне диапазона 0…100 %"}
	}
	if in.Status == "" {
		in.Status = model.InterestAccrued
	}
	if !in.Status.Valid() {
		return &ValidationError{Code: CodeInterestStatus, Message: "недопустимый статус начисления"}
	}
	return nil
}

// unixOrZero переводит время в unix-секунды; нулевое время — 0.
func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// placeholders возвращает список «?, ?, …» для IN-выражения.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := "?"
	for i := 1; i < n; i++ {
		out += ", ?"
	}
	return out
}
