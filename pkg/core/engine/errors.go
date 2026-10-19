// Package engine реализует матрицу правил ASL-2.2 и движок проводок asledger.
//
// Пользователь передаёт тип операции, счета и положительные суммы; сторону
// каждой строки (DEBIT/CREDIT) назначает движок по матрице правил. Движок
// проверяет сквозные инварианты раздела 6 эпика и сохраняет проводку через
// слой данных (pkg/core/store).
package engine

import (
	"errors"

	"github.com/trefas/asledger/pkg/core/money"
)

// Машинные коды ошибок движка. Коды стабильны и используются web-интерфейсом,
// REST API и Android-клиентом для понятной реакции; полный список с описанием
// — в docs/validation-codes.md.
const (
	// CodeAccountNotLeaf — в строке указан нелистовой счёт (папка).
	CodeAccountNotLeaf = "ERR_ACCOUNT_NOT_LEAF"
	// CodeAccountArchived — счёт строки заархивирован.
	CodeAccountArchived = "ERR_ACCOUNT_ARCHIVED"
	// CodeAccountNotFound — счёт строки не существует.
	CodeAccountNotFound = "ERR_ACCOUNT_NOT_FOUND"
	// CodeMatrixCell — сочетание (операция, сторона, тип счёта) недопустимо.
	CodeMatrixCell = "ERR_MATRIX_CELL"
	// CodeReservedCell — ячейка зарезервирована под post-MVP и пока недоступна.
	CodeReservedCell = "ERR_RESERVED_CELL"
	// CodeUnbalanced — сумма дебета не равна сумме кредита.
	CodeUnbalanced = "ERR_UNBALANCED"
	// CodeMinBalance — проводка уводит остаток ниже min_balance в лимитном режиме.
	CodeMinBalance = "ERR_MIN_BALANCE"
	// CodeUUIDDup — проводка с таким uuid уже есть.
	CodeUUIDDup = "ERR_UUID_DUP"
	// CodeUUIDEmpty — не задан uuid проводки.
	CodeUUIDEmpty = "ERR_UUID_EMPTY"
	// CodeDateRange — дата вне допустимого диапазона.
	CodeDateRange = "ERR_DATE_RANGE"
	// CodeAmountNotPositive — сумма строки не положительна.
	CodeAmountNotPositive = "ERR_AMOUNT_NOT_POSITIVE"
	// CodeOpTypeUnknown — неизвестный тип операции.
	CodeOpTypeUnknown = "ERR_OP_TYPE_UNKNOWN"
	// CodeAccountTypeUnknown — неизвестный тип счёта.
	CodeAccountTypeUnknown = "ERR_ACCOUNT_TYPE_UNKNOWN"
	// CodeRoleUnknown — роль строки недопустима для операции.
	CodeRoleUnknown = "ERR_ROLE_UNKNOWN"
	// CodeInitialDone — начальные остатки уже введены.
	CodeInitialDone = "ERR_INITIAL_DONE"
	// CodeSystemAccount — служебную строку задаёт движок, вручную нельзя.
	CodeSystemAccount = "ERR_SYSTEM_ACCOUNT"
	// CodeReverseInvalid — отменяемая запись не подходит для сторно.
	CodeReverseInvalid = "ERR_REVERSE_INVALID"
	// CodeEntryMissing — не заданы обязательные строки операции.
	CodeEntryMissing = "ERR_ENTRY_MISSING"
	// CodeDirectionRequired — не задано направление переоценки/корректировки.
	CodeDirectionRequired = "ERR_DIRECTION_REQUIRED"
)

// Error — структурированная ошибка движка: машинный код, человекочитаемое
// сообщение на русском и контекст (счёт, сумма, ячейка матрицы).
type Error struct {
	// Code — машинный код из перечня выше.
	Code string
	// Message — понятное пользователю сообщение.
	Message string
	// Account — полный путь счёта, если ошибка связана со счётом.
	Account string
	// Amount — сумма строки, если ошибка связана с суммой.
	Amount money.Kop
	// Cell — ячейка матрицы «операция/сторона/тип счёта».
	Cell string
	// Err — исходная ошибка, если она есть.
	Err error
}

// Error реализует интерфейс error.
func (e *Error) Error() string {
	msg := e.Message
	if e.Account != "" {
		msg += " (счёт «" + e.Account + "»)"
	}
	if e.Cell != "" {
		msg += " (ячейка " + e.Cell + ")"
	}
	return e.Code + ": " + msg
}

// Unwrap возвращает исходную ошибку.
func (e *Error) Unwrap() error { return e.Err }

// NewError создаёт ошибку движка с кодом и сообщением.
func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// WithAccount дополняет ошибку путём счёта.
func (e *Error) WithAccount(path string) *Error {
	e.Account = path
	return e
}

// WithAmount дополняет ошибку суммой.
func (e *Error) WithAmount(amount money.Kop) *Error {
	e.Amount = amount
	return e
}

// WithCell дополняет ошибку ячейкой матрицы.
func (e *Error) WithCell(cell string) *Error {
	e.Cell = cell
	return e
}

// WithCause дополняет ошибку исходной причиной.
func (e *Error) WithCause(err error) *Error {
	e.Err = err
	return e
}

// CodeOf возвращает машинный код ошибки движка или пустую строку.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Codes возвращает полный перечень машинных кодов движка.
//
// Используется документацией и тестами: каждый код обязан быть описан в
// docs/validation-codes.md.
func Codes() []string {
	return []string{
		CodeAccountNotLeaf,
		CodeAccountArchived,
		CodeAccountNotFound,
		CodeMatrixCell,
		CodeReservedCell,
		CodeUnbalanced,
		CodeMinBalance,
		CodeUUIDDup,
		CodeUUIDEmpty,
		CodeDateRange,
		CodeAmountNotPositive,
		CodeOpTypeUnknown,
		CodeAccountTypeUnknown,
		CodeRoleUnknown,
		CodeInitialDone,
		CodeSystemAccount,
		CodeReverseInvalid,
		CodeEntryMissing,
		CodeDirectionRequired,
	}
}
