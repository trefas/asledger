package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// Status — исход приёма пакета операций (раздел 10 эпика).
type Status string

// Исходы приёма пакета.
const (
	// StatusAccepted — пакет принят полностью.
	StatusAccepted Status = "accepted"
	// StatusAcceptedWithWarnings — пакет принят с предупреждениями (rebase, дубли).
	StatusAcceptedWithWarnings Status = "accepted-with-warnings"
	// StatusRejected — пакет отклонён целиком, книга не изменена.
	StatusRejected Status = "rejected"
)

// Entry — строка операции в пакете клиента.
type Entry struct {
	AccountID int64     `json:"account_id"`
	Amount    money.Kop `json:"amount_kop"`
	Role      string    `json:"role"`
}

// Operation — операция пакета клиента.
type Operation struct {
	OpType      model.OpType `json:"op_type"`
	UUID        string       `json:"uuid"`
	Date        time.Time    `json:"date"`
	Description string       `json:"description,omitempty"`
	Direction   string       `json:"direction,omitempty"`
	Entries     []Entry      `json:"entries"`
}

// CommitPayload — содержимое коммита: именно оно попадает в ops_json и хэш.
type CommitPayload struct {
	CommitID   string      `json:"commit_id"`
	Operations []Operation `json:"operations"`
}

// PushRequest — запрос коммита операций.
type PushRequest struct {
	// ClientID — идентификатор устройства.
	ClientID string
	// ActorID — пользователь, от имени которого применяется пакет.
	ActorID int64
	// BaseSeq — seq, на котором стоял клиент при отправке.
	BaseSeq int64
	// CommitID — идентификатор коммита клиента.
	CommitID string
	// Operations — операции пакета.
	Operations []Operation
}

// PushResult — исход приёма пакета.
type PushResult struct {
	Status    Status   `json:"status"`
	CommitID  string   `json:"commit_id"`
	ResultSeq int64    `json:"result_seq"`
	Applied   []string `json:"applied"`
	Warnings  []string `json:"warnings"`
	// Duplicates — возможные дубли принятых проводок (эвристика раздела 9.3).
	Duplicates []Duplicate `json:"duplicates,omitempty"`
}

// Duplicate — возможный дубль новой проводки среди уже записанных.
//
// Плашка «Возможный дубль» показывается клиенту и в web-интерфейсе; коммит при
// этом принимается, а отменить запись можно сторно.
type Duplicate struct {
	// UUID — uuid новой проводки.
	UUID string `json:"uuid"`
	// Existing — uuid уже записанных проводок с той же подписью.
	Existing []string `json:"existing"`
}

// ErrRejected — пакет отклонён целиком; причина разбирается вызывающим кодом
// как код валидации движка.
var ErrRejected = errors.New("sync: пакет отклонён")

// Push принимает пакет операций.
//
// base_seq == head_seq — fast-forward: пакет применяется целиком в одной
// транзакции БД, любая ошибка отклоняет пакет без изменений. base_seq <
// head_seq — rebase: операции переигрываются поверх головы, конфликты и дубли
// попадают в предупреждения. Повторная отправка того же пакета идемпотентна.
func (s *Service) Push(ctx context.Context, req PushRequest) (*PushResult, error) {
	payload := CommitPayload{CommitID: req.CommitID, Operations: req.Operations}
	opsJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sync: не удалось сериализовать пакет: %w", err)
	}

	if existing, err := s.db.CommitByClientPayload(ctx, req.ClientID, string(opsJSON)); err == nil {
		return s.resultFromCommit(ctx, existing, payload)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	head, err := s.db.HeadSeq(ctx)
	if err != nil {
		return nil, err
	}

	var warnings []string
	applied := make([]*model.Transaction, 0, len(req.Operations))
	var duplicates []Duplicate
	seen := make(map[string]bool, len(req.Operations))
	for _, op := range req.Operations {
		if seen[op.UUID] {
			warnings = append(warnings, fmt.Sprintf("операция %s: дубликат uuid в пакете, пропущена", op.UUID))
			continue
		}
		seen[op.UUID] = true

		prepared, err := s.engine.Prepare(ctx, op.EngineOperation(req.ClientID), req.ActorID)
		if err != nil {
			return &PushResult{
				Status:    StatusRejected,
				CommitID:  req.CommitID,
				ResultSeq: head,
				Warnings:  warnings,
			}, fmt.Errorf("%w: %w", ErrRejected, err)
		}
		if prepared.Existing {
			warnings = append(warnings, fmt.Sprintf("операция %s уже применена, пропущена", op.UUID))
			continue
		}
		existing, err := s.duplicateUUIDs(ctx, prepared.Transaction)
		if err != nil {
			return nil, err
		}
		if len(existing) > 0 {
			// Дубль не блокирует приём: проводка принимается, а пользователь
			// получает плашку «Возможный дубль» с действием «отменить».
			duplicates = append(duplicates, Duplicate{UUID: op.UUID, Existing: existing})
			warnings = append(warnings, fmt.Sprintf(
				"операция %s: возможный дубль — совпадают тип, дата, счета и суммы", op.UUID))
		}
		applied = append(applied, prepared.Transaction)
	}

	commit := &model.Commit{
		ClientID: req.ClientID,
		ActorID:  req.ActorID,
		BaseSeq:  req.BaseSeq,
		OpsJSON:  string(opsJSON),
		Warnings: warnings,
	}
	if _, err := s.db.ApplyCommit(ctx, commit, applied); err != nil {
		return nil, err
	}
	for _, tr := range applied {
		if tr.OpType == model.OpInitial {
			if err := s.engine.MarkInitialApplied(ctx); err != nil {
				return nil, err
			}
		}
	}

	uuids := make([]string, 0, len(applied))
	for _, tr := range applied {
		uuids = append(uuids, tr.UUID)
	}
	return &PushResult{
		Status:     statusFor(commit.Warnings),
		CommitID:   req.CommitID,
		ResultSeq:  commit.ResultSeq,
		Applied:    uuids,
		Warnings:   commit.Warnings,
		Duplicates: duplicates,
	}, nil
}

