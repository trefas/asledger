package model

import "testing"

// TestASL_16_ProductForAccountType проверяет соответствие продукта типу счёта.
func TestASL_16_ProductForAccountType(t *testing.T) {
	cases := []struct {
		name    string
		account AccountType
		want    ProductType
		ok      bool
	}{
		{"пассив — кредит", TypeLiability, ProductCredit, true},
		{"актив — вклад", TypeAsset, ProductDeposit, true},
		{"капитал", TypeCapital, "", false},
		{"доход", TypeIncome, "", false},
		{"расход", TypeExpense, "", false},
		{"неизвестный", AccountType("счёт"), "", false},
	}
	for _, tc := range cases {
		got, ok := ProductForAccountType(tc.account)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("%s: ProductForAccountType(%q) = (%q, %v), ожидалось (%q, %v)",
				tc.name, tc.account, got, ok, tc.want, tc.ok)
		}
	}
}

// TestASL_16_CreditEnums проверяет допустимые значения перечислений кредита.
func TestASL_16_CreditEnums(t *testing.T) {
	for _, interestType := range CreditInterestTypes() {
		if !interestType.Valid() {
			t.Fatalf("способ расчёта %q должен быть допустим", interestType)
		}
	}
	if CreditInterestType("annuity").Valid() {
		t.Fatal("неизвестный способ расчёта признан допустимым")
	}

	for _, order := range RepaymentOrders() {
		if !order.Valid() {
			t.Fatalf("порядок погашения %q должен быть допустим", order)
		}
	}
	if RepaymentOrder("body_first").Valid() {
		t.Fatal("неизвестный порядок погашения признан допустимым")
	}
}

// TestASL_16_DepositEnums проверяет допустимые значения перечислений вклада.
func TestASL_16_DepositEnums(t *testing.T) {
	for _, bonus := range DepositBonusTypes() {
		if !bonus.Valid() {
			t.Fatalf("вид бонуса %q должен быть допустим", bonus)
		}
	}
	if DepositBonusType("cashback").Valid() {
		t.Fatal("неизвестный вид бонуса признан допустимым")
	}

	for _, frequency := range DepositAccrualFrequencies() {
		if !frequency.Valid() {
			t.Fatalf("частота начисления %q должна быть допустима", frequency)
		}
	}
	if DepositAccrualFrequency("weekly").Valid() {
		t.Fatal("неизвестная частота начисления признана допустимой")
	}
}

// TestASL_16_InterestAccrualEnums проверяет виды и статусы начислений.
func TestASL_16_InterestAccrualEnums(t *testing.T) {
	for _, kind := range InterestAccrualKinds() {
		if !kind.Valid() {
			t.Fatalf("вид начисления %q должен быть допустим", kind)
		}
	}
	if InterestAccrualKind("fee").Valid() {
		t.Fatal("неизвестный вид начисления признан допустимым")
	}

	for _, status := range InterestAccrualStatuses() {
		if !status.Valid() {
			t.Fatalf("статус начисления %q должен быть допустим", status)
		}
	}
	if InterestAccrualStatus("cancelled").Valid() {
		t.Fatal("неизвестный статус начисления признан допустимым")
	}
}
