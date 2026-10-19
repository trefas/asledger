package engine

import (
	"context"
	"testing"

	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_6_ReverseExpense проверяет сторно как инверсию проводки.
func TestASL_6_ReverseExpense(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	original, err := f.post(t, f.expenseOp("rev-orig-1", money.FromKopecks(10000)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	reversal, err := f.engine.Reverse(ctx, original.ID, f.actor, "rev-1", opDate(), "отмена")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if reversal.ReversesID == nil || *reversal.ReversesID != original.ID {
		t.Fatalf("reverses_id = %v, ожидался %d", reversal.ReversesID, original.ID)
	}
	for _, line := range reversal.Lines {
		for _, src := range original.Lines {
			if line.AccountID == src.AccountID && line.Side != src.Side.Opposite() {
				t.Fatalf("счёт %d: сторона %s, ожидалась инверсия %s",
					line.AccountID, line.Side, src.Side.Opposite())
			}
		}
	}
	if got := f.balance(t, f.wallet); got != 0 {
		t.Fatalf("кошелёк = %s, ожидалось 0 после сторно", got)
	}
	if got := f.balance(t, f.expense); got != 0 {
		t.Fatalf("статья расхода = %s, ожидалось 0 после сторно", got)
	}
}

// TestASL_6_ReverseIdempotent проверяет, что повторная отмена идемпотентна.
func TestASL_6_ReverseIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	original, err := f.post(t, f.expenseOp("rev-orig-2", money.FromKopecks(10000)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	first, err := f.engine.Reverse(ctx, original.ID, f.actor, "rev-2a", opDate(), "отмена")
	if err != nil {
		t.Fatalf("первый Reverse: %v", err)
	}
	second, err := f.engine.Reverse(ctx, original.ID, f.actor, "rev-2b", opDate(), "отмена повторно")
	if err != nil {
		t.Fatalf("повторный Reverse: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("повторная отмена дала проводку %d вместо %d", second.ID, first.ID)
	}
}

// TestASL_6_ReverseOfReverse запрещает цепочки «сторно сторно».
func TestASL_6_ReverseOfReverse(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	original, err := f.post(t, f.expenseOp("rev-orig-3", money.FromKopecks(10000)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	reversal, err := f.engine.Reverse(ctx, original.ID, f.actor, "rev-3", opDate(), "отмена")
	if err != nil {
		t.Fatalf("Reverse: %v", err)
	}
	if _, err := f.engine.Reverse(ctx, reversal.ID, f.actor, "rev-3b", opDate(), "отмена отмены"); codeOf(err) != CodeReverseInvalid {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeReverseInvalid, err)
	}
}

// TestASL_6_ReverseMissing запрещает сторно несуществующей записи.
func TestASL_6_ReverseMissing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.engine.Reverse(ctx, 9999, f.actor, "rev-4", opDate(), "отмена"); codeOf(err) != CodeReverseInvalid {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeReverseInvalid, err)
	}
}

// TestASL_6_ReverseUUIDRequired проверяет требование uuid сторно.
func TestASL_6_ReverseUUIDRequired(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	original, err := f.post(t, f.expenseOp("rev-orig-5", money.FromKopecks(10000)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if _, err := f.engine.Reverse(ctx, original.ID, f.actor, "", opDate(), "отмена"); codeOf(err) != CodeUUIDEmpty {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeUUIDEmpty, err)
	}
}
