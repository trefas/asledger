package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/trefas/asledger/pkg/core/auth"
	"github.com/trefas/asledger/pkg/core/model"
)

// SettingInitialBalances — ключ настройки «начальные остатки введены».
//
// Флаг фиксирует, что стартовые остатки внесены, и запрещает повторный ввод.
const SettingInitialBalances = "начальные_остатки_введены"

// SettingBalanceMode — ключ настройки режима контроля остатка (раздел 9.2).
const SettingBalanceMode = "режим_контроля_остатка"

// SettingDevicesEnrollment — ключ настройки привязки новых устройств.
//
// Значение store.EnrollmentAllow разрешает привязку из локальной сети,
// store.EnrollmentDeny запрещает её. Отсутствие настройки означает allow.
const SettingDevicesEnrollment = "привязка_устройств"

// Значения настройки привязки устройств.
const (
	// EnrollmentAllow — привязка новых устройств разрешена.
	EnrollmentAllow = "allow"
	// EnrollmentDeny — привязка новых устройств запрещена.
	EnrollmentDeny = "deny"
)

// Режимы контроля остатка.
const (
	// BalanceModeFree — проводка принимается, минус подсвечивается.
	BalanceModeFree = "свободный"
	// BalanceModeLimited — проводка ниже min_balance отклоняется.
	BalanceModeLimited = "лимитный"
)

// CreateUser создаёт пользователя семьи.
//
// Секрет (пароль или токен) хешируется PBKDF2-HMAC-SHA256 со случайной солью:
// в колонке token_hash открытое значение никогда не сохраняется.
func (db *DB) CreateUser(ctx context.Context, name, secret string) (*model.User, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("store: имя пользователя не может быть пустым")
	}
	tokenHash, err := auth.Hash(secret)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось захешировать секрет: %w", err)
	}
	res, err := db.ExecContext(ctx, `INSERT INTO users(name, token_hash) VALUES(?, ?)`, name, tokenHash)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("store: пользователь %q уже существует", name)
		}
		return nil, fmt.Errorf("store: не удалось создать пользователя: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: не удалось получить id пользователя: %w", err)
	}
	return &model.User{ID: id, Name: name, TokenHash: tokenHash}, nil
}

// RenameUser переименовывает пользователя семьи.
//
// Пользователи и их устройства не входят в журнал изменений: они передаются
// новым клиентам вместе со снапшотом книги.
func (db *DB) RenameUser(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("store: имя пользователя не может быть пустым")
	}
	res, err := db.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("store: пользователь %q уже существует", name)
		}
		return fmt.Errorf("store: не удалось переименовать пользователя: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: не удалось переименовать пользователя: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: пользователь %d", ErrNotFound, id)
	}
	return nil
}

// Authenticate проверяет секрет пользователя и при успехе возвращает его.
//
// Отсутствие пользователя даёт ErrNotFound, несовпадение секрета — ErrAuthFailed.
func (db *DB) Authenticate(ctx context.Context, name, secret string) (*model.User, error) {
	user, err := db.UserByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if !auth.Verify(user.TokenHash, secret) {
		return nil, fmt.Errorf("%w: %s", ErrAuthFailed, name)
	}
	return user, nil
}

// User возвращает пользователя по идентификатору.
func (db *DB) User(ctx context.Context, id int64) (*model.User, error) {
	row := db.QueryRowContext(ctx, `SELECT id, name, token_hash FROM users WHERE id = ?`, id)
	return scanUser(row, id, "")
}

// UserByName возвращает пользователя по имени.
func (db *DB) UserByName(ctx context.Context, name string) (*model.User, error) {
	row := db.QueryRowContext(ctx, `SELECT id, name, token_hash FROM users WHERE name = ?`, name)
	return scanUser(row, 0, name)
}

// ListUsers возвращает всех пользователей семьи.
func (db *DB) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, token_hash FROM users ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать пользователей: %w", err)
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Name, &u.TokenHash); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать пользователя: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения пользователей: %w", err)
	}
	return users, nil
}

// SetSetting сохраняет значение настройки.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("store: не удалось сохранить настройку %q: %w", key, err)
	}
	return nil
}

// Setting возвращает значение настройки; второй результат — признак наличия.
func (db *DB) Setting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: не удалось прочитать настройку %q: %w", key, err)
	}
	return value, true, nil
}

// Settings возвращает все настройки книги.
func (db *DB) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать настройки: %w", err)
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать настройку: %w", err)
		}
		settings[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения настроек: %w", err)
	}
	return settings, nil
}

// DeleteSetting удаляет настройку; отсутствие ключа ошибкой не считается.
func (db *DB) DeleteSetting(ctx context.Context, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("store: не удалось удалить настройку %q: %w", key, err)
	}
	return nil
}

// scanUser разбирает строку выборки пользователя.
func scanUser(s rowScanner, id int64, name string) (*model.User, error) {
	var u model.User
	if err := s.Scan(&u.ID, &u.Name, &u.TokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: пользователь %d/%q", ErrNotFound, id, name)
		}
		return nil, err
	}
	return &u, nil
}
