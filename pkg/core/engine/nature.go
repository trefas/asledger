package engine

import "github.com/trefas/asledger/pkg/core/model"

// Базовая природа счетов (раздел 6 эпика). Таблица кодируется один раз, а
// матрица операций складывается из неё автоматически.
//
//	актив, расход          → увеличиваются по DEBIT
//	пассив, капитал, доход → увеличиваются по CREDIT

// IncreaseSide возвращает сторону, по которой счёт увеличивается.
//
// Неизвестный тип счёта даёт ошибку ERR_ACCOUNT_TYPE_UNKNOWN (без паники).
func IncreaseSide(t model.AccountType) (model.Side, error) {
	switch t {
	case model.TypeAsset, model.TypeExpense:
		return model.SideDebit, nil
	case model.TypeLiability, model.TypeCapital, model.TypeIncome:
		return model.SideCredit, nil
	default:
		return "", NewError(CodeAccountTypeUnknown, "неизвестный тип счёта «"+string(t)+"»")
	}
}

// DecreaseSide возвращает сторону, по которой счёт уменьшается.
//
// Неизвестный тип счёта даёт ошибку ERR_ACCOUNT_TYPE_UNKNOWN (без паники).
func DecreaseSide(t model.AccountType) (model.Side, error) {
	side, err := IncreaseSide(t)
	if err != nil {
		return "", err
	}
	return side.Opposite(), nil
}
