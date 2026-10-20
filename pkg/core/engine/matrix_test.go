package engine

import (
	"slices"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// TestASL_6_MatrixHasEightRows проверяет, что закодированы все восемь строк
// матрицы ASL-2.2 в порядке эпика.
func TestASL_6_MatrixHasEightRows(t *testing.T) {
	rules := Rules()
	if len(rules) != 8 {
		t.Fatalf("строк матрицы %d, ожидалось 8", len(rules))
	}
	want := []model.OpType{
		model.OpExpense, model.OpIncome, model.OpTransfer, model.OpDebtPayment,
		model.OpLoan, model.OpInitial, model.OpRevaluation, model.OpAdjustment,
	}
	for i, op := range want {
		if rules[i].Op != op {
			t.Fatalf("строка %d: операция %q, ожидалось %q", i, rules[i].Op, op)
		}
		if rules[i].Title == "" || len(rules[i].Debit) == 0 || len(rules[i].Credit) == 0 {
			t.Fatalf("строка %q заполнена неполно: %+v", op, rules[i])
		}
		if rules[i].MinDebit < 1 || rules[i].MinCredit < 1 {
			t.Fatalf("строка %q: минимумы строк должны быть ≥ 1", op)
		}
	}
}

// TestASL_6_MatrixCells проверяет вывод ячейки по паре (операция, тип счёта).
func TestASL_6_MatrixCells(t *testing.T) {
	tests := []struct {
		name string
		op   model.OpType
		typ  model.AccountType
		side model.Side
		ok   bool
	}{

		{"трата: расход в дебете", model.OpExpense, model.TypeExpense, model.SideDebit, true},
		{"трата: актив в кредите", model.OpExpense, model.TypeAsset, model.SideCredit, true},
		{"трата: доход недопустим", model.OpExpense, model.TypeIncome, "", false},
		{"доход: актив в дебете", model.OpIncome, model.TypeAsset, model.SideDebit, true},
		{"доход: доход в кредите", model.OpIncome, model.TypeIncome, model.SideCredit, true},
		{"доход: расход недопустим", model.OpIncome, model.TypeExpense, "", false},
		{"перевод: актив в дебете", model.OpTransfer, model.TypeAsset, model.SideDebit, true},
		{"перевод: расход недопустим", model.OpTransfer, model.TypeExpense, "", false},
		{"погашение долга: пассив в дебете", model.OpDebtPayment, model.TypeLiability, model.SideDebit, true},
		{"погашение долга: доход недопустим", model.OpDebtPayment, model.TypeIncome, "", false},
		{"начальные остатки: актив в дебете", model.OpInitial, model.TypeAsset, model.SideDebit, true},
		{"начальные остатки: пассив в кредите", model.OpInitial, model.TypeLiability, model.SideCredit, true},
		{"начальные остатки: доход недопустим", model.OpInitial, model.TypeIncome, "", false},
		{"переоценка: актив допустим", model.OpRevaluation, model.TypeAsset, model.SideDebit, true},
		{"переоценка: доход недопустим", model.OpRevaluation, model.TypeIncome, "", false},
		{"корректировка: капитал допустим", model.OpAdjustment, model.TypeCapital, model.SideDebit, true},
		{"корректировка: расход недопустим", model.OpAdjustment, model.TypeExpense, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rule, ok := RuleFor(tc.op)
			if !ok {
				t.Fatalf("нет строки матрицы для %q", tc.op)
			}
			side, allowed := rule.Allows(tc.typ)
			if allowed != tc.ok {
				t.Fatalf("Allows(%s, %s) = %v, ожидалось %v", tc.op, tc.typ, allowed, tc.ok)
			}
			if allowed && side != tc.side {
				t.Fatalf("Allows(%s, %s) дал сторону %s, ожидалось %s", tc.op, tc.typ, side, tc.side)
			}
		})
	}
}

