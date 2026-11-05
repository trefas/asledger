package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// maxRatePPM — 100 % годовых в ppm (1 ppm = 0.0001 %).
const maxRatePPM int64 = 1_000_000

// CreditSettingsInput — входные данные настроек кредита.
type CreditSettingsInput struct {
	RatePPM        int64
	InterestType   model.CreditInterestType
	RepaymentOrder model.RepaymentOrder
	GraceEnabled   bool
	GraceEndDate   time.Time
}

// DepositStepRateInput — ступенчатая ставка вклада.
//
// To == nil означает «до бесконечности».
type DepositStepRateInput struct {
	From    money.Kop
	To      *money.Kop
	RatePPM int64
}

// DepositSettingsInput — входные данные настроек вклада.
type DepositSettingsInput struct {
	BaseRatePPM      int64
	MinBalance       money.Kop
	BonusType        model.DepositBonusType
	BonusRatePPM     int64
	BonusAmount      money.Kop
	AccrualFrequency model.DepositAccrualFrequency
	Capitalization   bool
	OpenedAt         time.Time
	ClosedAt         time.Time
	Steps            []DepositStepRateInput
}

const creditSettingsColumns = `account_id, rate_ppm, interest_type, repayment_order,
	grace_enabled, grace_end_date, created_at, updated_at`

// CreditSettings возвращает настройки кредитного счёта.
func (db *DB) CreditSettings(ctx context.Context, accountID int64) (*model.CreditSettings, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+creditSettingsColumns+` FROM credit_settings WHERE account_id = ?`, accountID)
	settings, err := scanCreditSettings(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: настройки кредита для счёта %d", ErrNotFound, accountID)
	}
	return settings, err
}

// SetCreditSettings создаёт или заменяет настройки кредитного счёта.
func (db *DB) SetCreditSettings(ctx context.Context, accountID int64, in CreditSettingsInput) (*model.CreditSettings, error) {
	if in.InterestType == "" {
		in.InterestType = model.CreditInterestSimple
	}
	if in.RepaymentOrder == "" {
		in.RepaymentOrder = model.RepaymentInterestFirst
	}
	if err := validateCreditSettings(in); err != nil {
		return nil, err
	}

	now := Now()
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		if err := validateCreditAccountTx(ctx, tx, accountID); err != nil {
			return err
		}
		exists, err := settingsExistsTx(ctx, tx, "credit_settings", accountID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO credit_settings(account_id, rate_ppm, interest_type, repayment_order,
			                             grace_enabled, grace_end_date, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id) DO UPDATE SET
			     rate_ppm = excluded.rate_ppm,
			     interest_type = excluded.interest_type,
			     repayment_order = excluded.repayment_order,
			     grace_enabled = excluded.grace_enabled,
			     grace_end_date = excluded.grace_end_date,
			     updated_at = excluded.updated_at`,
			accountID, in.RatePPM, string(in.InterestType), string(in.RepaymentOrder),
			boolInt(in.GraceEnabled), toUnix(in.GraceEndDate), now, now); err != nil {
			return fmt.Errorf("store: не удалось сохранить настройки кредита: %w", err)
		}
		op := model.OpInsert
		if exists {
			op = model.OpUpdate
		}
		return appendChangelog(ctx, tx, model.EntityCreditSettings, accountID, op)
	})
	if err != nil {
		return nil, err
	}
	return db.CreditSettings(ctx, accountID)
}

// DeleteCreditSettings удаляет настройки кредита, не трогая счёт.
func (db *DB) DeleteCreditSettings(ctx context.Context, accountID int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM credit_settings WHERE account_id = ?`, accountID)
		if err != nil {
			return fmt.Errorf("store: не удалось удалить настройки кредита: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: не удалось удалить настройки кредита: %w", err)
		}
		if affected == 0 {
			return fmt.Errorf("%w: настройки кредита для счёта %d", ErrNotFound, accountID)
		}
		return appendChangelog(ctx, tx, model.EntityCreditSettings, accountID, model.OpDelete)
	})
}

const depositSettingsColumns = `account_id, base_rate_ppm, min_balance_kop, bonus_type,
	bonus_rate_ppm, bonus_amount_kop, accrual_frequency, capitalization,
	opened_at, closed_at, created_at, updated_at`

