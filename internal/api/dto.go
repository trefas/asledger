package api

import (
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// lineDTO — строка проводки в ответе.
type lineDTO struct {
	AccountID int64      `json:"account_id"`
	Amount    money.Kop  `json:"amount_kop"`
	Side      model.Side `json:"side"`
}

// transactionDTO — проводка в ответе API.
type transactionDTO struct {
	ID          int64        `json:"id"`
	UUID        string       `json:"uuid"`
	OpType      model.OpType `json:"op_type"`
	Date        time.Time    `json:"date"`
	Description string       `json:"description,omitempty"`
	ReversesID  *int64       `json:"reverses_id,omitempty"`
	CommitID    int64        `json:"commit_id,omitempty"`
	Lines       []lineDTO    `json:"lines"`
	// Duplicates — возможные дубли проводки: плашка «Возможный дубль».
	Duplicates []syncsvc.Duplicate `json:"duplicates,omitempty"`
}

// toTransactionDTO переводит проводку в формат ответа.
func toTransactionDTO(tr *model.Transaction) transactionDTO {
	lines := make([]lineDTO, 0, len(tr.Lines))
	for _, line := range tr.Lines {
		lines = append(lines, lineDTO{AccountID: line.AccountID, Amount: line.Amount, Side: line.Side})
	}
	return transactionDTO{
		ID:          tr.ID,
		UUID:        tr.UUID,
		OpType:      tr.OpType,
		Date:        tr.Date.UTC(),
		Description: tr.Description,
		ReversesID:  tr.ReversesID,
		CommitID:    tr.CommitID,
		Lines:       lines,
	}
}

// accountDTO — счёт в ответе подсказок.
type accountDTO struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	FullPath string            `json:"full_path"`
	Type     model.AccountType `json:"type"`
	IsLeaf   bool              `json:"is_leaf"`
	// Usable — пригоден ли счёт для проводки текущей операции.
	Usable bool `json:"usable"`
}
