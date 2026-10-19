package engine

import (
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_6_Expense проверяет строку 1 матрицы: трату, включая частичную оплату
// несколькими источниками.
func TestASL_6_Expense(t *testing.T) {
	t.Run("статья и источник", func(t *testing.T) {
		f := newFixture(t)
		tr, err := f.post(t, f.expenseOp("exp-1", money.FromKopecks(25000)))
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if tr.OpType != model.OpExpense {
			t.Fatalf("тип операции = %s", tr.OpType)
		}
		if got := f.balance(t, f.expense); got != money.FromKopecks(25000) {
			t.Fatalf("статья расхода = %s, ожидалось 250,00 ₽", got)
		}
		if got := f.balance(t, f.wallet); got != money.FromKopecks(-25000) {
			t.Fatalf("кошелёк = %s, ожидалось -250,00 ₽", got)
		}
	})

	t.Run("несколько статей расхода", func(t *testing.T) {
		f := newFixture(t)
		expenses := accountByPath(t, f.db, "Расходы")
		transport := createTestAccount(t, f.db, "Транспорт", model.TypeExpense, &expenses.ID)
		tr, err := f.post(t, Operation{
			OpType: model.OpExpense,
			UUID:   "exp-2",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
				{AccountID: transport.ID, Amount: money.FromKopecks(5000), Role: RoleExpense},
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(15000), Role: RoleSource},
			},
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if len(tr.Lines) != 3 {
			t.Fatalf("строк %d, ожидалось 3", len(tr.Lines))
		}
	})

	t.Run("частичная оплата двумя источниками", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpExpense,
			UUID:   "exp-3",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(30000), Role: RoleExpense},
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(20000), Role: RoleSource},
				{AccountID: f.card.ID, Amount: money.FromKopecks(10000), Role: RoleSource},
			},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
		if got := f.balance(t, f.wallet); got != money.FromKopecks(-20000) {
			t.Fatalf("кошелёк = %s, ожидалось -200,00 ₽", got)
		}
		if got := f.balance(t, f.card); got != money.FromKopecks(-10000) {
			t.Fatalf("карта = %s, ожидалось -100,00 ₽", got)
		}
	})

	t.Run("Σ расходов не равна Σ источников", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpExpense,
			UUID:   "exp-4",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(30000), Role: RoleExpense},
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(20000), Role: RoleSource},
				{AccountID: f.card.ID, Amount: money.FromKopecks(5000), Role: RoleSource},
			},
		})
		if codeOf(err) != CodeUnbalanced {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeUnbalanced, err)
		}
	})

	t.Run("источник — не актив", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpExpense,
			UUID:   "exp-5",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleExpense},
				{AccountID: f.income.ID, Amount: money.FromKopecks(10000), Role: RoleSource},
			},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})
}

// TestASL_6_Income проверяет строку 2 матрицы: доход.
func TestASL_6_Income(t *testing.T) {
	t.Run("поступление на актив", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpIncome,
			UUID:   "inc-1",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(50000), Role: RoleReceipt},
				{AccountID: f.income.ID, Amount: money.FromKopecks(50000), Role: RoleIncome},
			},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
		if got := f.balance(t, f.wallet); got != money.FromKopecks(50000) {
			t.Fatalf("кошелёк = %s, ожидалось 500,00 ₽", got)
		}
		if got := f.balance(t, f.income); got != money.FromKopecks(50000) {
			t.Fatalf("доход = %s, ожидалось 500,00 ₽", got)
		}
	})

	t.Run("Σ доходов не равна Σ поступлений", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpIncome,
			UUID:   "inc-2",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(50000), Role: RoleReceipt},
				{AccountID: f.income.ID, Amount: money.FromKopecks(40000), Role: RoleIncome},
			},
		})
		if codeOf(err) != CodeUnbalanced {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeUnbalanced, err)
		}
	})

	t.Run("доход не может стоять в дебете", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpIncome,
			UUID:   "inc-3",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.income.ID, Amount: money.FromKopecks(10000), Role: RoleReceipt},
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(10000), Role: RoleIncome},
			},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})
}

