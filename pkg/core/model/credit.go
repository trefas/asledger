package model

import (
	"time"

	"github.com/trefas/asledger/pkg/core/money"
)

// ProductType — тип банковского продукта, привязанного к счёту.
//
// Тип счёта при этом не меняется: кредит остаётся пассивом, вклад — активом,
// чтобы матрица проводок и Android-клиент работали без изменений.
type ProductType string

// Допустимые типы продуктов.
const (
	// ProductCredit — кредит или заём на счёте типа «пассив».
	ProductCredit ProductType = "credit"
	// ProductDeposit — вклад или накопительный счёт типа «актив».
	ProductDeposit ProductType = "deposit"
)

// ProductForAccountType возвращает продукт для типа счёта.
//
// Пассив соответствует кредиту, актив — вкладу. Для остальных типов продукта
// нет: настройки кредита и вклада к ним не применяются.
func ProductForAccountType(t AccountType) (ProductType, bool) {
	switch t {
	case TypeLiability:
		return ProductCredit, true
	case TypeAsset:
		return ProductDeposit, true
	default:
		return "", false
	}
}

// CreditInterestType — способ расчёта процентов по кредиту.
type CreditInterestType string

// Допустимые способы расчёта процентов по кредиту.
const (
	// CreditInterestSimple — проценты по простой ставке на остаток долга.
	CreditInterestSimple CreditInterestType = "simple"
	// CreditInterestOnBalance — проценты на остаток долга на дату операции.
	CreditInterestOnBalance CreditInterestType = "on_balance"
)

// CreditInterestTypes возвращает все допустимые способы расчёта.
func CreditInterestTypes() []CreditInterestType {
	return []CreditInterestType{CreditInterestSimple, CreditInterestOnBalance}
}

// Valid сообщает, является ли способ расчёта допустимым.
func (t CreditInterestType) Valid() bool {
	switch t {
	case CreditInterestSimple, CreditInterestOnBalance:
		return true
	default:
		return false
	}
}

// RepaymentOrder — порядок распределения платежа по кредиту.
type RepaymentOrder string

// Допустимые порядки погашения.
const (
	// RepaymentInterestFirst — сначала проценты, затем тело долга.
	RepaymentInterestFirst RepaymentOrder = "interest_first"
	// RepaymentPrincipalFirst — сначала тело долга, затем проценты.
	RepaymentPrincipalFirst RepaymentOrder = "principal_first"
)

// RepaymentOrders возвращает все допустимые порядки погашения.
func RepaymentOrders() []RepaymentOrder {
	return []RepaymentOrder{RepaymentInterestFirst, RepaymentPrincipalFirst}
}

// Valid сообщает, является ли порядок погашения допустимым.
func (o RepaymentOrder) Valid() bool {
	switch o {
	case RepaymentInterestFirst, RepaymentPrincipalFirst:
		return true
	default:
		return false
	}
}

// DepositBonusType — вид бонуса по вкладу.
type DepositBonusType string

// Допустимые виды бонуса.
const (
	// DepositBonusNone — бонус не начисляется.
	DepositBonusNone DepositBonusType = "none"
	// DepositBonusFixed — фиксированная сумма за период.
	DepositBonusFixed DepositBonusType = "fixed"
	// DepositBonusRate — процент сверх базовой ставки.
	DepositBonusRate DepositBonusType = "rate"
)

// DepositBonusTypes возвращает все допустимые виды бонуса.
func DepositBonusTypes() []DepositBonusType {
	return []DepositBonusType{DepositBonusNone, DepositBonusFixed, DepositBonusRate}
}

// Valid сообщает, является ли вид бонуса допустимым.
func (t DepositBonusType) Valid() bool {
	switch t {
	case DepositBonusNone, DepositBonusFixed, DepositBonusRate:
		return true
	default:
		return false
	}
}

// DepositAccrualFrequency — частота начисления процентов по вкладу.
type DepositAccrualFrequency string

// Допустимые частоты начисления.
const (
	// DepositAccrualDaily — начисление на фактический остаток каждый день.
	DepositAccrualDaily DepositAccrualFrequency = "daily"
	// DepositAccrualMonthly — начисление на минимальный остаток за месяц.
	DepositAccrualMonthly DepositAccrualFrequency = "monthly"
)

// DepositAccrualFrequencies возвращает все допустимые частоты.
func DepositAccrualFrequencies() []DepositAccrualFrequency {
	return []DepositAccrualFrequency{DepositAccrualDaily, DepositAccrualMonthly}
}

// Valid сообщает, является ли частота начисления допустимой.
func (f DepositAccrualFrequency) Valid() bool {
	switch f {
	case DepositAccrualDaily, DepositAccrualMonthly:
		return true
	default:
		return false
	}
}

// InterestAccrualKind — вид начисления процентов или бонуса.
type InterestAccrualKind string