// TestASL_6_ReservedCells проверяет пометку зарезервированных ячеек post-MVP.
func TestASL_6_ReservedCells(t *testing.T) {
	expense, ok := RuleFor(model.OpExpense)
	if !ok {
		t.Fatal("нет строки матрицы для траты")
	}
	if !expense.Reserved(model.TypeLiability) {
		t.Fatal("оплата в кредит (пассив в трате) не помечена зарезервированной")
	}
	if expense.Reserved(model.TypeAsset) {
		t.Fatal("актив в трате ошибочно помечен зарезервированным")
	}
}

// TestASL_6_MVPOperations проверяет, какие операции отнесены к MVP.
func TestASL_6_MVPOperations(t *testing.T) {
	want := map[model.OpType]bool{
		model.OpExpense: true, model.OpIncome: true, model.OpTransfer: true, model.OpInitial: true,
		model.OpDebtPayment: false, model.OpLoan: false, model.OpRevaluation: false, model.OpAdjustment: false,
	}
	for op, mvp := range want {
		rule, ok := RuleFor(op)
		if !ok {
			t.Fatalf("нет строки матрицы для %q", op)
		}
		if rule.MVP != mvp {
			t.Fatalf("MVP(%s) = %v, ожидалось %v", op, rule.MVP, mvp)
		}
	}
}

// TestASL_6_UnknownOperation проверяет отсутствие строки для неизвестной операции.
func TestASL_6_UnknownOperation(t *testing.T) {
	if _, ok := RuleFor(model.OpType("вымысел")); ok {
		t.Fatal("для неизвестной операции нашлась строка матрицы")
	}
}

// TestRolesFor проверяет клиентское описание ролей для всех операций.
func TestRolesFor(t *testing.T) {
	for _, op := range model.OpTypes() {
		roles, ok := RolesFor(op)
		if !ok {
			t.Fatalf("нет ролей для %q", op)
		}
		if len(roles) == 0 {
			t.Fatalf("пустой список ролей для %q", op)
		}
		for _, role := range roles {
			if role.Role == "" || len(role.AccountTypes) == 0 {
				t.Fatalf("%s: неполная роль %+v", op, role)
			}
			if role.DirectionRequired && role.Side != "" {
				t.Fatalf("%s/%s: сторона задана при обязательном направлении", op, role.Role)
			}
			if !role.DirectionRequired && !role.Side.Valid() {
				t.Fatalf("%s/%s: недопустимая сторона %q", op, role.Role, role.Side)
			}
			rule, _ := RuleFor(op)
			if role.Reserved != !rule.MVP {
				t.Fatalf("%s/%s: reserved=%v, MVP=%v", op, role.Role, role.Reserved, rule.MVP)
			}
		}

		// Для операций с фиксированными сторонами ответ обязан совпадать
		// с roleSpecs — единственным источником правил движка.
		if op != model.OpRevaluation && op != model.OpAdjustment {
			specs := roleSpecs[op]
			if len(roles) != len(specs) {
				t.Fatalf("%s: ролей %d, в roleSpecs %d", op, len(roles), len(specs))
			}
			for _, role := range roles {
				spec, ok := specs[role.Role]
				if !ok {
					t.Fatalf("%s: роли %q нет в roleSpecs", op, role.Role)
				}
				if spec.side != role.Side || !slices.Equal(spec.types, role.AccountTypes) {
					t.Fatalf("%s/%s: ответ %+v не совпадает с roleSpecs %+v",
						op, role.Role, role, spec)
				}
			}
		}
	}

	// Переоценка и корректировка выполняются по направлению.
	for _, op := range []model.OpType{model.OpRevaluation, model.OpAdjustment} {
		roles, ok := RolesFor(op)
		if !ok || len(roles) != 1 {
			t.Fatalf("%s: роли %+v, ok=%v", op, roles, ok)
		}
		if !roles[0].DirectionRequired || roles[0].Side != "" || roles[0].Role != RoleAdjust {
			t.Fatalf("%s: роль %+v, ожидалась корректируемая без стороны", op, roles[0])
		}
	}

	if _, ok := RolesFor(model.OpType("вымысел")); ok {
		t.Fatal("для неизвестной операции нашлись роли")
	}
}
