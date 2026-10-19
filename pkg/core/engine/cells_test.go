package engine

import (
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_6_MatrixAllCells прогоняет по одной допустимой и одной недопустимой
// ячейке для каждой из восьми строк матрицы ASL-2.2.
func TestASL_6_MatrixAllCells(t *testing.T) {
	kop := money.FromKopecks

	cases := []struct {
		name     string
		build    func(f *fixture) Operation
		wantCode string
	}{
		{
			name: "трата: расход DEBIT + актив CREDIT",
			build: func(f *fixture) Operation {
				return f.expenseOp("cell-expense-ok", kop(10000))
			},
		},
		{
			name: "трата: доход в источнике",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpExpense, UUID: "cell-expense-bad", Date: opDate(), Entries: []Entry{
					{AccountID: f.expense.ID, Amount: kop(10000), Role: RoleExpense},
					{AccountID: f.income.ID, Amount: kop(10000), Role: RoleSource},
				}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "доход: актив DEBIT + доход CREDIT",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpIncome, UUID: "cell-income-ok", Date: opDate(), Entries: []Entry{
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleReceipt},
					{AccountID: f.income.ID, Amount: kop(10000), Role: RoleIncome},
				}}
			},
		},
		{
			name: "доход: расход в поступлении",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpIncome, UUID: "cell-income-bad", Date: opDate(), Entries: []Entry{
					{AccountID: f.expense.ID, Amount: kop(10000), Role: RoleReceipt},
					{AccountID: f.income.ID, Amount: kop(10000), Role: RoleIncome},
				}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "перевод: актив DEBIT + актив CREDIT",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpTransfer, UUID: "cell-transfer-ok", Date: opDate(), Entries: []Entry{
					{AccountID: f.card.ID, Amount: kop(10000), Role: RoleTo},
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleFrom},
				}}
			},
		},
		{
			name: "перевод: пассив в переводе",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpTransfer, UUID: "cell-transfer-bad", Date: opDate(), Entries: []Entry{
					{AccountID: f.credit.ID, Amount: kop(10000), Role: RoleFrom},
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleTo},
				}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "погашение долга: пассив DEBIT + актив CREDIT",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpDebtPayment, UUID: "cell-debt-ok", Date: opDate(), Entries: []Entry{
					{AccountID: f.credit.ID, Amount: kop(10000), Role: RoleLiability},
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleSource},
				}}
			},
		},
		{
			name: "погашение долга: доход в обязательстве",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpDebtPayment, UUID: "cell-debt-bad", Date: opDate(), Entries: []Entry{
					{AccountID: f.income.ID, Amount: kop(10000), Role: RoleLiability},
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleSource},
				}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "выдача займа: дебиторка DEBIT + актив CREDIT",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpLoan, UUID: "cell-loan-ok", Date: opDate(), Entries: []Entry{
					{AccountID: f.debtor.ID, Amount: kop(10000), Role: RoleDebtor},
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleSource},
				}}
			},
		},
		{
			name: "выдача займа: дебитор не из ветки",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpLoan, UUID: "cell-loan-bad", Date: opDate(), Entries: []Entry{
					{AccountID: f.card.ID, Amount: kop(10000), Role: RoleDebtor},
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleSource},
				}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "начальные остатки: актив DEBIT + пассив CREDIT",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpInitial, UUID: "cell-initial-ok", Date: opDate(), Entries: []Entry{
					{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleAsset},
					{AccountID: f.credit.ID, Amount: kop(4000), Role: RoleLiability},
				}}
			},
		},
		{
			name: "начальные остатки: доход в активе",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpInitial, UUID: "cell-initial-bad", Date: opDate(), Entries: []Entry{
					{AccountID: f.income.ID, Amount: kop(10000), Role: RoleAsset},
				}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "переоценка: неденежный актив",
			build: func(f *fixture) Operation {
				asset := nonMoneyAsset(f)
				return Operation{OpType: model.OpRevaluation, UUID: "cell-reval-ok", Date: opDate(), Direction: DirectionUp,
					Entries: []Entry{{AccountID: asset.ID, Amount: kop(10000), Role: RoleAsset}}}
			},
		},
		{
			name: "переоценка: доход",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpRevaluation, UUID: "cell-reval-bad", Date: opDate(), Direction: DirectionUp,
					Entries: []Entry{{AccountID: f.income.ID, Amount: kop(10000), Role: RoleAsset}}}
			},
			wantCode: CodeMatrixCell,
		},
		{
			name: "корректировка: актив",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpAdjustment, UUID: "cell-adj-ok", Date: opDate(), Direction: DirectionUp,
					Entries: []Entry{{AccountID: f.wallet.ID, Amount: kop(10000), Role: RoleAdjust}}}
			},
		},
		{
			name: "корректировка: расход",
			build: func(f *fixture) Operation {
				return Operation{OpType: model.OpAdjustment, UUID: "cell-adj-bad", Date: opDate(), Direction: DirectionUp,
					Entries: []Entry{{AccountID: f.expense.ID, Amount: kop(10000), Role: RoleAdjust}}}
			},
			wantCode: CodeMatrixCell,
		},
	}

	covered := map[model.OpType]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			op := tc.build(f)
			covered[op.OpType] = true
			_, err := f.post(t, op)
			if codeOf(err) != tc.wantCode {
				t.Fatalf("код ошибки = %q, ожидался %q (err=%v)", codeOf(err), tc.wantCode, err)
			}
		})
	}

	for _, op := range model.OpTypes() {
		if !covered[op] {
			t.Fatalf("операция %q не покрыта тестом ячеек матрицы", op)
		}
	}
}
