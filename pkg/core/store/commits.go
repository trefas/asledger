package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/trefas/asledger/pkg/core/model"
)

// ApplyCommit применяет пакет операций одним коммитом.
//
// Все проводки пакета записываются в одной транзакции БД: любая ошибка
// отклоняет пакет целиком и не оставляет ни одной проводки (атомарность,
// критерий эпика). Коммит хранит звено хэш-цепочки и результат head_seq.
func (db *DB) ApplyCommit(ctx context.Context, commit *model.Commit, transactions []*model.Transaction) ([]int64, error) {
	for _, tr := range transactions {
		if err := validateTransaction(tr); err != nil {
			return nil, err
		}
	}

	ids := make([]int64, 0, len(transactions))
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		warnings, err := json.Marshal(commit.Warnings)
		if err != nil {
			return fmt.Errorf("store: не удалось сохранить предупреждения коммита: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO commits(client_id, actor_id, base_seq, ops_json, result_seq, prev_hash, warnings, created_at)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			commit.ClientID, commit.ActorID, commit.BaseSeq, commit.OpsJSON,
			0, commit.Hash, string(warnings), Now())
		if err != nil {
			return fmt.Errorf("store: не удалось создать коммит: %w", err)
		}
		commitID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("store: не удалось получить id коммита: %w", err)
		}

		for _, tr := range transactions {
			tr.CommitID = commitID
			id, err := insertTransactionTx(ctx, tx, tr)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}

		var head sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(seq) FROM changelog`).Scan(&head); err != nil {
			return fmt.Errorf("store: не удалось прочитать head_seq: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE commits SET result_seq = ? WHERE id = ?`, head.Int64, commitID); err != nil {
			return fmt.Errorf("store: не удалось записать result_seq коммита: %w", err)
		}
		commit.ID = commitID
		commit.ResultSeq = head.Int64
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// CommitColumns — единый список колонок для чтения коммита.
const commitColumns = `id, client_id, actor_id, base_seq, ops_json, result_seq, prev_hash, warnings, created_at`

// LastCommit возвращает последний коммит (для звена хэш-цепочки).
func (db *DB) LastCommit(ctx context.Context) (*model.Commit, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+commitColumns+` FROM commits ORDER BY id DESC LIMIT 1`)
	commit, err := scanCommit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: коммиты", ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return commit, nil
}

// CommitsSince возвращает коммиты, у которых result_seq больше since,
// в порядке применения (для верификации хэш-цепочки клиентом).
func (db *DB) CommitsSince(ctx context.Context, since int64) ([]model.Commit, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+commitColumns+` FROM commits WHERE result_seq > ? ORDER BY id`, since)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать коммиты: %w", err)
	}
	defer rows.Close()

	var commits []model.Commit
	for rows.Next() {
		commit, err := scanCommit(rows)
		if err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать коммит: %w", err)
		}
		commits = append(commits, *commit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения коммитов: %w", err)
	}
	return commits, nil
}

// CommitByClientPayload ищет коммит клиента по содержимому пакета.
//
// Повторная отправка того же пакета (например, после обрыва связи) даёт тот же
// ответ, что исходный приём.
func (db *DB) CommitByClientPayload(ctx context.Context, clientID, opsJSON string) (*model.Commit, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+commitColumns+` FROM commits WHERE client_id = ? AND ops_json = ? ORDER BY id DESC LIMIT 1`,
		clientID, opsJSON)
	commit, err := scanCommit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: коммит клиента %q", ErrNotFound, clientID)
	}
	if err != nil {
		return nil, err
	}
	return commit, nil
}

// CommitByResultSeq возвращает коммит, после которого head_seq равен seq.
func (db *DB) CommitByResultSeq(ctx context.Context, seq int64) (*model.Commit, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+commitColumns+` FROM commits WHERE result_seq = ? ORDER BY id DESC LIMIT 1`, seq)
	commit, err := scanCommit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: коммит с result_seq %d", ErrNotFound, seq)
	}
	if err != nil {
		return nil, err
	}
	return commit, nil
}

// scanCommit разбирает строку выборки коммита.
func scanCommit(s rowScanner) (*model.Commit, error) {
	var (
		commit    model.Commit
		warnings  sql.NullString
		createdAt int64
	)
	if err := s.Scan(&commit.ID, &commit.ClientID, &commit.ActorID, &commit.BaseSeq,
		&commit.OpsJSON, &commit.ResultSeq, &commit.Hash, &warnings, &createdAt); err != nil {
		return nil, err
	}
	if warnings.Valid && warnings.String != "" {
		if err := json.Unmarshal([]byte(warnings.String), &commit.Warnings); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать предупреждения коммита: %w", err)
		}
	}
	commit.CreatedAt = fromUnix(createdAt)
	return &commit, nil
}
