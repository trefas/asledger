package store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrTargetExists — целевой файл уже существует и не пуст.
var ErrTargetExists = errors.New("store: целевой файл уже существует")

// ExportEncrypted копирует открытую книгу в новый SQLCipher-файл с targetKey.
//
// Источник может быть как зашифрованным, так и незашифрованным. Копия
// создаётся через SQLCipher `sqlcipher_export` в отдельном подключении и не
// меняет исходную книгу. Целевой файл должен отсутствовать или быть пустым.
func (db *DB) ExportEncrypted(ctx context.Context, targetPath string, targetKey []byte) error {
	if len(targetKey) != keyLen {
		return fmt.Errorf("store: ключ должен быть %d байт, получено %d", keyLen, len(targetKey))
	}
	target, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("store: не удалось получить путь %q: %w", targetPath, err)
	}
	source, err := filepath.Abs(db.path)
	if err != nil {
		return fmt.Errorf("store: не удалось получить путь книги %q: %w", db.path, err)
	}
	if target == source {
		return errors.New("store: нельзя экспортировать книгу поверх самой себя")
	}
	if info, err := os.Stat(target); err == nil {
		if info.Size() > 0 {
			return fmt.Errorf("%w: %s", ErrTargetExists, target)
		}
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("store: не удалось удалить пустой целевой файл %q: %w", target, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: не удалось проверить целевой файл %q: %w", target, err)
	}

	// ATTACH привязан к конкретному соединению, поэтому весь экспорт
	// выполняется на одном *sql.Conn, а не через пул.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: не удалось получить соединение: %w", err)
	}
	defer func() { _ = conn.Close() }()

	hexKey := "x'" + hex.EncodeToString(targetKey) + "'"
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS migrated KEY ?", target, hexKey); err != nil {
		return fmt.Errorf("store: не удалось подключить целевой файл: %w", err)
	}
	attached := true
	defer func() {
		if attached {
			_, _ = conn.ExecContext(context.Background(), "DETACH DATABASE migrated")
		}
	}()

	if _, err := conn.ExecContext(ctx, "SELECT sqlcipher_export('migrated')"); err != nil {
		return fmt.Errorf("store: не удалось экспортировать книгу: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "DETACH DATABASE migrated"); err != nil {
		return fmt.Errorf("store: не удалось отключить целевой файл: %w", err)
	}
	attached = false

	if err := os.Chmod(target, 0o600); err != nil {
		return fmt.Errorf("store: не удалось выставить права на копию %q: %w", target, err)
	}
	return nil
}

// IntegrityCheck выполняет PRAGMA integrity_check и возвращает ошибку при
// любом результате, отличном от ok.
func (db *DB) IntegrityCheck(ctx context.Context) error {
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("store: integrity_check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var problems []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("store: integrity_check: %w", err)
		}
		if result != "ok" {
			problems = append(problems, result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: integrity_check: %w", err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("store: integrity_check нашёл проблемы: %v", problems)
	}
	return nil
}
