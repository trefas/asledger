package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/trefas/asledger/pkg/core/model"
)

// UpsertDevice привязывает устройство clientID к пользователю и сохраняет
// хеш токена. Повторная привязка того же clientID перевыпускает токен.
func (db *DB) UpsertDevice(ctx context.Context, clientID string, userID int64, tokenHash string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO devices(client_id, user_id, token_hash, created_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(client_id) DO UPDATE SET user_id = excluded.user_id,
		                                      token_hash = excluded.token_hash,
		                                      created_at = excluded.created_at`,
		clientID, userID, tokenHash, Now())
	if err != nil {
		return fmt.Errorf("store: не удалось сохранить устройство %q: %w", clientID, err)
	}
	return nil
}

// DeviceByTokenHash возвращает устройство по хешу токена.
func (db *DB) DeviceByTokenHash(ctx context.Context, tokenHash string) (*model.Device, error) {
	row := db.QueryRowContext(ctx,
		`SELECT client_id, user_id, token_hash, created_at FROM devices WHERE token_hash = ?`, tokenHash)
	var (
		device    model.Device
		createdAt int64
	)
	err := row.Scan(&device.ClientID, &device.UserID, &device.TokenHash, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: устройство по токену", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать устройство: %w", err)
	}
	device.CreatedAt = fromUnix(createdAt)
	return &device, nil
}

// DeleteDevice отзывает устройство по clientID; отсутствие — не ошибка.
func (db *DB) DeleteDevice(ctx context.Context, clientID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM devices WHERE client_id = ?`, clientID); err != nil {
		return fmt.Errorf("store: не удалось отозвать устройство %q: %w", clientID, err)
	}
	return nil
}

// ListDevices возвращает привязанные устройства семьи.
func (db *DB) ListDevices(ctx context.Context) ([]model.Device, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT client_id, user_id, token_hash, created_at FROM devices ORDER BY created_at, client_id`)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать устройства: %w", err)
	}
	defer rows.Close()

	var devices []model.Device
	for rows.Next() {
		var (
			device    model.Device
			createdAt int64
		)
		if err := rows.Scan(&device.ClientID, &device.UserID, &device.TokenHash, &createdAt); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать устройство: %w", err)
		}
		device.CreatedAt = fromUnix(createdAt)
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения устройств: %w", err)
	}
	return devices, nil
}
