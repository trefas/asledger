package engine

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// TestASL_6_ErrorFormat проверяет структурированную ошибку движка: код,
// сообщение и контекст.
func TestASL_6_ErrorFormat(t *testing.T) {
	err := NewError(CodeUnbalanced, "дебет не равен кредиту").
		WithAccount("Активы:Наличные").
		WithAmount(money.FromKopecks(1000)).
		WithCell("трата/DEBIT/расход")

	var e *Error
	if !errors.As(err, &e) {
		t.Fatal("ошибка не разбирается как *engine.Error")
	}
	if e.Code != CodeUnbalanced {
		t.Fatalf("код = %q, ожидался %s", e.Code, CodeUnbalanced)
	}
	if e.Account != "Активы:Наличные" || e.Amount != money.FromKopecks(1000) || e.Cell == "" {
		t.Fatalf("контекст ошибки неполон: %+v", e)
	}
	if codeOf(err) != CodeUnbalanced {
		t.Fatalf("CodeOf = %q", codeOf(err))
	}
	text := err.Error()
	for _, want := range []string{CodeUnbalanced, "Активы:Наличные", "трата/DEBIT/расход"} {
		if !contains(text, want) {
			t.Fatalf("в тексте ошибки %q нет %q", text, want)
		}
	}
}

// TestASL_6_CodeOfForeign проверяет, что CodeOf не паникует на чужих ошибках.
func TestASL_6_CodeOfForeign(t *testing.T) {
	if codeOf(errors.New("чужая ошибка")) != "" {
		t.Fatal("CodeOf вернул код для чужой ошибки")
	}
	if codeOf(nil) != "" {
		t.Fatal("CodeOf(nil) должен быть пустым")
	}
}

// contains проверяет вхождение подстроки.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestASL_6_CodesDocumented проверяет, что каждый код движка описан в одном
// месте — docs/validation-codes.md — и не повторяется.
func TestASL_6_CodesDocumented(t *testing.T) {
	data, err := os.ReadFile("../../../docs/validation-codes.md")
	if err != nil {
		t.Fatalf("не удалось прочитать документацию кодов: %v", err)
	}
	doc := string(data)

	seen := map[string]bool{}
	for _, code := range Codes() {
		if code == "" {
			t.Fatal("пустой код в перечне")
		}
		if seen[code] {
			t.Fatalf("код %s продублирован", code)
		}
		seen[code] = true
		if !contains(doc, code) {
			t.Fatalf("код %s не описан в docs/validation-codes.md", code)
		}
	}
}

// TestASL_6_CodeCoverage проверяет, что каждый код достижим из движка.
func TestASL_6_CodeCoverage(t *testing.T) {
	kop := money.FromKopecks

	t.Run("неизвестный тип операции", func(t *testing.T) {
		f := newFixture(t)
		op := f.expenseOp("cov-op", kop(1000))
		op.OpType = model.OpType("вымысел")
		if _, err := f.post(t, op); codeOf(err) != CodeOpTypeUnknown {
			t.Fatalf("код = %q", codeOf(err))
		}
	})

	t.Run("неизвестная роль", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpExpense, UUID: "cov-role", Date: opDate(),
			Entries: []Entry{{AccountID: f.expense.ID, Amount: kop(1000), Role: Role("вымысел")}},
		})
		if codeOf(err) != CodeRoleUnknown {
			t.Fatalf("код = %q", codeOf(err))
		}
	})

	t.Run("нет строк", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{OpType: model.OpExpense, UUID: "cov-empty", Date: opDate()}); codeOf(err) != CodeEntryMissing {
			t.Fatalf("код = %q", codeOf(err))
		}
	})

	t.Run("счёт не найден", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpExpense, UUID: "cov-missing", Date: opDate(),
			Entries: []Entry{{AccountID: 999999, Amount: kop(1000), Role: RoleExpense}},
		})
		if codeOf(err) != CodeAccountNotFound {
			t.Fatalf("код = %q", codeOf(err))
		}
	})

	t.Run("дубликат uuid слоя данных", func(t *testing.T) {
		err := mapStoreError(fmt.Errorf("%w: dup", store.ErrDuplicateUUID))
		if codeOf(err) != CodeUUIDDup {
			t.Fatalf("код = %q", codeOf(err))
		}
	})
}
