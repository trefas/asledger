package engine

import (
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_17_InterestAccrual проверяет начисление процентов по кредиту и
// вкладу через роли новой операции.
func TestASL_17_InterestAccrual(t *testing.T) {
	t.Run("проценты по кредиту", func(t *testing.T) {
		f := newFixture(t)
		tr, err := f.post(t, Operation{
			OpType: model.OpInterestAccrual,
			UUID:   "interest-credit",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleInterestExpense},
				{AccountID: f.credit.ID, Amount: money.FromKopecks(10000), Role: RoleLiability},
			},
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		sides := lineSides(tr)
		if sides[f.expense.ID] != model.SideDebit {
			t.Fatalf("расход: сторона %s, ожидался DEBIT", sides[f.expense.ID])
		}
		if sides[f.credit.ID] != model.SideCredit {
			t.Fatalf("пассив: сторона %s, ожидался CREDIT", sides[f.credit.ID])
		}
	})

	t.Run("проценты по вкладу", func(t *testing.T) {
		f := newFixture(t)
		tr, err := f.post(t, Operation{
			OpType: model.OpInterestAccrual,
			UUID:   "interest-deposit",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.card.ID, Amount: money.FromKopecks(10000), Role: RoleAsset},
				{AccountID: f.income.ID, Amount: money.FromKopecks(10000), Role: RoleInterestIncome},
			},
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		sides := lineSides(tr)
		if sides[f.card.ID] != model.SideDebit {
			t.Fatalf("актив: сторона %s, ожидался DEBIT", sides[f.card.ID])
		}
		if sides[f.income.ID] != model.SideCredit {
			t.Fatalf("доход: сторона %s, ожидался CREDIT", sides[f.income.ID])
		}
	})

	t.Run("недопустимая ячейка", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.post(t, Operation{
			OpType: model.OpInterestAccrual,
			UUID:   "interest-bad",
			Date:   opDate(),
			Entries: []Entry{
				{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleInterestIncome},
				{AccountID: f.credit.ID, Amount: money.FromKopecks(10000), Role: RoleLiability},
			},
		})
		if codeOf(err) != CodeMatrixCell {
			t.Fatalf("код ошибки = %q, ожидался %s (err=%v)", codeOf(err), CodeMatrixCell, err)
		}
	})
}

// TestASL_17_DebtPaymentInterest проверяет расширение погашения долга:
// проценты отдельной строкой расхода в дебете.
func TestASL_17_DebtPaymentInterest(t *testing.T) {
	f := newFixture(t)
	tr, err := f.post(t, Operation{
		OpType: model.OpDebtPayment,
		UUID:   "debt-with-interest",
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.credit.ID, Amount: money.FromKopecks(30000), Role: RoleLiability},
			{AccountID: f.expense.ID, Amount: money.FromKopecks(10000), Role: RoleInterest},
			{AccountID: f.wallet.ID, Amount: money.FromKopecks(40000), Role: RoleSource},
		},
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	sides := lineSides(tr)
	if sides[f.credit.ID] != model.SideDebit {
		t.Fatalf("тело долга: сторона %s, ожидался DEBIT", sides[f.credit.ID])
	}
	if sides[f.expense.ID] != model.SideDebit {
		t.Fatalf("проценты: сторона %s, ожидался DEBIT", sides[f.expense.ID])
	}
	if sides[f.wallet.ID] != model.SideCredit {
		t.Fatalf("источник: сторона %s, ожидался CREDIT", sides[f.wallet.ID])
	}
}

// lineSides строит карту «счёт → сторона» по строкам проводки.
func lineSides(tr *model.Transaction) map[int64]model.Side {
	sides := make(map[int64]model.Side, len(tr.Lines))
	for _, line := range tr.Lines {
		sides[line.AccountID] = line.Side
	}
	return sides
}
