package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/trefas/asledger/pkg/core/model"
)

// appendChangelog добавляет запись в журнал изменений внутри транзакции.
//
// Журнал используется синхронизацией для выдачи дельты (pull) и построения
// хэш-цепочки коммитов.
func appendChangelog(ctx context.Context, tx *sql.Tx, entity string, id int64, op string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO changelog(entity, entity_id, op, changed_at) VALUES(?, ?, ?, ?)`,
		entity, id, op, Now())
	if err != nil {
		return fmt.Errorf("store: не удалось записать изменение %s/%d: %w", entity, id, err)
	}
	return nil
}

// Changelog возвращает записи журнала с seq больше since в порядке возрастания.
func (db *DB) Changelog(ctx context.Context, since int64) ([]model.ChangelogEntry, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT seq, entity, entity_id, op, changed_at
		   FROM changelog
		  WHERE seq > ?
		  ORDER BY seq`, since)
	if err != nil {
		return nil, fmt.Errorf("store: не удалось прочитать журнал изменений: %w", err)
	}
	defer rows.Close()

	var entries []model.ChangelogEntry
	for rows.Next() {
		var e model.ChangelogEntry
		var changedAt int64
		if err := rows.Scan(&e.Seq, &e.Entity, &e.EntityID, &e.Op, &changedAt); err != nil {
			return nil, fmt.Errorf("store: не удалось разобрать запись журнала: %w", err)
		}
		e.ChangedAt = fromUnix(changedAt)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: ошибка чтения журнала изменений: %w", err)
	}
	return entries, nil
}

// HeadSeq возвращает наибольший seq журнала изменений (0, если журнал пуст).
func (db *DB) HeadSeq(ctx context.Context) (int64, error) {
	var seq sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(seq) FROM changelog`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("store: не удалось прочитать head_seq: %w", err)
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}
