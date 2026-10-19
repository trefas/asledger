package engine

import (
	"errors"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// TestASL_6_NatureOfAccounts проверяет таблицу базовой природы счетов построчно.
func TestASL_6_NatureOfAccounts(t *testing.T) {
	tests := []struct {
		name     string
		typ      model.AccountType
		increase model.Side
		decrease model.Side
	}{
		{"актив", model.TypeAsset, model.SideDebit, model.SideCredit},
		{"расход", model.TypeExpense, model.SideDebit, model.SideCredit},
		{"пассив", model.TypeLiability, model.SideCredit, model.SideDebit},
		{"капитал", model.TypeCapital, model.SideCredit, model.SideDebit},
		{"доход", model.TypeIncome, model.SideCredit, model.SideDebit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inc, err := IncreaseSide(tc.typ)
			if err != nil {
				t.Fatalf("IncreaseSide(%s): %v", tc.typ, err)
			}
			if inc != tc.increase {
				t.Fatalf("IncreaseSide(%s) = %s, ожидалось %s", tc.typ, inc, tc.increase)
			}
			dec, err := DecreaseSide(tc.typ)
			if err != nil {
				t.Fatalf("DecreaseSide(%s): %v", tc.typ, err)
			}
			if dec != tc.decrease {
				t.Fatalf("DecreaseSide(%s) = %s, ожидалось %s", tc.typ, dec, tc.decrease)
			}
			if inc.Opposite() != dec {
				t.Fatalf("для %s увеличение и уменьшение не противоположны", tc.typ)
			}
		})
	}
}

// TestASL_6_NatureUnknownType проверяет, что неизвестный тип не паникует.
func TestASL_6_NatureUnknownType(t *testing.T) {
	for _, fn := range []func(model.AccountType) (model.Side, error){IncreaseSide, DecreaseSide} {
		side, err := fn(model.AccountType("вымысел"))
		if err == nil {
			t.Fatal("неизвестный тип счёта не дал ошибку")
		}
		if side != "" {
			t.Fatalf("при ошибке вернулась сторона %q", side)
		}
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeAccountTypeUnknown {
			t.Fatalf("код ошибки = %q, ожидался %s", codeOf(err), CodeAccountTypeUnknown)
		}
	}
}
