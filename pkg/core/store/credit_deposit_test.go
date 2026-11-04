package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// TestASL_16_CreditDepositMigration проверяет таблицы, индексы и триггеры
// миграции кредитов и вкладов на пустой книге.
func TestASL_16_CreditDepositMigration(t *testing.T) {
	db := openTestDB(t)

	for _, table := range []string{
		"credit_settings", "deposit_settings",
		"deposit_step_rates", "interest_accruals",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("таблица %q не создана", table)
		}
	}

	for _, index := range []string{
		"idx_interest_accruals_account_date",
		"idx_interest_accruals_status",
		"idx_interest_accruals_transaction",
		"idx_interest_accruals_period",
	} {
		var name string
		if err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&name); err != nil {
			t.Fatalf("индекс %q не найден: %v", index, err)
		}
	}

	for _, trigger := range []string{
		"credit_settings_account_insert", "credit_settings_account_update",
		"deposit_settings_account_insert", "deposit_settings_account_update",
		"interest_accruals_account_insert", "interest_accruals_account_update",
	} {
		var name string
		if err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'trigger' AND name = ?`, trigger).Scan(&name); err != nil {
			t.Fatalf("триггер %q не найден: %v", trigger, err)
		}
	}

	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("версия схемы: %v", err)
	}
	if version != 8 {
		t.Fatalf("версия схемы = %d, ожидалась 8", version)
	}
}

// TestASL_16_CreditDepositMigrationFromV133 проверяет обновление книги v1.3.3:
// схема 0001–0005 уже есть, 0006–0008 применяются без потери данных.
func TestASL_16_CreditDepositMigrationFromV133(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "v133.db"), WithKey(testKey))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migrations {
		if m.version > 5 {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			t.Fatalf("миграция %s: %v", m.name, err)
		}
	}

	userID := mustCreateUser(t, db, "Андрей")
	credit := mustCreateAccount(t, db, "Кредит", model.TypeLiability, nil)
	deposit := mustCreateAccount(t, db, "Вклад", model.TypeAsset, nil)

	if tableExists(t, db, "credit_settings") {
		t.Fatal("таблица credit_settings уже существует до миграции 0007")
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var version int
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("версия схемы после миграции: %v", err)
	}
	if version != 8 {
		t.Fatalf("версия схемы = %d, ожидалась 8", version)
	}

	var userName string
	if err := db.QueryRowContext(ctx, `SELECT name FROM users WHERE id = ?`, userID).Scan(&userName); err != nil {
		t.Fatalf("пользователь потерян: %v", err)
	}
	if userName != "Андрей" {
		t.Fatalf("имя пользователя = %q", userName)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO credit_settings(account_id, rate_ppm, created_at, updated_at)
		 VALUES(?, 150000, 1, 1)`, credit.ID); err != nil {
		t.Fatalf("credit_settings после миграции: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_settings(account_id, base_rate_ppm, created_at, updated_at)
		 VALUES(?, 180000, 1, 1)`, deposit.ID); err != nil {
		t.Fatalf("deposit_settings после миграции: %v", err)
	}
}

// TestASL_16_CreditSettingsConstraints проверяет ограничения кредитных
// настроек: ставку, перечисления и допустимость счёта.
func TestASL_16_CreditSettingsConstraints(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	parent := mustCreateAccount(t, db, "Кредиты", model.TypeLiability, nil)
	credit := mustCreateAccount(t, db, "Карта", model.TypeLiability, &parent.ID)
	asset := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)

	insert := func(accountID int64) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO credit_settings(account_id, rate_ppm, interest_type, repayment_order,
			                             grace_enabled, grace_end_date, created_at, updated_at)
			 VALUES(?, 150000, 'simple', 'interest_first', 1, 1800000000, 1, 1)`, accountID)
		return err
	}

	if err := insert(credit.ID); err != nil {
		t.Fatalf("настройки листового пассива: %v", err)
	}
	if err := insert(credit.ID); err == nil {
		t.Fatal("ожидалась ошибка повторной настройки счёта")
	}
	if err := insert(parent.ID); err == nil || !strings.Contains(err.Error(), "листовым пассивом") {
		t.Fatalf("настройки нелистового счёта: %v", err)
	}
	if err := insert(asset.ID); err == nil || !strings.Contains(err.Error(), "листовым пассивом") {
		t.Fatalf("настройки актива: %v", err)
	}

	for _, tc := range []struct {
		name  string
		query string
		arg   any
	}{
		{"ставка ниже нуля", `UPDATE credit_settings SET rate_ppm = ? WHERE account_id = ?`, -1},
		{"ставка выше 100%", `UPDATE credit_settings SET rate_ppm = ? WHERE account_id = ?`, 1000001},
		{"неизвестный тип процентов", `UPDATE credit_settings SET interest_type = ? WHERE account_id = ?`, "annuity"},
		{"неизвестный порядок", `UPDATE credit_settings SET repayment_order = ? WHERE account_id = ?`, "body_first"},
		{"grace без даты", `UPDATE credit_settings SET grace_end_date = ? WHERE account_id = ?`, 0},
	} {
		if _, err := db.ExecContext(ctx, tc.query, tc.arg, credit.ID); err == nil {
			t.Fatalf("%s: ожидалась ошибка", tc.name)
		}
	}

	// Перенос настроек на счёт другого типа запрещён.
	if _, err := db.ExecContext(ctx,
		`UPDATE credit_settings SET account_id = ? WHERE account_id = ?`, asset.ID, credit.ID); err == nil {
		t.Fatal("ожидалась ошибка переноса настроек на актив")
	}

	// Удаление счёта каскадно удаляет настройки.
	if _, err := db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, credit.ID); err != nil {
		t.Fatalf("удаление счёта: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM credit_settings WHERE account_id = ?`, credit.ID).Scan(&count); err != nil {
		t.Fatalf("подсчёт настроек: %v", err)
	}
	if count != 0 {
		t.Fatalf("настройки не удалены каскадом: %d", count)
	}
}

