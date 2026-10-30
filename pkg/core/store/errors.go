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
	// ErrAuthFailed — пользователь есть, но секрет не совпал.
	ErrAuthFailed = errors.New("store: неверный секрет пользователя")
	// ErrShoppingVersionConflict — позиция списка изменена другим клиентом.
	ErrShoppingVersionConflict = errors.New("store: конфликт версии позиции списка покупок")
	// ErrShoppingListArchived — список покупок в архиве и недоступен для правок.
	ErrShoppingListArchived = errors.New("store: список покупок заархивирован")
	// ErrShoppingItemPurchased — купленную позицию нельзя удалить.
	ErrShoppingItemPurchased = errors.New("store: купленная позиция защищена от удаления")
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
	// CodeAccountNotLeaf — счёт проводки не листовой (выбрана папка).
	CodeAccountNotLeaf = "ERR_ACCOUNT_NOT_LEAF"
	// CodeAccountArchived — счёт проводки заархивирован.
	CodeAccountArchived = "ERR_ACCOUNT_ARCHIVED"
	// CodeAccountNotFound — счёт проводки не существует.
	CodeAccountNotFound = "ERR_ACCOUNT_NOT_FOUND"
	// CodeAccountTypeMismatch — тип счёта не совпадает с типом родителя.
	CodeAccountTypeMismatch = "ERR_ACCOUNT_TYPE_MISMATCH"
	// CodeShoppingName — пустое или слишком длинное наименование.
	CodeShoppingName = "ERR_SHOPPING_NAME"
	// CodeShoppingStatus — недопустимый статус списка или позиции.
	CodeShoppingStatus = "ERR_SHOPPING_STATUS"
	// CodeShoppingPriority — недопустимый приоритет позиции.
	CodeShoppingPriority = "ERR_SHOPPING_PRIORITY"
	// CodeShoppingAccountInvalid — статья расходов не листовая или не «расход».
	CodeShoppingAccountInvalid = "ERR_SHOPPING_ACCOUNT_INVALID"
	// CodeShoppingAssigneeInvalid — ответственный не найден.
	CodeShoppingAssigneeInvalid = "ERR_SHOPPING_ASSIGNEE_INVALID"
	// CodeShoppingPriceInvalid — отрицательная цена позиции.
	CodeShoppingPriceInvalid = "ERR_SHOPPING_PRICE_INVALID"
	// CodeShoppingVersionConflict — устаревшая версия позиции.
	CodeShoppingVersionConflict = "ERR_SHOPPING_VERSION_CONFLICT"
	// CodeShoppingListArchived — список заархивирован.
	CodeShoppingListArchived = "ERR_SHOPPING_LIST_ARCHIVED"
	// CodeShoppingItemPurchased — купленную позицию нельзя удалить.
	CodeShoppingItemPurchased = "ERR_SHOPPING_ITEM_PURCHASED"
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
