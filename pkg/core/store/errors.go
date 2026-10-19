package store

import "errors"

// Ошибки слоя данных.
var (
	// ErrNotFound — запись не найдена.
	ErrNotFound = errors.New("store: запись не найдена")
	// ErrDuplicatePath — счёт с таким полным путём уже существует.
	ErrDuplicatePath = errors.New("store: счёт с таким путём уже существует")
	// ErrSystemAccount — служебный счёт нельзя изменить или заархивировать.
	ErrSystemAccount = errors.New("store: служебный счёт защищён от изменений")
	// ErrAccountHasChildren — у счёта есть потомки.
	ErrAccountHasChildren = errors.New("store: у счёта есть потомки")
	// ErrAccountHasOperations — по счёту есть проводки.
	ErrAccountHasOperations = errors.New("store: по счёту есть проводки")
	// ErrAccountArchived — счёт заархивирован.
	ErrAccountArchived = errors.New("store: счёт заархивирован")
	// ErrMoveIntoDescendant — попытка переместить счёт в собственного потомка.
	ErrMoveIntoDescendant = errors.New("store: нельзя переместить счёт в своего потомка")
	// ErrDuplicateUUID — транзакция с таким uuid уже существует.
	ErrDuplicateUUID = errors.New("store: транзакция с таким uuid уже существует")
)

// Коды нарушений инвариантов записи. Совпадают с кодами валидации движка,
// которые используются в API и web-интерфейсе.
const (
	// CodeUnbalanced — сумма дебета не равна сумме кредита.
	CodeUnbalanced = "ERR_UNBALANCED"
	// CodeAmountNotPositive — сумма строки не положительна.
	CodeAmountNotPositive = "ERR_AMOUNT_NOT_POSITIVE"
	// CodeSideInvalid — недопустимая сторона проводки.
	CodeSideInvalid = "ERR_SIDE_INVALID"
	// CodeNoLines — в проводке нет строк.
	CodeNoLines = "ERR_NO_LINES"
)

// ValidationError описывает нарушение инварианта сохранения проводки.
type ValidationError struct {
	// Code — машинный код нарушения.
	Code string
	// Message — человекочитаемое описание на русском языке.
	Message string
}

// Error реализует интерфейс error.
func (e *ValidationError) Error() string { return e.Code + ": " + e.Message }
