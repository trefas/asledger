package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Миграции встраиваются в бинарник и применяются по возрастанию версии.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration — одна миграция схемы.
type migration struct {
	version int
	name    string
	body    string
}

// loadMigrations читает и сортирует встроенные миграции.
//
// Имя файла имеет вид <версия>_<описание>.sql, например 0001_init.sql.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать миграции: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, err := parseVersion(e.Name())
		if err != nil {
			return nil, err
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("store: не удалось прочитать миграцию %q: %w", e.Name(), err)
		}
		migrations = append(migrations, migration{
			version: version,
			name:    e.Name(),
			body:    string(body),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}

// parseVersion извлекает номер версии из имени файла миграции.
func parseVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("store: имя миграции %q должно иметь вид <версия>_<описание>.sql", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("store: не удалось разобрать версию миграции %q: %w", name, err)
	}
	return version, nil
}

// Migrate применяет все неприменённые миграции в порядке версий.
//
// Миграции только прямые (forward-only) и идемпотентны: повторный вызов
// не применяет ничего повторно.
func (db *DB) Migrate(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("store: не удалось создать таблицу миграций: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		applied, err := db.migrationApplied(ctx, m.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	// Служебные счета создаются после схемы: строка капитала генерируется
	// движком, а ветка «Активы:Дебиторка» используется операцией займа.
	return db.EnsureSystemAccounts(ctx)
}

// migrationApplied сообщает, применена ли миграция указанной версии.
func (db *DB) migrationApplied(ctx context.Context, version int) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("store: не удалось проверить миграцию %d: %w", version, err)
	}
	return count > 0, nil
}

// applyMigration применяет одну миграцию в отдельной транзакции.
func (db *DB) applyMigration(ctx context.Context, m migration) error {
	return db.Tx(ctx, func(tx *sql.Tx) error {
		for _, stmt := range splitStatements(m.body) {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("store: миграция %s не применилась: %w", m.name, err)
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, Now())
		if err != nil {
			return fmt.Errorf("store: не удалось записать миграцию %s: %w", m.name, err)
		}
		return nil
	})
}

// splitStatements делит текст миграции на отдельные инструкции.
//
// Точка с запятой внутри строки, комментария или блока BEGIN…END (тело
// триггера) не завершает инструкцию. Строчные комментарии «--» и блочные
// «/* */» отбрасываются, чтобы инструкции оставались читаемыми в тестах.
func splitStatements(sqlText string) []string {
	var (
		b          strings.Builder
		statements []string
		blocks     []string
	)
	runes := []rune(sqlText)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\'':
			b.WriteRune(c)
			for i++; i < len(runes); i++ {
				b.WriteRune(runes[i])
				if runes[i] != '\'' {
					continue
				}
				if i+1 < len(runes) && runes[i+1] == '\'' {
					i++
					b.WriteRune(runes[i])
					continue
				}
				break
			}
		case c == '"':
			b.WriteRune(c)
			for i++; i < len(runes); i++ {
				b.WriteRune(runes[i])
				if runes[i] != '"' {
					continue
				}
				if i+1 < len(runes) && runes[i+1] == '"' {
					i++
					b.WriteRune(runes[i])
					continue
				}
				break
			}
		case c == '`':
			b.WriteRune(c)
			for i++; i < len(runes); i++ {
				b.WriteRune(runes[i])
				if runes[i] == '`' {
					break
				}
			}
		case c == '[':
			b.WriteRune(c)
			for i++; i < len(runes); i++ {
				b.WriteRune(runes[i])
				if runes[i] == ']' {
					break
				}
			}
		case c == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			if i < len(runes) {
				b.WriteRune('\n')
			}
		case c == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i++
			b.WriteRune(' ')
		case c == ';' && len(blocks) == 0:
			if s := strings.TrimSpace(b.String()); s != "" {
				statements = append(statements, s)
			}
			b.Reset()
		case isWordStart(c):
			start := i
			for i+1 < len(runes) && isWordPart(runes[i+1]) {
				i++
			}
			word := strings.ToUpper(string(runes[start : i+1]))
			switch word {
			case "BEGIN", "CASE":
				blocks = append(blocks, word)
			case "END":
				if len(blocks) > 0 {
					blocks = blocks[:len(blocks)-1]
				}
			}
			b.WriteString(string(runes[start : i+1]))
		default:
			b.WriteRune(c)
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		statements = append(statements, s)
	}
	return statements
}

// isWordStart сообщает, может ли руна начинать слово SQL.
func isWordStart(r rune) bool {
	return r == '_' ||
		(r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

// isWordPart сообщает, может ли руна продолжать слово SQL.
func isWordPart(r rune) bool {
	return isWordStart(r) || (r >= '0' && r <= '9')
}