// resultFromCommit повторяет ответ ранее принятого пакета (идемпотентность).
func (s *Service) resultFromCommit(ctx context.Context, commit *model.Commit, payload CommitPayload) (*PushResult, error) {
	applied := make([]string, 0, len(payload.Operations))
	seen := make(map[string]bool, len(payload.Operations))
	for _, op := range payload.Operations {
		if seen[op.UUID] {
			continue
		}
		seen[op.UUID] = true
		tr, err := s.db.TransactionByUUID(ctx, op.UUID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if tr.CommitID == commit.ID {
			applied = append(applied, tr.UUID)
		}
	}
	return &PushResult{
		Status:    statusFor(commit.Warnings),
		CommitID:  payload.CommitID,
		ResultSeq: commit.ResultSeq,
		Applied:   applied,
		Warnings:  commit.Warnings,
	}, nil
}

// statusFor выводит исход приёма из списка предупреждений.
func statusFor(warnings []string) Status {
	if len(warnings) > 0 {
		return StatusAcceptedWithWarnings
	}
	return StatusAccepted
}

// Duplicates возвращает плашку «Возможный дубль» для проводки.
//
// Используется web-интерфейсом после онлайн-проводки: если найдены совпадения,
// форма показывает плашку с действием «отменить» (сторно).
func (s *Service) Duplicates(ctx context.Context, tr *model.Transaction) ([]Duplicate, error) {
	existing, err := s.duplicateUUIDs(ctx, tr)
	if err != nil {
		return nil, err
	}
	if len(existing) == 0 {
		return nil, nil
	}
	return []Duplicate{{UUID: tr.UUID, Existing: existing}}, nil
}

// duplicateUUIDs ищет проводки, совпадающие по типу, дате, счетам и суммам.
//
// Это эвристика раздела 9.3: коммит принимается, но клиент и web-интерфейс
// получают плашку «Возможный дубль» с действием «отменить». Ложные срабатывания
// возможны только при полном совпадении подписи в пределах одного дня, поэтому
// обычные операции разных дней не помечаются.
func (s *Service) duplicateUUIDs(ctx context.Context, tr *model.Transaction) ([]string, error) {
	dayStart := time.Date(tr.Date.Year(), tr.Date.Month(), tr.Date.Day(), 0, 0, 0, 0, tr.Date.Location())
	existing, err := s.db.ListTransactions(ctx, store.TransactionFilter{
		From: dayStart.Unix(),
		To:   dayStart.AddDate(0, 0, 1).Unix() - 1,
	})
	if err != nil {
		return nil, err
	}
	signature := transactionSignature(tr)
	var matches []string
	for i := range existing {
		other := &existing[i]
		if other.OpType != tr.OpType {
			continue
		}
		if transactionSignature(other) == signature {
			matches = append(matches, other.UUID)
		}
	}
	sort.Strings(matches)
	return matches, nil
}

// transactionSignature — устойчивая подпись проводки для поиска дублей.
func transactionSignature(tr *model.Transaction) string {
	parts := make([]string, 0, len(tr.Lines))
	for _, line := range tr.Lines {
		parts = append(parts, fmt.Sprintf("%d:%s:%d", line.AccountID, line.Side, line.Amount.Kopecks()))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// EngineOperation переводит операцию пакета в запрос движка.
func (o Operation) EngineOperation(clientID string) engine.Operation {
	entries := make([]engine.Entry, 0, len(o.Entries))
	for _, e := range o.Entries {
		entries = append(entries, engine.Entry{
			AccountID: e.AccountID,
			Amount:    e.Amount,
			Role:      engine.Role(e.Role),
		})
	}
	return engine.Operation{
		OpType:      o.OpType,
		UUID:        o.UUID,
		Date:        o.Date,
		Description: o.Description,
		Source:      "api:" + clientID,
		Direction:   engine.Direction(o.Direction),
		Entries:     entries,
	}
}
