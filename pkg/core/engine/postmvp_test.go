package engine

import (
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// nonMoneyAsset создаёт неденежный актив (для переоценки).
func nonMoneyAsset(f *fixture) *model.Account {
	f.t.Helper()
	property := createTestAccount(f.t, f.db, "Имущество", model.TypeAsset, nil)
	return createTestAccount(f.t, f.db, "Недвижимость", model.TypeAsset, &property.ID)
}

// TestASL_6_DebtPayment проверяет строку 4 матрицы: погашение долга.
func TestASL_6_DebtPayment(t *testing.T) {
	f := newFixture(t)
	tr, err := f.post(t, Operation{
		OpType: model.OpDebtPayment,
		UUID:   "dp-1",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.credit.ID, Amount: money.FromKopecks(5000), Role: RoleLiability},
			{AccountID: f.wallet.ID, Amount: money.FromKopecks(5000), Role: RoleSource},
		},
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	sides := map[int64]model.Side{}
	for _, line := range tr.Lines {
		sides[line.AccountID] = line.Side
	}
	if sides[f.credit.ID] != model.SideDebit {
		t.Fatalf("обязательство: сторона %s, ожидался DEBIT", sides[f.credit.ID])
	}
	if sides[f.wallet.ID] != model.SideCredit {
		t.Fatalf("источник: сторона %s, ожидался CREDIT", sides[f.wallet.ID])
	}
}

// TestASL_6_Loan проверяет строку 5 матрицы: выдачу займа в ветку «Дебиторка».
func TestASL_6_Loan(t *testing.T) {
	t.Run("заём в дебиторку", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpLoan,
			UUID:   "loan-1",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.debtor.ID, Amount: money.FromKopecks(20000), Role: RoleDebtor},
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(20000), Role: RoleSource},
			},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
		if got := f.balance(t, f.debtor); got != money.FromKopecks(20000) {
			t.Fatalf("дебиторка = %s, ожидалось 200,00 ₽", got)
		}
	})

	t.Run("заём вне ветки Дебиторка", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpLoan,
			UUID:   "loan-2",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.card.ID, Amount: money.FromKopecks(20000), Role: RoleDebtor},
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(20000), Role: RoleSource},
			},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})
}

// TestASL_6_Revaluation проверяет строку 7 матрицы: переоценку.
func TestASL_6_Revaluation(t *testing.T) {
	t.Run("рост неденежного актива", func(t *testing.T) {
		f := newFixture(t)
		asset := nonMoneyAsset(f)
		tr, err := f.post(t, Operation{
			OpType:    model.OpRevaluation,
			UUID:      "reval-1",
			Date:      opDate(),
			Direction: DirectionUp,
			Entries:   []Entry{{AccountID: asset.ID, Amount: money.FromKopecks(100000), Role: RoleAsset}},
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if len(tr.Lines) != 2 {
			t.Fatalf("строк %d, ожидалось 2", len(tr.Lines))
		}
		capital := accountByPath(t, f.db, RevaluationCapitalPath)
		if got := f.balance(t, asset); got != money.FromKopecks(100000) {
			t.Fatalf("актив = %s, ожидалось 1000,00 ₽", got)
		}
		if got := f.balance(t, capital); got != money.FromKopecks(100000) {
			t.Fatalf("капитал переоценки = %s, ожидалось 1000,00 ₽", got)
		}
	})

	t.Run("снижение неденежного актива", func(t *testing.T) {
		f := newFixture(t)
		asset := nonMoneyAsset(f)
		if _, err := f.post(t, Operation{
			OpType:    model.OpRevaluation,
			UUID:      "reval-2",
			Date:      opDate(),
			Direction: DirectionUp,
			Entries:   []Entry{{AccountID: asset.ID, Amount: money.FromKopecks(100000), Role: RoleAsset}},
		}); err != nil {
			t.Fatalf("первый Post: %v", err)
		}
		if _, err := f.post(t, Operation{
			OpType:    model.OpRevaluation,
			UUID:      "reval-3",
			Date:      opDate(),
			Direction: DirectionDown,
			Entries:   []Entry{{AccountID: asset.ID, Amount: money.FromKopecks(30000), Role: RoleAsset}},
		}); err != nil {
			t.Fatalf("второй Post: %v", err)
		}
		if got := f.balance(t, asset); got != money.FromKopecks(70000) {
			t.Fatalf("актив = %s, ожидалось 700,00 ₽", got)
		}
	})

	t.Run("денежный счёт переоценивать нельзя", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType:    model.OpRevaluation,
			UUID:      "reval-4",
			Date:      opDate(),
			Direction: DirectionUp,
			Entries:   []Entry{{AccountID: f.wallet.ID, Amount: money.FromKopecks(10000), Role: RoleAsset}},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})

	t.Run("направление обязательно", func(t *testing.T) {
		f := newFixture(t)
		asset := nonMoneyAsset(f)
		_, err := f.post(t, Operation{
			OpType:  model.OpRevaluation,
			UUID:    "reval-5",
			Date:    opDate(),
			Entries: []Entry{{AccountID: asset.ID, Amount: money.FromKopecks(10000), Role: RoleAsset}},
		})
		if codeOf(err) != CodeDirectionRequired {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeDirectionRequired, err)
		}
	})
}

// TestASL_6_Adjustment проверяет строку 8 матрицы: корректировку.
func TestASL_6_Adjustment(t *testing.T) {
	t.Run("увеличение актива", func(t *testing.T) {
		f := newFixture(t)
		tr, err := f.post(t, Operation{
			OpType:    model.OpAdjustment,
			UUID:      "adj-1",
			Date:      opDate(),
			Direction: DirectionUp,
			Entries:   []Entry{{AccountID: f.wallet.ID, Amount: money.FromKopecks(5000), Role: RoleAdjust}},
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if len(tr.Lines) != 2 {
			t.Fatalf("строк %d, ожидалось 2", len(tr.Lines))
		}
		capital := accountByPath(t, f.db, AdjustmentCapitalPath)
		if got := f.balance(t, f.wallet); got != money.FromKopecks(5000) {
			t.Fatalf("кошелёк = %s, ожидалось 50,00 ₽", got)
		}
		if got := f.balance(t, capital); got != money.FromKopecks(5000) {
			t.Fatalf("капитал корректировки = %s, ожидалось 50,00 ₽", got)
		}
	})

	t.Run("уменьшение обязательства", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType:    model.OpAdjustment,
			UUID:      "adj-2",
			Date:      opDate(),
			Direction: DirectionUp,
			Entries:   []Entry{{AccountID: f.credit.ID, Amount: money.FromKopecks(7000), Role: RoleAdjust}},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
		if got := f.balance(t, f.credit); got != money.FromKopecks(7000) {
			t.Fatalf("обязательство = %s, ожидалось 70,00 ₽", got)
		}
	})

	t.Run("расход корректировать нельзя", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType:    model.OpAdjustment,
			UUID:      "adj-3",
			Date:      opDate(),
			Direction: DirectionUp,
			Entries:   []Entry{{AccountID: f.expense.ID, Amount: money.FromKopecks(1000), Role: RoleAdjust}},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})
}
