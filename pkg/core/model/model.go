// Package model описывает доменные типы asledger: план счетов, транзакции,
// проводки и служебные сущности.
//
// Денежные величины представляются типом money.Kop (целые копейки).
package model

import (
	"time"

	"github.com/trefas/asledger/pkg/core/money"
)

// AccountType — тип счёта плана счетов.
type AccountType string

// Допустимые типы счетов.
const (
	TypeAsset     AccountType = "актив"
	TypeLiability AccountType = "пассив"
	TypeCapital   AccountType = "капитал"
	TypeIncome    AccountType = "доход"
	TypeExpense   AccountType = "расход"
)

// AccountTypes возвращает все допустимые типы счетов.
func AccountTypes() []AccountType {
	return []AccountType{TypeAsset, TypeLiability, TypeCapital, TypeIncome, TypeExpense}
}

// Valid сообщает, является ли тип счёта допустимым.
func (t AccountType) Valid() bool {
	switch t {
	case TypeAsset, TypeLiability, TypeCapital, TypeIncome, TypeExpense:
		return true
	default:
		return false
	}
}

// Side — сторона бухгалтерской проводки. Сторону назначает движок по матрице
// правил, а не пользователь.
type Side string

// Стороны проводки.
const (
	SideDebit  Side = "DEBIT"
	SideCredit Side = "CREDIT"
)

// Valid сообщает, является ли сторона допустимой.
func (s Side) Valid() bool { return s == SideDebit || s == SideCredit }

// Opposite возвращает противоположную сторону (для сторно и инверсии).
func (s Side) Opposite() Side {
	if s == SideDebit {
		return SideCredit
	}
	return SideDebit
}

// OpType — тип операции.
type OpType string

// Типы операций матрицы ASL-2.2.
const (
	OpExpense     OpType = "трата"
	OpIncome      OpType = "доход"
	OpTransfer    OpType = "перевод"
	OpDebtPayment OpType = "погашение_долга"
	OpLoan        OpType = "выдача_займа"
	OpInitial     OpType = "начальные_остатки"
	OpRevaluation OpType = "переоценка"
	OpAdjustment  OpType = "корректировка"
)

// OpTypes возвращает все допустимые типы операций.
func OpTypes() []OpType {
	return []OpType{
		OpExpense, OpIncome, OpTransfer, OpDebtPayment, OpLoan,
		OpInitial, OpRevaluation, OpAdjustment,
	}
}

// MVPOpTypes возвращает типы операций, реализуемые в первую очередь.
func MVPOpTypes() []OpType { return []OpType{OpExpense, OpIncome, OpTransfer, OpInitial} }

// Valid сообщает, является ли тип операции допустимым.
func (o OpType) Valid() bool {
	switch o {
	case OpExpense, OpIncome, OpTransfer, OpDebtPayment, OpLoan,
		OpInitial, OpRevaluation, OpAdjustment:
		return true
	default:
		return false
	}
}

// Account — счёт плана счетов.
type Account struct {
	ID         int64
	Name       string
	FullPath   string
	Type       AccountType
	ParentID   *int64
	IsLeaf     bool
	MinBalance money.Kop
	IsArchived bool
	IsSystem   bool
	CreatedAt  time.Time
}

// Transaction — заголовок проводки (многострочной операции).
type Transaction struct {
	ID          int64
	UUID        string
	OpType      OpType
	Date        time.Time
	Description string
	ActorID     int64
	ReversesID  *int64
	Source      string
	SourceHash  string
	CreatedAt   time.Time
	CommitID    int64
	Lines       []TransactionLine
}

// TransactionLine — строка проводки: счёт, положительная сумма и сторона,
// назначенная движком.
type TransactionLine struct {
	ID            int64
	TransactionID int64
	AccountID     int64
	Amount        money.Kop
	Side          Side
}

// User — член семьи, ведущий учёт.
type User struct {
	ID        int64
	Name      string
	TokenHash string
}

// ChangelogEntry — запись журнала изменений для синхронизации.
type ChangelogEntry struct {
	Seq       int64
	Entity    string
	EntityID  int64
	Op        string
	ChangedAt time.Time
}

// Сущности журнала изменений.
const (
	EntityAccount     = "accounts"
	EntityTransaction = "transactions"
)

// Операции журнала изменений.
const (
	OpInsert  = "insert"
	OpUpdate  = "update"
	OpArchive = "archive"
)