// Допустимые виды начислений.
const (
	// InterestAccrualCredit — проценты по кредиту.
	InterestAccrualCredit InterestAccrualKind = "credit_interest"
	// InterestAccrualDeposit — проценты по вкладу.
	InterestAccrualDeposit InterestAccrualKind = "deposit_interest"
	// InterestAccrualBonus — бонус по вкладу.
	InterestAccrualBonus InterestAccrualKind = "deposit_bonus"
)

// InterestAccrualKinds возвращает все допустимые виды начислений.
func InterestAccrualKinds() []InterestAccrualKind {
	return []InterestAccrualKind{
		InterestAccrualCredit, InterestAccrualDeposit, InterestAccrualBonus,
	}
}

// Valid сообщает, является ли вид начисления допустимым.
func (k InterestAccrualKind) Valid() bool {
	switch k {
	case InterestAccrualCredit, InterestAccrualDeposit, InterestAccrualBonus:
		return true
	default:
		return false
	}
}

// InterestAccrualStatus — статус начисления.
type InterestAccrualStatus string

// Допустимые статусы начисления.
const (
	// InterestAccrued — начисление рассчитано, но ещё не проведено.
	InterestAccrued InterestAccrualStatus = "accrued"
	// InterestPaid — проценты выплачены.
	InterestPaid InterestAccrualStatus = "paid"
	// InterestCapitalized — проценты капитализированы (добавлены к вкладу).
	InterestCapitalized InterestAccrualStatus = "capitalized"
)

// InterestAccrualStatuses возвращает все допустимые статусы начислений.
func InterestAccrualStatuses() []InterestAccrualStatus {
	return []InterestAccrualStatus{InterestAccrued, InterestPaid, InterestCapitalized}
}

// Valid сообщает, является ли статус начисления допустимым.
func (s InterestAccrualStatus) Valid() bool {
	switch s {
	case InterestAccrued, InterestPaid, InterestCapitalized:
		return true
	default:
		return false
	}
}

// CreditSettings — настройки кредитного счёта.
//
// Ставка задаётся в ppm, суммы — в копейках; валюта учёта — рубль.
type CreditSettings struct {
	AccountID      int64              `json:"account_id"`
	RatePPM        int64              `json:"rate_ppm"`
	InterestType   CreditInterestType `json:"interest_type"`
	RepaymentOrder RepaymentOrder     `json:"repayment_order"`
	GraceEnabled   bool               `json:"grace_enabled"`
	// GraceEndDate — конец беспроцентного периода; нулевое время — не задан.
	GraceEndDate time.Time `json:"grace_end_date,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DepositSettings — настройки вклада или накопительного счёта.
//
// Ставки задаются в ppm, суммы — в копейках; валюта учёта — рубль.
type DepositSettings struct {
	AccountID        int64                   `json:"account_id"`
	BaseRatePPM      int64                   `json:"base_rate_ppm"`
	MinBalance       money.Kop               `json:"min_balance_kop"`
	BonusType        DepositBonusType        `json:"bonus_type"`
	BonusRatePPM     int64                   `json:"bonus_rate_ppm"`
	BonusAmount      money.Kop               `json:"bonus_amount_kop"`
	AccrualFrequency DepositAccrualFrequency `json:"accrual_frequency"`
	Capitalization   bool                    `json:"capitalization"`
	// OpenedAt и ClosedAt — даты открытия и закрытия; нулевое время — не задана.
	OpenedAt  time.Time `json:"opened_at,omitempty"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Steps — ступенчатые ставки; пусто, если используется базовая ставка.
	Steps []DepositStepRate `json:"steps,omitempty"`
}

// DepositStepRate — ступенчатая ставка вклада.
//
// To == nil означает «до бесконечности». Диапазоны не пересекаются.
type DepositStepRate struct {
	AccountID int64      `json:"account_id"`
	From      money.Kop  `json:"from_kop"`
	To        *money.Kop `json:"to_kop,omitempty"`
	RatePPM   int64      `json:"rate_ppm"`
}

// InterestAccrual — начисление процентов или бонуса по кредиту/вкладу.
type InterestAccrual struct {
	ID            int64                 `json:"id"`
	UUID          string                `json:"uuid"`
	AccountID     int64                 `json:"account_id"`
	Date          time.Time             `json:"date"`
	Kind          InterestAccrualKind   `json:"kind"`
	Amount        money.Kop             `json:"amount_kop"`
	BaseAmount    money.Kop             `json:"base_amount_kop"`
	RatePPM       int64                 `json:"rate_ppm"`
	PeriodFrom    time.Time             `json:"period_from,omitempty"`
	PeriodTo      time.Time             `json:"period_to,omitempty"`
	Status        InterestAccrualStatus `json:"status"`
	TransactionID *int64                `json:"transaction_id,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// Сущности журнала изменений для кредитов и вкладов.
const (
	// EntityCreditSettings — настройки кредитного счёта.
	EntityCreditSettings = "credit_settings"
	// EntityDepositSettings — настройки вклада.
	EntityDepositSettings = "deposit_settings"
	// EntityInterestAccrual — начисление процентов или бонуса.
	EntityInterestAccrual = "interest_accruals"
)
