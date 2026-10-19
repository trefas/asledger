package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// Reverse создаёт сторно проводки: те же счета, противоположные стороны и
// reverses_id, указывающий на отменяемую запись.
//
// Отменяемая запись должна существовать, не быть сторно (цепочек «сторно
// сторно» нет) и не быть уже отменённой. Повторная отмена идемпотентна:
// возвращается уже созданная инверсная проводка.
func (e *Engine) Reverse(ctx context.Context, originalID, actorID int64, uuid string, date time.Time, description string) (*model.Transaction, error) {
	if strings.TrimSpace(uuid) == "" {
		return nil, NewError(CodeUUIDEmpty, "не задан uuid сторно")
	}

	original, err := e.db.Transaction(ctx, originalID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, NewError(CodeReverseInvalid, "отменяемая проводка не найдена").WithCause(err)
	}
	if err != nil {
		return nil, err
	}
	if original.ReversesID != nil {
		return nil, NewError(CodeReverseInvalid, "нельзя отменить сторно")
	}

	existing, err := e.reversalOf(ctx, originalID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	lines := make([]model.TransactionLine, 0, len(original.Lines))
	for _, line := range original.Lines {
		lines = append(lines, model.TransactionLine{
			AccountID: line.AccountID,
			Amount:    line.Amount,
			Side:      line.Side.Opposite(),
		})
	}

	tr := &model.Transaction{
		UUID:        uuid,
		OpType:      original.OpType,
		Date:        date,
		Description: description,
		ActorID:     actorID,
		ReversesID:  &originalID,
		Lines:       lines,
	}
	if tr.Date.IsZero() {
		tr.Date = e.now()
	}
	// Сторно — инверсия уже принятой операции, поэтому ячейки матрицы не
	// перепроверяются; сквозные инварианты (листовые счета, положительные
	// суммы, баланс, дата, min_balance) проверяются.
	if err := e.validate(ctx, tr, reversedChecks); err != nil {
		return nil, err
	}

	id, err := e.db.InsertTransaction(ctx, tr)
	if err != nil {
		return nil, mapStoreError(err)
	}
	return e.db.Transaction(ctx, id)
}

// reversalOf возвращает существующее сторно проводки, если оно есть.
func (e *Engine) reversalOf(ctx context.Context, originalID int64) (*model.Transaction, error) {
	found, err := e.db.ListTransactions(ctx, store.TransactionFilter{ReversesID: &originalID, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}