// TestASL_6_Transfer проверяет строку 3 матрицы: перевод между активами.
func TestASL_6_Transfer(t *testing.T) {
	t.Run("между двумя активами", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpTransfer,
			UUID:   "tr-1",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(15000), Role: RoleFrom},
				{AccountID: f.card.ID, Amount: money.FromKopecks(15000), Role: RoleTo},
			},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
		if got := f.balance(t, f.wallet); got != money.FromKopecks(-15000) {
			t.Fatalf("кошелёк = %s, ожидалось -150,00 ₽", got)
		}
		if got := f.balance(t, f.card); got != money.FromKopecks(15000) {
			t.Fatalf("карта = %s, ожидалось 150,00 ₽", got)
		}
	})

	t.Run("возврат займа — перевод на ветку Дебиторка", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpTransfer,
			UUID:   "tr-2",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(10000), Role: RoleFrom},
				{AccountID: f.debtor.ID, Amount: money.FromKopecks(10000), Role: RoleTo},
			},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
	})

	t.Run("не актив в переводе", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpTransfer,
			UUID:   "tr-3",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleFrom},
				{AccountID: f.card.ID, Amount: money.FromKopecks(10000), Role: RoleTo},
			},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})
}

// TestASL_6_Initial проверяет строку 6 матрицы: начальные остатки и авто-капитал.
func TestASL_6_Initial(t *testing.T) {
	t.Run("активы больше обязательств", func(t *testing.T) {
		f := newFixture(t)
		tr, err := f.post(t, Operation{
			OpType: model.OpInitial,
			UUID:   "init-1",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(100000), Role: RoleAsset},
				{AccountID: f.credit.ID, Amount: money.FromKopecks(30000), Role: RoleLiability},
			},
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if len(tr.Lines) != 3 {
			t.Fatalf("строк %d, ожидалось 3 (актив, обязательство, капитал)", len(tr.Lines))
		}
		capital := accountByPath(t, f.db, InitialCapitalPath)
		if got := f.balance(t, capital); got != money.FromKopecks(70000) {
			t.Fatalf("капитал = %s, ожидалось 700,00 ₽", got)
		}
	})

	t.Run("обязательства больше активов", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpInitial,
			UUID:   "init-2",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(10000), Role: RoleAsset},
				{AccountID: f.credit.ID, Amount: money.FromKopecks(30000), Role: RoleLiability},
			},
		}); err != nil {
			t.Fatalf("Post: %v", err)
		}
		capital := accountByPath(t, f.db, InitialCapitalPath)
		if got := f.balance(t, capital); got != money.FromKopecks(-20000) {
			t.Fatalf("капитал = %s, ожидалось -200,00 ₽", got)
		}
	})

	t.Run("ввод возможен только один раз", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.post(t, Operation{
			OpType: model.OpInitial,
			UUID:   "init-3",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(10000), Role: RoleAsset},
			},
		}); err != nil {
			t.Fatalf("первый Post: %v", err)
		}
		_, err := f.post(t, Operation{
			OpType: model.OpInitial,
			UUID:   "init-4",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.wallet.ID, Amount: money.FromKopecks(5000), Role: RoleAsset},
			},
		})
		if codeOf(err) != CodeInitialDone {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeInitialDone, err)
		}
	})

	t.Run("ручная строка капитала запрещена", func(t *testing.T) {
		f := newFixture(t)
		capital := accountByPath(t, f.db, InitialCapitalPath)
		_, err := f.post(t, Operation{
			OpType: model.OpInitial,
			UUID:   "init-5",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: capital.ID, Amount: money.FromKopecks(10000), Role: RoleAsset},
			},
		})
		if codeOf(err) != CodeSystemAccount {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeSystemAccount, err)
		}
	})
}
