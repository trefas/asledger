package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// ChartNode — узел вложенного плана счетов в формате импорта JSON.
//
// MinBalance задаётся в копейках (0 для наличных, отрицательное значение —
// овердрафт); денежные величины не используют числа с плавающей точкой.
type ChartNode struct {
	Name       string            `json:"name"`
	Type       model.AccountType `json:"type"`
	MinBalance int64             `json:"min_balance,omitempty"`
	Children   []ChartNode       `json:"children,omitempty"`
}

// Chart — план счетов в формате импорта JSON.
//
// Допускаются обе формы файла: объект с полем accounts и просто массив узлов.
type Chart struct {
	Name     string      `json:"name,omitempty"`
	Accounts []ChartNode `json:"accounts"`
}

// ImportReport — итог импорта плана счетов.
type ImportReport struct {
	// Created — число созданных счетов.
	Created int
	// Existing — число путей, уже присутствовавших в книге (пропущены).
	Existing int
}

// ParseChart разбирает план счетов из JSON.
//
// Формат вложенный: узел содержит name, type, min_balance и children.
// Неизвестные поля считаются опечаткой и отклоняются.
func ParseChart(data []byte) (*Chart, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("store: пустой план счетов")
	}

	if trimmed[0] == '[' {
		var nodes []ChartNode
		if err := decodeStrict(trimmed, &nodes); err != nil {
			return nil, err
		}
		return &Chart{Accounts: nodes}, nil
	}

	var chart Chart
	if err := decodeStrict(trimmed, &chart); err != nil {
		return nil, err
	}
	return &chart, nil
}

// decodeStrict разбирает JSON, отклоняя неизвестные поля.
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("store: не удалось разобрать план счетов: %w", err)
	}
	return nil
}

// flatAccount — узел плана счетов, развёрнутый в линейный список.
type flatAccount struct {
	name       string
	typ        model.AccountType
	minBalance money.Kop
	path       string
	parent     int // индекс родителя в списке, -1 для корня
}

// ValidateChart проверяет структуру плана счетов: имена, допустимость типов,
// согласованность типов по ветке и отсутствие дубликатов путей.
func ValidateChart(chart *Chart) error {
	_, err := flattenChart(chart)
	return err
}

// ImportChart импортирует план счетов целиком в одной БД-транзакции.
//
// Сначала дерево проверяется (имена, типы, дубликаты путей), и только затем
// счета создаются. Ошибка проверки не оставляет частично импортированного
// дерева. Пути, уже присутствующие в книге, пропускаются — повторный импорт
// шаблона идемпотентен.
func (db *DB) ImportChart(ctx context.Context, chart *Chart) (ImportReport, error) {
	flat, err := flattenChart(chart)
	if err != nil {
		return ImportReport{}, err
	}

	var report ImportReport
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		report = ImportReport{}
		ids := make([]int64, len(flat))
		for i, node := range flat {
			existing, err := accountIDByPathTx(ctx, tx, node.path)
			if err == nil {
				ids[i] = existing
				report.Existing++
				continue
			}
			if !errors.Is(err, ErrNotFound) {
				return err
			}

			var parentID *int64
			if node.parent >= 0 {
				pid := ids[node.parent]
				parentID = &pid
			}
			id, err := insertAccountTx(ctx, tx, AccountInput{
				Name: node.name, Type: node.typ, ParentID: parentID, MinBalance: node.minBalance,
			})
			if err != nil {
				return err
			}
			ids[i] = id
			report.Created++
		}
		return db.rebuildPathsTx(ctx, tx)
	})
	if err != nil {
		return ImportReport{}, err
	}
	return report, nil
}

// flattenChart проверяет план счетов и разворачивает дерево в линейный список
// в порядке родитель-перед-потомком.
func flattenChart(chart *Chart) ([]flatAccount, error) {
	if chart == nil || len(chart.Accounts) == 0 {
		return nil, errors.New("store: план счетов пуст")
	}

	var flat []flatAccount
	seen := map[string]bool{}

	var walk func(nodes []ChartNode, parent int, parentPath string, parentType model.AccountType) error
	walk = func(nodes []ChartNode, parent int, parentPath string, parentType model.AccountType) error {
		for _, node := range nodes {
			name := strings.TrimSpace(node.Name)
			if err := validateAccountName(name); err != nil {
				return err
			}
			if !node.Type.Valid() {
				return fmt.Errorf("store: недопустимый тип счёта %q для %q", node.Type, name)
			}
			if parentPath != "" && parentType != node.Type {
				return fmt.Errorf("store: тип счёта %q не совпадает с типом родителя %q (%s)",
					node.Type, parentPath, parentType)
			}

			fullPath := name
			if parentPath != "" {
				fullPath = parentPath + pathSeparator + name
			}
			if seen[fullPath] {
				return fmt.Errorf("%w: %s", ErrDuplicatePath, fullPath)
			}
			seen[fullPath] = true

			index := len(flat)
			flat = append(flat, flatAccount{
				name: name, typ: node.Type, minBalance: money.FromKopecks(node.MinBalance),
				path: fullPath, parent: parent,
			})
			if err := walk(node.Children, index, fullPath, node.Type); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(chart.Accounts, -1, "", ""); err != nil {
		return nil, err
	}
	return flat, nil
}

// insertAccountTx создаёт счёт в рамках транзакции и пишет запись журнала.
func insertAccountTx(ctx context.Context, tx *sql.Tx, in AccountInput) (int64, error) {
	fullPath := in.Name
	if in.ParentID != nil {
		parent, err := getAccountTx(ctx, tx, *in.ParentID)
		if err != nil {
			return 0, err
		}
		fullPath = parent.FullPath + pathSeparator + in.Name
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO accounts(name, full_path, type, parent_id, is_leaf, min_balance, is_archived, is_system, created_at)
		 VALUES(?, ?, ?, ?, 1, ?, 0, 0, ?)`,
		in.Name, fullPath, string(in.Type), nullableID(in.ParentID), int64(in.MinBalance), Now())
	if err != nil {
		if isUniqueViolation(err) {
			return 0, fmt.Errorf("%w: %s", ErrDuplicatePath, fullPath)
		}
		return 0, fmt.Errorf("store: не удалось создать счёт %q: %w", fullPath, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: не удалось получить id счёта: %w", err)
	}
	if err := appendChangelog(ctx, tx, model.EntityAccount, id, model.OpInsert); err != nil {
		return 0, err
	}
	return id, nil
}

// accountIDByPathTx возвращает id счёта по пути в рамках транзакции.
func accountIDByPathTx(ctx context.Context, tx *sql.Tx, fullPath string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE full_path = ?`, fullPath).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: не удалось найти счёт %q: %w", fullPath, err)
	}
	return id, nil
}