// DepositSettings возвращает настройки вклада вместе со ступенями.
func (db *DB) DepositSettings(ctx context.Context, accountID int64) (*model.DepositSettings, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+depositSettingsColumns+` FROM deposit_settings WHERE account_id = ?`, accountID)
	settings, err := scanDepositSettings(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: настройки вклада для счёта %d", ErrNotFound, accountID)
	}
	if err != nil {
		return nil, err
	}
	steps, err := db.depositStepRates(ctx, accountID)
	if err != nil {
		return nil, err
	}
	settings.Steps = steps
	return settings, nil
}

// SetDepositSettings создаёт или заменяет настройки вклада и его ступени.
func (db *DB) SetDepositSettings(ctx context.Context, accountID int64, in DepositSettingsInput) (*model.DepositSettings, error) {
	if in.BonusType == "" {
		in.BonusType = model.DepositBonusNone
	}
	if in.AccrualFrequency == "" {
		in.AccrualFrequency = model.DepositAccrualMonthly
	}
	steps, err := normalizeDepositSteps(in.Steps)
	if err != nil {
		return nil, err
	}
	if err := validateDepositSettings(in); err != nil {
		return nil, err
	}

	now := Now()
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		if err := validateDepositAccountTx(ctx, tx, accountID); err != nil {
			return err
		}
		exists, err := settingsExistsTx(ctx, tx, "deposit_settings", accountID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO deposit_settings(account_id, base_rate_ppm, min_balance_kop,
			                              bonus_type, bonus_rate_ppm, bonus_amount_kop,
			                              accrual_frequency, capitalization,
			                              opened_at, closed_at, created_at, updated_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(account_id) DO UPDATE SET
			     base_rate_ppm = excluded.base_rate_ppm,
			     min_balance_kop = excluded.min_balance_kop,
			     bonus_type = excluded.bonus_type,
			     bonus_rate_ppm = excluded.bonus_rate_ppm,
			     bonus_amount_kop = excluded.bonus_amount_kop,
			     accrual_frequency = excluded.accrual_frequency,
			     capitalization = excluded.capitalization,
			     opened_at = excluded.opened_at,
			     closed_at = excluded.closed_at,
			     updated_at = excluded.updated_at`,
			accountID, in.BaseRatePPM, int64(in.MinBalance),
			string(in.BonusType), in.BonusRatePPM, int64(in.BonusAmount),
			string(in.AccrualFrequency), boolInt(in.Capitalization),
			toUnix(in.OpenedAt), toUnix(in.ClosedAt), now, now); err != nil {
			return fmt.Errorf("store: не удалось сохранить настройки вклада: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM deposit_step_rates WHERE account_id = ?`, accountID); err != nil {
			return fmt.Errorf("store: не удалось заменить ступени вклада: %w", err)
		}
		for _, step := range steps {
			var to any
			if step.To != nil {
				to = int64(*step.To)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
				 VALUES(?, ?, ?, ?)`,
				accountID, int64(step.From), to, step.RatePPM); err != nil {
				return fmt.Errorf("store: не удалось сохранить ступень вклада: %w", err)
			}
		}
		op := model.OpInsert
		if exists {
			op = model.OpUpdate
		}
		return appendChangelog(ctx, tx, model.EntityDepositSettings, accountID, op)
	})
	if err != nil {
		return nil, err
	}
	return db.DepositSettings(ctx, accountID)
}

// DeleteDepositSettings удаляет настройки вклада и ступени, не трогая счёт.
func (db *DB) DeleteDepositSettings(ctx context.Context, accountID int64) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`DELETE FROM deposit_settings WHERE account_id = ?`, accountID)
		if err != nil {
			return fmt.Errorf("store: не удалось удалить настройки вклада: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: не удалось удалить настройки вклада: %w", err)
		}
		if affected == 0 {
			return fmt.Errorf("%w: настройки вклада для счёта %d", ErrNotFound, accountID)
		}
		return appendChangelog(ctx, tx, model.EntityDepositSettings, accountID, model.OpDelete)
	})
}

// AccruedInterest возвращает сумму начисленных, но ещё не погашенных процентов
// по счёту указанного вида.
//
// Учитываются только начисления со статусом «accrued»: выплаченные и
// капитализированные проценты в остаток не входят.
func (db *DB) AccruedInterest(ctx context.Context, accountID int64, kind model.InterestAccrualKind) (money.Kop, error) {
	if !kind.Valid() {
		return 0, fmt.Errorf("store: недопустимый вид начисления %q", kind)
	}
	if _, err := db.Account(ctx, accountID); err != nil {
		return 0, err
	}
	var total int64
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_kop), 0)
		  FROM interest_accruals
		 WHERE account_id = ? AND kind = ? AND status = ?`,
		accountID, string(kind), string(model.InterestAccrued)).Scan(&total); err != nil {
		return 0, fmt.Errorf("store: не удалось прочитать начисленные проценты: %w", err)
	}
	return money.FromKopecks(total), nil
}

// depositStepRates читает ступени вклада по возрастанию нижней границы.
func (db *DB) depositStepRates(ctx context.Context, accountID int64) ([]model.DepositStepRate, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT account_id, from_kop, to_kop, rate_ppm
		   FROM deposit_step_rates
		  WHERE account_id = ?
		  ORDER BY from_kop`, accountID)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать ступени вклада: %w", err)
	}
	defer rows.Close()

	var steps []model.DepositStepRate
	for rows.Next() {
		var (
			step model.DepositStepRate
			to   sql.NullInt64
		)
		if err := rows.Scan(&step.AccountID, &step.From, &to, &step.RatePPM); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать ступень вклада: %w", err)
		}
		if to.Valid {
			value := money.FromKopecks(to.Int64)
			step.To = &value
		}
		steps = append(steps, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения ступеней вклада: %w", err)
	}
	return steps, nil
}

// scanCreditSettings разбирает строку настроек кредита.
func scanCreditSettings(s rowScanner) (*model.CreditSettings, error) {
	var (
		settings     model.CreditSettings
		interestType string
		repayment    string
		graceEnabled int
		graceEnd     int64
		created      int64
		updated      int64
	)
	if err := s.Scan(&settings.AccountID, &settings.RatePPM, &interestType, &repayment,
		&graceEnabled, &graceEnd, &created, &updated); err != nil {
		return nil, err
	}
	settings.InterestType = model.CreditInterestType(interestType)
	settings.RepaymentOrder = model.RepaymentOrder(repayment)
	settings.GraceEnabled = graceEnabled != 0
	settings.GraceEndDate = fromUnixOptional(graceEnd)
	settings.CreatedAt = fromUnix(created)
	settings.UpdatedAt = fromUnix(updated)
	return &settings, nil
}

// scanDepositSettings разбирает строку настроек вклада.
func scanDepositSettings(s rowScanner) (*model.DepositSettings, error) {
	var (
		settings    model.DepositSettings
		bonusType   string
		frequency   string
		capitalized int
		opened      int64
		closed      int64
		created     int64
		updated     int64
	)
	if err := s.Scan(&settings.AccountID, &settings.BaseRatePPM, &settings.MinBalance,
		&bonusType, &settings.BonusRatePPM, &settings.BonusAmount,
		&frequency, &capitalized, &opened, &closed, &created, &updated); err != nil {
		return nil, err
	}
	settings.BonusType = model.DepositBonusType(bonusType)
	settings.AccrualFrequency = model.DepositAccrualFrequency(frequency)
	settings.Capitalization = capitalized != 0
	settings.OpenedAt = fromUnixOptional(opened)
	settings.ClosedAt = fromUnixOptional(closed)
	settings.CreatedAt = fromUnix(created)
	settings.UpdatedAt = fromUnix(updated)
	return &settings, nil
}

// validateCreditSettings проверяет поля настроек кредита.
func validateCreditSettings(in CreditSettingsInput) error {
	if in.RatePPM < 0 || in.RatePPM > maxRatePPM {
		return creditValidation(CodeCreditRate,
			fmt.Sprintf("ставка кредита должна быть в диапазоне 0…%d ppm", maxRatePPM))
	}
	if !in.InterestType.Valid() {
		return creditValidation(CodeCreditInterestType,
			"недопустимый способ расчёта процентов «"+string(in.InterestType)+"»")
	}
	if !in.RepaymentOrder.Valid() {
		return creditValidation(CodeCreditRepaymentOrder,
			"недопустимый порядок погашения «"+string(in.RepaymentOrder)+"»")
	}
	if in.GraceEnabled && in.GraceEndDate.IsZero() {
		return creditValidation(CodeCreditGraceDate,
			"для беспроцентного периода укажите дату окончания")
	}
	return nil
}

// validateDepositSettings проверяет поля настроек вклада.
func validateDepositSettings(in DepositSettingsInput) error {
	if in.BaseRatePPM < 0 || in.BaseRatePPM > maxRatePPM {
		return depositValidation(CodeDepositRate,
			fmt.Sprintf("ставка вклада должна быть в диапазоне 0…%d ppm", maxRatePPM))
	}
	if in.MinBalance < 0 {
		return depositValidation(CodeDepositMinBalance, "минимальный остаток не может быть отрицательным")
	}
	if !in.BonusType.Valid() {
		return depositValidation(CodeDepositBonus,
			"недопустимый вид бонуса «"+string(in.BonusType)+"»")
	}
	if in.BonusRatePPM < 0 || in.BonusRatePPM > maxRatePPM {
		return depositValidation(CodeDepositBonus,
			fmt.Sprintf("бонусная ставка должна быть в диапазоне 0…%d ppm", maxRatePPM))
	}
	if in.BonusAmount < 0 {
		return depositValidation(CodeDepositBonus, "сумма бонуса не может быть отрицательной")
	}
	if in.BonusType == model.DepositBonusFixed && in.BonusAmount == 0 {
		return depositValidation(CodeDepositBonus, "для фиксированного бонуса укажите сумму")
	}
	if in.BonusType == model.DepositBonusRate && in.BonusRatePPM == 0 {
		return depositValidation(CodeDepositBonus, "для процентного бонуса укажите ставку")
	}
	if !in.AccrualFrequency.Valid() {
		return depositValidation(CodeDepositFrequency,
			"недопустимая частота начисления «"+string(in.AccrualFrequency)+"»")
	}
	if !in.OpenedAt.IsZero() && !in.ClosedAt.IsZero() && in.ClosedAt.Before(in.OpenedAt) {
		return depositValidation(CodeDepositDates, "дата закрытия раньше даты открытия")
	}
	return nil
}

// normalizeDepositSteps сортирует ступени и проверяет их корректность.
func normalizeDepositSteps(steps []DepositStepRateInput) ([]DepositStepRateInput, error) {
	normalized := append([]DepositStepRateInput(nil), steps...)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].From < normalized[j].From
	})
	for i, step := range normalized {
		if step.From < 0 {
			return nil, depositValidation(CodeDepositSteps, "нижняя граница ступени не может быть отрицательной")
		}
		if step.RatePPM < 0 || step.RatePPM > maxRatePPM {
			return nil, depositValidation(CodeDepositSteps,
				fmt.Sprintf("ставка ступени должна быть в диапазоне 0…%d ppm", maxRatePPM))
		}
		if step.To != nil && *step.To <= step.From {
			return nil, depositValidation(CodeDepositSteps, "верхняя граница ступени должна быть больше нижней")
		}
		if i == 0 {
			continue
		}
		previous := normalized[i-1]
		if previous.To == nil || step.From < *previous.To {
			return nil, depositValidation(CodeDepositSteps, "диапазоны ступеней пересекаются")
		}
	}
	return normalized, nil
}

// validateCreditAccountTx проверяет, что счёт — листовой пассив.
func validateCreditAccountTx(ctx context.Context, tx *sql.Tx, accountID int64) error {
	acc, err := getAccountTx(ctx, tx, accountID)
	if err != nil {
		return creditValidation(CodeCreditAccountInvalid, "счёт кредита не найден")
	}
	if acc.IsArchived || !acc.IsLeaf || acc.Type != model.TypeLiability {
		return creditValidation(CodeCreditAccountInvalid,
			"кредит настраивается только для листового счёта типа «пассив»: "+acc.FullPath)
	}
	return nil
}

// validateDepositAccountTx проверяет, что счёт — листовой актив.
func validateDepositAccountTx(ctx context.Context, tx *sql.Tx, accountID int64) error {
	acc, err := getAccountTx(ctx, tx, accountID)
	if err != nil {
		return depositValidation(CodeDepositAccountInvalid, "счёт вклада не найден")
	}
	if acc.IsArchived || !acc.IsLeaf || acc.Type != model.TypeAsset {
		return depositValidation(CodeDepositAccountInvalid,
			"вклад настраивается только для листового счёта типа «актив»: "+acc.FullPath)
	}
	return nil
}

// settingsExistsTx сообщает, есть ли настройки счёта в указанной таблице.
func settingsExistsTx(ctx context.Context, tx *sql.Tx, table string, accountID int64) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM `+table+` WHERE account_id = ?`, accountID).Scan(&count); err != nil {
		return false, fmt.Errorf("store: не удалось проверить настройки счёта: %w", err)
	}
	return count > 0, nil
}

// creditValidation создаёт ошибку валидации настроек кредита.
func creditValidation(code, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}

// depositValidation создаёт ошибку валидации настроек вклада.
func depositValidation(code, message string) *ValidationError {
	return &ValidationError{Code: code, Message: message}
}