// TestASL_16_DepositSettingsConstraints проверяет ограничения вклада и
// ступенчатых ставок.
func TestASL_16_DepositSettingsConstraints(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	deposit := mustCreateAccount(t, db, "Вклад", model.TypeAsset, nil)
	credit := mustCreateAccount(t, db, "Кредит", model.TypeLiability, nil)

	insert := func(accountID int64) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO deposit_settings(account_id, base_rate_ppm, min_balance_kop,
			                              bonus_type, bonus_rate_ppm, bonus_amount_kop,
			                              accrual_frequency, capitalization,
			                              opened_at, closed_at, created_at, updated_at)
			 VALUES(?, 180000, 10000, 'none', 0, 0, 'monthly', 1, 1700000000, 0, 1, 1)`, accountID)
		return err
	}

	if err := insert(deposit.ID); err != nil {
		t.Fatalf("настройки листового актива: %v", err)
	}
	if err := insert(deposit.ID); err == nil {
		t.Fatal("ожидалась ошибка повторной настройки вклада")
	}
	if err := insert(credit.ID); err == nil || !strings.Contains(err.Error(), "листовым активом") {
		t.Fatalf("настройки пассива: %v", err)
	}

	for _, tc := range []struct {
		name  string
		query string
		arg   any
	}{
		{"отрицательный минимум", `UPDATE deposit_settings SET min_balance_kop = ? WHERE account_id = ?`, -1},
		{"неизвестный бонус", `UPDATE deposit_settings SET bonus_type = ? WHERE account_id = ?`, "cashback"},
		{"бонус-ставка без ставки", `UPDATE deposit_settings SET bonus_type = 'rate', bonus_rate_ppm = 0 WHERE account_id = ?`, deposit.ID},
		{"фиксированный бонус без суммы", `UPDATE deposit_settings SET bonus_type = 'fixed', bonus_amount_kop = 0 WHERE account_id = ?`, deposit.ID},
		{"капитализация не 0/1", `UPDATE deposit_settings SET capitalization = ? WHERE account_id = ?`, 2},
		{"закрытие раньше открытия", `UPDATE deposit_settings SET opened_at = ?, closed_at = ? WHERE account_id = ?`, 1700000000},
	} {
		var err error
		switch tc.name {
		case "бонус-ставка без ставки", "фиксированный бонус без суммы":
			_, err = db.ExecContext(ctx, tc.query, deposit.ID)
		case "закрытие раньше открытия":
			_, err = db.ExecContext(ctx, tc.query, 1700000000, 1600000000, deposit.ID)
		default:
			_, err = db.ExecContext(ctx, tc.query, tc.arg, deposit.ID)
		}
		if err == nil {
			t.Fatalf("%s: ожидалась ошибка", tc.name)
		}
	}

	// Ступенчатые ставки: корректный диапазон и проверки.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
		 VALUES(?, 0, 10000000, 150000)`, deposit.ID); err != nil {
		t.Fatalf("ступенчатая ставка: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
		 VALUES(?, 0, 20000000, 160000)`, deposit.ID); err == nil {
		t.Fatal("ожидалась ошибка повторной нижней границы")
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
		 VALUES(?, 10000000, 10000000, 160000)`, deposit.ID); err == nil {
		t.Fatal("ожидалась ошибка пустого диапазона")
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
		 VALUES(?, 10000000, 20000000, 1000001)`, deposit.ID); err == nil {
		t.Fatal("ожидалась ошибка ставки выше 100%")
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
		 VALUES(?, 10000000, 20000000, 160000)`, credit.ID); err == nil {
		t.Fatal("ожидалась ошибка ступени без настроек вклада")
	}

	// Удаление настроек вклада каскадно удаляет ступени, но не счёт.
	if _, err := db.ExecContext(ctx, `DELETE FROM deposit_settings WHERE account_id = ?`, deposit.ID); err != nil {
		t.Fatalf("удаление настроек вклада: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deposit_step_rates WHERE account_id = ?`, deposit.ID).Scan(&count); err != nil {
		t.Fatalf("подсчёт ступеней: %v", err)
	}
	if count != 0 {
		t.Fatalf("ступени не удалены каскадом: %d", count)
	}
	if _, err := db.Account(ctx, deposit.ID); err != nil {
		t.Fatalf("счёт удалён вместе с настройками: %v", err)
	}

	// Удаление счёта каскадно удаляет настройки и ступени.
	if err := insert(deposit.ID); err != nil {
		t.Fatalf("повторные настройки вклада: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_step_rates(account_id, from_kop, to_kop, rate_ppm)
		 VALUES(?, 0, 10000000, 150000)`, deposit.ID); err != nil {
		t.Fatalf("ступень перед удалением счёта: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, deposit.ID); err != nil {
		t.Fatalf("удаление счёта вклада: %v", err)
	}
	for _, table := range []string{"deposit_settings", "deposit_step_rates"} {
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM `+table+` WHERE account_id = ?`, deposit.ID).Scan(&count); err != nil {
			t.Fatalf("подсчёт %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s не удалены каскадом: %d", table, count)
		}
	}
}

// TestASL_16_InterestAccrualConstraints проверяет субледжер начислений:
// виды, статусы, идемпотентность периода и каскады.
func TestASL_16_InterestAccrualConstraints(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	userID := mustCreateUser(t, db, "Андрей")
	credit := mustCreateAccount(t, db, "Кредит", model.TypeLiability, nil)
	deposit := mustCreateAccount(t, db, "Вклад", model.TypeAsset, nil)
	other := mustCreateAccount(t, db, "Наличные", model.TypeAsset, nil)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO credit_settings(account_id, rate_ppm, created_at, updated_at)
		 VALUES(?, 150000, 1, 1)`, credit.ID); err != nil {
		t.Fatalf("настройки кредита: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO deposit_settings(account_id, base_rate_ppm, created_at, updated_at)
		 VALUES(?, 180000, 1, 1)`, deposit.ID); err != nil {
		t.Fatalf("настройки вклада: %v", err)
	}

	res, err := db.ExecContext(ctx,
		`INSERT INTO transactions(uuid, op_type, date, description, actor_id, created_at)
		 VALUES('tx-accrual', 'корректировка', 1700000000, 'начисление', ?, 1700000000)`, userID)
	if err != nil {
		t.Fatalf("проводка для начисления: %v", err)
	}
	txID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("id проводки: %v", err)
	}

	insert := func(uuid string, accountID int64, kind, status string, from, to int64) error {
		_, err := db.ExecContext(ctx,
			`INSERT INTO interest_accruals(uuid, account_id, date, kind, amount_kop,
			                               base_amount_kop, rate_ppm, period_from, period_to,
			                               status, transaction_id, created_at, updated_at)
			 VALUES(?, ?, 1700000000, ?, 10000, 1000000, 150000, ?, ?, ?, ?, 1, 1)`,
			uuid, accountID, kind, from, to, status, txID)
		return err
	}

	if err := insert("accrual-1", credit.ID, "credit_interest", "accrued", 1690000000, 1700000000); err != nil {
		t.Fatalf("начисление по кредиту: %v", err)
	}
	if err := insert("accrual-1", deposit.ID, "deposit_interest", "accrued", 0, 0); err == nil {
		t.Fatal("ожидалась ошибка повторного uuid")
	}
	if err := insert("accrual-2", credit.ID, "credit_interest", "accrued", 1690000000, 1700000000); err == nil {
		t.Fatal("ожидалась ошибка повторного периода")
	}
	if err := insert("accrual-3", deposit.ID, "deposit_bonus", "capitalized", 0, 0); err != nil {
		t.Fatalf("ручное начисление бонуса: %v", err)
	}
	if err := insert("accrual-4", deposit.ID, "deposit_bonus", "paid", 0, 0); err != nil {
		t.Fatalf("повторное ручное начисление: %v", err)
	}
	if err := insert("accrual-5", other.ID, "deposit_interest", "accrued", 0, 0); err == nil ||
		!strings.Contains(err.Error(), "нет настроек") {
		t.Fatalf("начисление без настроек: %v", err)
	}
	if err := insert("accrual-6", credit.ID, "unknown", "accrued", 0, 0); err == nil {
		t.Fatal("ожидалась ошибка неизвестного вида")
	}
	if err := insert("accrual-7", credit.ID, "credit_interest", "unknown", 0, 0); err == nil {
		t.Fatal("ожидалась ошибка неизвестного статуса")
	}
	if err := insert("accrual-8", credit.ID, "credit_interest", "accrued", 1700000000, 1600000000); err == nil {
		t.Fatal("ожидалась ошибка обратного периода")
	}

	// Удаление проводки обнуляет ссылку, начисление остаётся.
	if _, err := db.ExecContext(ctx, `DELETE FROM transactions WHERE id = ?`, txID); err != nil {
		t.Fatalf("удаление проводки: %v", err)
	}
	var linked any
	if err := db.QueryRowContext(ctx,
		`SELECT transaction_id FROM interest_accruals WHERE uuid = 'accrual-1'`).Scan(&linked); err != nil {
		t.Fatalf("чтение начисления: %v", err)
	}
	if linked != nil {
		t.Fatalf("transaction_id = %v, ожидался NULL", linked)
	}

	// Удаление счёта каскадно удаляет начисления.
	if _, err := db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, credit.ID); err != nil {
		t.Fatalf("удаление счёта кредита: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM interest_accruals WHERE account_id = ?`, credit.ID).Scan(&count); err != nil {
		t.Fatalf("подсчёт начислений: %v", err)
	}
	if count != 0 {
		t.Fatalf("начисления не удалены каскадом: %d", count)
	}
}

// TestASL_16_CreditDepositMigrationKeepsForeignKeys проверяет, что миграция
// не отключает внешние ключи и не ломает существующие счета.
func TestASL_16_CreditDepositMigrationKeepsForeignKeys(t *testing.T) {
	db := openTestDB(t)
	var enabled int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, ожидалось 1", enabled)
	}
	if _, err := db.Account(context.Background(), 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Account(нет счёта) = %v, ожидался ErrNotFound", err)
	}
}
