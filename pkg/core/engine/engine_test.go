package engine

import (
	"context"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// TestASL_6_EngineAssignsSides проверяет, что стороны строк назначает движок:
// у строки запроса нет стороны, а в сохранённой проводке стороны выставлены
// по матрице.
func TestASL_6_EngineAssignsSides(t *testing.T) {
	f := newFixture(t)
	tr, err := f.post(t, f.expenseOp("side-1", money.FromKopecks(10000)))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if len(tr.Lines) != 2 {
		t.Fatalf("строк %d, ожидалось 2", len(tr.Lines))
	}
	sides := map[int64]model.Side{}
	for _, line := range tr.Lines {
		sides[line.AccountID] = line.Side
	}
	if sides[f.expense.ID] != model.SideDebit {
		t.Fatalf("статья расхода: сторона %s, ожидался DEBIT", sides[f.expense.ID])
	}
	if sides[f.wallet.ID] != model.SideCredit {
		t.Fatalf("источник: сторона %s, ожидался CREDIT", sides[f.wallet.ID])
	}
}

// TestASL_6_EngineUnbalanced проверяет отлонение несбалансированной проводки.
func TestASL_6_EngineUnbalanced(t *testing.T) {
	f := newFixture(t)
	_, err := f.post(t, Operation{
		OpType: model.OpExpense,
		UUID:   "unbalanced-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
			{AccountID: f.wallet.ID, Amount: money.FromKopecks(9000), Role: RoleSource},
		},
	})
	if codeOf(err) != CodeUnbalanced {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeUnbalanced, err)
	}
}

// TestASL_6_EngineLeafOnly проверяет, что папка в строке отклоняется.
func TestASL_6_EngineLeafOnly(t *testing.T) {
	f := newFixture(t)
	folder := accountByPath(t, f.db, "Активы")
	_, err := f.post(t, Operation{
		OpType: model.OpExpense,
		UUID:   "leaf-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
			{AccountID: folder.ID, Amount: money.FromKopecks(10000), Role: RoleSource},
		},
	})
	if codeOf(err) != CodeAccountNotLeaf {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeAccountNotLeaf, err)
	}
}

// TestASL_6_EngineArchived проверяет, что архивированный счёт отклоняется.
func TestASL_6_EngineArchived(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	assets := accountByPath(t, f.db, "Активы")
	old := createTestAccount(t, f.db, "Старая карта", model.TypeAsset, &assets.ID)
	if err := f.db.ArchiveAccount(ctx, old.ID); err != nil {
		t.Fatalf("ArchiveAccount: %v", err)
	}
	_, err := f.post(t, Operation{
		OpType: model.OpExpense,
		UUID:   "archived-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
			{AccountID: old.ID, Amount: money.FromKopecks(10000), Role: RoleSource},
		},
	})
	if codeOf(err) != CodeAccountArchived {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeAccountArchived, err)
	}
}

// TestASL_6_EngineAmountNotPositive проверяет требование положительных сумм.
func TestASL_6_EngineAmountNotPositive(t *testing.T) {
	f := newFixture(t)
	_, err := f.post(t, Operation{
		OpType: model.OpExpense,
		UUID:   "amount-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: 0, Role: RoleExpense},
			{AccountID: f.wallet.ID, Amount: 0, Role: RoleSource},
		},
	})
	if codeOf(err) != CodeAmountNotPositive {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeAmountNotPositive, err)
	}
}

// TestASL_6_EngineUUIDRequired проверяет требование uuid.
func TestASL_6_EngineUUIDRequired(t *testing.T) {
	f := newFixture(t)
	op := f.expenseOp(" ", money.FromKopecks(10000))
	if _, err := f.post(t, op); codeOf(err) != CodeUUIDEmpty {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeUUIDEmpty, err)
	}
}

