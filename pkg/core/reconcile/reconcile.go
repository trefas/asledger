// Package reconcile реализует сценарий «Сверка счёта» (раздел 9.4 эпика):
// пользователь вводит фактический остаток, система показывает расхождение и
// может сгенерировать корректировку через служебный счёт «Капитал:Корректировка».
//
// Корректировка создаётся движком, поэтому балансовое уравнение (ΣDEBIT =
// ΣCREDIT) сохраняется, а служебная строка капитала добавляется автоматически.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// Service — сверка счетов поверх книги и движка.
type Service struct {
	db     *store.DB
	engine *engine.Engine
}

// New создаёт сервис сверки.
func New(db *store.DB) *Service {
	return &Service{db: db, engine: engine.New(db)}
}

// Request — запрос сверки счёта.
type Request struct {
	// AccountID — сверяемый счёт.
	AccountID int64
	// Fact — фактический остаток.
	Fact money.Kop
	// Date — дата сверки (для корректировки).
	Date time.Time
	// ActorID — автор сверки и корректировки.
	ActorID int64
	// Generate — сгенерировать корректировку на расхождение.
	Generate bool
	// UUID — идентификатор корректировки (генерирует клиент).
	UUID string
	// Source — источник корректировки ('web', 'api:…').
	Source string
}

// Result — результат сверки.
type Result struct {
	// Reconciliation — сохранённая сверка.
	Reconciliation model.Reconciliation
	// Difference — расхождение: фактический минус расчётный остаток.
	Difference money.Kop
	// Transaction — сгенерированная корректировка (если запрошена).
	Transaction *model.Transaction
}

// ErrNotAdjustable сообщает, что счёт нельзя скорректировать (доход/расход).
var ErrNotAdjustable = errors.New("reconcile: счёт этого типа нельзя скорректировать")

// Check выполняет сверку счёта и при необходимости создаёт корректировку.
//
// Расхождение считается от расчётного остатка на момент сверки. Корректировка
// создаётся движком в один шаг: счёт-пользователя с одной стороны и
// «Капитал:Корректировка» — с другой.
func (s *Service) Check(ctx context.Context, req Request) (*Result, error) {
	account, err := s.db.Account(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	expected, err := s.db.Balance(ctx, req.AccountID)
	if err != nil {
		return nil, err
	}
	difference := req.Fact - expected

	result := &Result{Difference: difference}
	result.Reconciliation = model.Reconciliation{
		AccountID: account.ID,
		Fact:      req.Fact,
		Expected:  expected,
		CheckedAt: time.Now().UTC(),
	}

	if req.Generate && difference != 0 {
		if !Adjustable(account.Type) {
			return nil, fmt.Errorf("%w: %q", ErrNotAdjustable, account.FullPath)
		}
		direction := engine.DirectionUp
		amount := difference
		if difference < 0 {
			direction = engine.DirectionDown
			amount = -difference
		}
		transaction, err := s.engine.Post(ctx, engine.Operation{
			OpType:      model.OpAdjustment,
			UUID:        req.UUID,
			Date:        req.Date,
			Description: "Сверка счёта: " + account.FullPath,
			Source:      req.Source,
			Direction:   direction,
			Entries: []engine.Entry{{
				AccountID: account.ID,
				Amount:    amount,
				Role:      engine.RoleAdjust,
			}},
		}, req.ActorID)
		if err != nil {
			return nil, err
		}
		result.Transaction = transaction
		id := transaction.ID
		result.Reconciliation.TransactionID = &id
	}

	id, err := s.db.RecordReconciliation(ctx, &result.Reconciliation)
	if err != nil {
		return nil, err
	}
	result.Reconciliation.ID = id
	return result, nil
}

// Discrepancy — расхождение по счёту.
type Discrepancy struct {
	// AccountID — счёт.
	AccountID int64
	// Path и Type — путь и тип счёта.
	Path string
	Type model.AccountType
	// Fact — фактический остаток последней сверки.
	Fact money.Kop
	// Balance — текущий расчётный остаток.
	Balance money.Kop
	// Difference — расхождение: факт минус текущий расчётный остаток.
	Difference money.Kop
	// CheckedAt — время последней сверки.
	CheckedAt time.Time
	// Adjustable — по счёту можно сгенерировать корректировку.
	Adjustable bool
}

// Discrepancies возвращает расхождения по счетам.
//
// Для каждого счёта берётся последняя сверка; расхождение считается от текущего
// остатка, поэтому после корректировки оно обнуляется.
func (s *Service) Discrepancies(ctx context.Context) ([]Discrepancy, error) {
	reconciliations, err := s.db.LatestReconciliations(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]model.Account, len(accounts))
	for i := range accounts {
		byID[accounts[i].ID] = accounts[i]
	}

	var out []Discrepancy
	for _, rec := range reconciliations {
		account, ok := byID[rec.AccountID]
		if !ok {
			continue
		}
		balance, err := s.db.Balance(ctx, rec.AccountID)
		if err != nil {
			return nil, err
		}
		out = append(out, Discrepancy{
			AccountID:  rec.AccountID,
			Path:       account.FullPath,
			Type:       account.Type,
			Fact:       rec.Fact,
			Balance:    balance,
			Difference: rec.Fact - balance,
			CheckedAt:  rec.CheckedAt,
			Adjustable: Adjustable(account.Type),
		})
	}
	return out, nil
}

// Adjustable сообщает, можно ли скорректировать счёт данного типа.
//
// Корректировка допустима для активов, обязательств и капитала; доходы и расходы
// правятся сторно или новой проводкой, а не сверкой остатка.
func Adjustable(t model.AccountType) bool {
	switch t {
	case model.TypeAsset, model.TypeLiability, model.TypeCapital:
		return true
	default:
		return false
	}
}
