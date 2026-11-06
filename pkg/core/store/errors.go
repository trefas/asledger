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
	// CodeShoppingItemNotFound — позиция не найдена или не принадлежит списку.
	CodeShoppingItemNotFound = "ERR_SHOPPING_ITEM_NOT_FOUND"
	// CodeShoppingItemNotConfirmed — позиция не подтверждена к покупке.
	CodeShoppingItemNotConfirmed = "ERR_SHOPPING_ITEM_NOT_CONFIRMED"
	// CodeCreditAccountInvalid — счёт не является листовым пассивом.
	CodeCreditAccountInvalid = "ERR_CREDIT_ACCOUNT_INVALID"
	// CodeCreditSettingsMissing — у счёта нет настроек кредита.
	CodeCreditSettingsMissing = "ERR_CREDIT_SETTINGS_MISSING"
	// CodeCreditRate — ставка кредита вне диапазона 0…1 000 000 ppm.
	CodeCreditRate = "ERR_CREDIT_RATE"
	// CodeCreditInterestType — недопустимый способ расчёта процентов.
	CodeCreditInterestType = "ERR_CREDIT_INTEREST_TYPE"
	// CodeCreditRepaymentOrder — недопустимый порядок погашения.
	CodeCreditRepaymentOrder = "ERR_CREDIT_REPAYMENT_ORDER"
	// CodeCreditGraceDate — беспроцентный период включён без даты окончания.
	CodeCreditGraceDate = "ERR_CREDIT_GRACE_DATE"
	// CodeDepositAccountInvalid — счёт не является листовым активом.
	CodeDepositAccountInvalid = "ERR_DEPOSIT_ACCOUNT_INVALID"
	// CodeDepositRate — ставка вклада вне диапазона 0…1 000 000 ppm.
	CodeDepositRate = "ERR_DEPOSIT_RATE"
	// CodeDepositMinBalance — отрицательный минимальный остаток.
	CodeDepositMinBalance = "ERR_DEPOSIT_MIN_BALANCE"
	// CodeDepositBonus — недопустимый вид или параметры бонуса.
	CodeDepositBonus = "ERR_DEPOSIT_BONUS"
	// CodeDepositFrequency — недопустимая частота начисления.
	CodeDepositFrequency = "ERR_DEPOSIT_FREQUENCY"
	// CodeDepositDates — дата закрытия раньше даты открытия.
	CodeDepositDates = "ERR_DEPOSIT_DATES"
	// CodeDepositSteps — пересекающиеся или некорректные ступени.
	CodeDepositSteps = "ERR_DEPOSIT_STEPS"
	// CodeCreditAmount — сумма погашения не положительна.
	CodeCreditAmount = "ERR_CREDIT_AMOUNT"
	// CodeCreditSourceInvalid — счёт-источник не является листовым активом.
	CodeCreditSourceInvalid = "ERR_CREDIT_SOURCE_INVALID"
	// CodeCreditInterestAccountInvalid — статья процентов не является листовым расходом.
	CodeCreditInterestAccountInvalid = "ERR_CREDIT_INTEREST_ACCOUNT_INVALID"
	// CodeCreditNothingToPay — погашать нечего: долг и проценты равны нулю.
	CodeCreditNothingToPay = "ERR_CREDIT_NOTHING_TO_PAY"
	// CodeCreditDate — не задана дата погашения.
	CodeCreditDate = "ERR_CREDIT_DATE"
	// CodeInterestUUID — не задан uuid начисления.
	CodeInterestUUID = "ERR_INTEREST_UUID"
	// CodeInterestAccount — не задан счёт начисления.
	CodeInterestAccount = "ERR_INTEREST_ACCOUNT"
	// CodeInterestDate — не задана дата начисления.
	CodeInterestDate = "ERR_INTEREST_DATE"
	// CodeInterestKind — недопустимый вид начисления.
	CodeInterestKind = "ERR_INTEREST_KIND"
	// CodeInterestAmount — сумма начисления не положительна.
	CodeInterestAmount = "ERR_INTEREST_AMOUNT"
	// CodeInterestRate — ставка начисления вне диапазона.
	CodeInterestRate = "ERR_INTEREST_RATE"
	// CodeInterestStatus — недопустимый статус начисления.
	CodeInterestStatus = "ERR_INTEREST_STATUS"
	// CodeInterestPeriod — некорректный период начисления.
	CodeInterestPeriod = "ERR_INTEREST_PERIOD"
	// CodeInterestAccountInvalid — счёт не подходит для начисления.
	CodeInterestAccountInvalid = "ERR_INTEREST_ACCOUNT_INVALID"
	// CodeInterestSettingsMissing — у счёта нет настроек кредита или вклада.
	CodeInterestSettingsMissing = "ERR_INTEREST_SETTINGS_MISSING"
	// CodeInterestIncomeAccount — статья доходов не листовая или не «доход».
	CodeInterestIncomeAccount = "ERR_INTEREST_INCOME_ACCOUNT"
	// CodeInterestExpenseAccount — статья расходов не листовая или не «расход».
	CodeInterestExpenseAccount = "ERR_INTEREST_EXPENSE_ACCOUNT"
	// CodeInterestNothing — за период нечего начислять.
	CodeInterestNothing = "ERR_INTEREST_NOTHING"
	// CodeDepositAmount — сумма снятия не положительна.
	CodeDepositAmount = "ERR_DEPOSIT_AMOUNT"
	// CodeDepositDestinationInvalid — счёт-получатель не является листовым активом.
	CodeDepositDestinationInvalid = "ERR_DEPOSIT_DESTINATION_INVALID"
	// CodeDepositIncomeInvalid — статья доходов не является листовым доходом.
	CodeDepositIncomeInvalid = "ERR_DEPOSIT_INCOME_INVALID"
	// CodeDepositNothingToWithdraw — снимать нечего: остаток и проценты равны нулю.
	CodeDepositNothingToWithdraw = "ERR_DEPOSIT_NOTHING_TO_WITHDRAW"
	// CodeDepositDate — не задана дата снятия.
	CodeDepositDate = "ERR_DEPOSIT_DATE"
	// CodeDepositSettingsMissing — у счёта нет настроек вклада.
	CodeDepositSettingsMissing = "ERR_DEPOSIT_SETTINGS_MISSING"
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