// TestASL_6_EngineDateRange проверяет диапазон даты: не раньше создания счёта
// и не больше «завтра».
func TestASL_6_EngineDateRange(t *testing.T) {
	t.Run("раньше создания счёта", func(t *testing.T) {
		f := newFixture(t)
		op := f.expenseOp("date-past", money.FromKopecks(10000))
		op.Date = time.Now().AddDate(0, 0, -3)
		if _, err := f.post(t, op); codeOf(err) != CodeDateRange {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeDateRange, err)
		}
	})
	t.Run("позже завтра", func(t *testing.T) {
		f := newFixture(t)
		op := f.expenseOp("date-future", money.FromKopecks(10000))
		op.Date = time.Now().AddDate(0, 0, 3)
		if _, err := f.post(t, op); codeOf(err) != CodeDateRange {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeDateRange, err)
		}
	})
}

// TestASL_6_EngineIdempotentUUID проверяет, что повтор с тем же uuid даёт одну
// запись и тот же ответ.
func TestASL_6_EngineIdempotentUUID(t *testing.T) {
	f := newFixture(t)
	op := f.expenseOp("idem-1", money.FromKopecks(10000))
	first, err := f.post(t, op)
	if err != nil {
		t.Fatalf("первый Post: %v", err)
	}
	second, err := f.post(t, op)
	if err != nil {
		t.Fatalf("повторный Post: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("повтор дал проводку %d вместо %d", second.ID, first.ID)
	}
	list, err := f.db.ListTransactions(context.Background(), store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("проводок %d, ожидалась 1", len(list))
	}
}

// TestASL_6_EngineMatrixCell проверяет отклонение недопустимой ячейки.
func TestASL_6_EngineMatrixCell(t *testing.T) {
	f := newFixture(t)
	_, err := f.post(t, Operation{
		OpType: model.OpExpense,
		UUID:   "cell-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
			{AccountID: f.income.ID, Amount: money.FromKopecks(10000), Role: RoleSource},
		},
	})
	if codeOf(err) != CodeMatrixCell {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
	}
}

// TestASL_6_EngineReservedCell проверяет пометку «оплата в кредит» как post-MVP.
func TestASL_6_EngineReservedCell(t *testing.T) {
	f := newFixture(t)
	_, err := f.post(t, Operation{
		OpType: model.OpExpense,
		UUID:   "reserved-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
			{AccountID: f.credit.ID, Amount: money.FromKopecks(10000), Role: RoleSource},
		},
	})
	if codeOf(err) != CodeReservedCell {
		t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeReservedCell, err)
	}
}

// TestASL_6_EngineMinBalance проверяет режим контроля остатка.
func TestASL_6_EngineMinBalance(t *testing.T) {
	ctx := context.Background()
	setMode := func(t *testing.T, f *fixture, mode string) {
		t.Helper()
		if err := f.db.SetSetting(ctx, store.SettingBalanceMode, mode); err != nil {
			t.Fatalf("SetSetting: %v", err)
		}
	}

	t.Run("лимитный режим отклоняет уход ниже минимума", func(t *testing.T) {
		f := newFixture(t)
		if err := f.db.SetMinBalance(ctx, f.card.ID, money.FromKopecks(-10000)); err != nil {
			t.Fatalf("SetMinBalance: %v", err)
		}
		setMode(t, f, store.BalanceModeLimited)
		_, err := f.post(t, Operation{
			OpType: model.OpExpense,
			UUID:   "min-1",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(20000), Role: RoleExpense},
				{AccountID: f.card.ID, Amount: money.FromKopecks(20000), Role: RoleSource},
			},
		})
		if codeOf(err) != CodeMinBalance {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMinBalance, err)
		}
	})

	t.Run("свободный режим пропускает минус", func(t *testing.T) {
		f := newFixture(t)
		if err := f.db.SetMinBalance(ctx, f.card.ID, money.FromKopecks(-10000)); err != nil {
			t.Fatalf("SetMinBalance: %v", err)
		}
		setMode(t, f, store.BalanceModeFree)
		if _, err := f.post(t, Operation{
			OpType: model.OpExpense,
			UUID:   "min-2",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(20000), Role: RoleExpense},
				{AccountID: f.card.ID, Amount: money.FromKopecks(20000), Role: RoleSource},
			},
		}); err != nil {
			t.Fatalf("Post в свободном режиме: %v", err)
		}
		if got := f.balance(t, f.card); got != money.FromKopecks(-20000) {
			t.Fatalf("остаток карты = %s, ожидался -200,00 ₽", got)
		}
	})
}
