package engine

import (
	"errors"
	"testing"

	"github.com/trefas/asledger/pkg/core/money"
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
