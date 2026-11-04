package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/trefas/asledger/pkg/core/model"
)

// Role — семантическая роль строки в операции. Пользователь задаёт роль, а не
// сторону: сторону (DEBIT/CREDIT) назначает движок по матрице.
type Role string

// Роли строк операций.
const (
	// RoleExpense — статья расхода (DEBIT в трате).
	RoleExpense Role = "расход"
	// RoleSource — источник средств (CREDIT).
	RoleSource Role = "источник"
	// RoleIncome — статья дохода (CREDIT).
	RoleIncome Role = "доход"
	// RoleReceipt — поступление на актив (DEBIT).
	RoleReceipt Role = "поступление"
	// RoleFrom — счёт-источник перевода (CREDIT).
	RoleFrom Role = "списание"
	// RoleTo — счёт-получатель перевода (DEBIT).
	RoleTo Role = "зачисление"
	// RoleAsset — счёт актива в начальных остатках/переоценке (DEBIT или по направлению).
	RoleAsset Role = "актив"
	// RoleLiability — счёт обязательства (CREDIT).
	RoleLiability Role = "пассив"
	// RoleDebtor — счёт выданного займа (DEBIT, только ветка «Дебиторка»).
	RoleDebtor Role = "дебитор"
	// RoleInterestExpense — статья расходов на проценты (DEBIT).
	RoleInterestExpense Role = "проценты_расход"
	// RoleInterestIncome — статья доходов по процентам (CREDIT).
	RoleInterestIncome Role = "проценты_доход"
	// RoleInterest — статья расходов на проценты при погашении долга (DEBIT).
	RoleInterest Role = "проценты"
	// RoleAdjust — корректируемый счёт (сторона по направлению).
	RoleAdjust Role = "корректируемый"
)

// Rule — строка матрицы ASL-2.2: допустимые типы счетов по сторонам.
type Rule struct {
	// Op — тип операции.
	Op model.OpType
	// Title — человекочитаемое название операции.
	Title string
	// Debit и Credit — типы счетов, допустимые на соответствующей стороне.
	Debit  []model.AccountType
	Credit []model.AccountType
	// MinDebit и MinCredit — минимальное число строк на стороне.
	MinDebit  int
	MinCredit int
	// ReservedDebit и ReservedCredit — зарезервированные (post-MVP) типы.
	ReservedDebit  []model.AccountType
	ReservedCredit []model.AccountType
	// MVP отмечает операции первой очереди.
	MVP bool
	// Notes — примечание из матрицы.
	Notes string
}

// matrix — восемь строк матрицы ASL-2.2 в порядке эпика.
var matrix = []Rule{
	{
		Op: model.OpExpense, Title: "Трата",
		Debit: []model.AccountType{model.TypeExpense}, Credit: []model.AccountType{model.TypeAsset},
		MinDebit: 1, MinCredit: 1, MVP: true,
		ReservedCredit: []model.AccountType{model.TypeLiability},
		Notes:          "Σ(расход) = Σ(источники); источники — листовые активы. Оплата в кредит — post-MVP.",
	},
	{
		Op: model.OpIncome, Title: "Доход",
		Debit: []model.AccountType{model.TypeAsset}, Credit: []model.AccountType{model.TypeIncome},
		MinDebit: 1, MinCredit: 1, MVP: true,
		Notes: "Σ(доходы) = Σ(поступления); поступления — активы.",
	},
	{
		Op: model.OpTransfer, Title: "Перевод",
		Debit: []model.AccountType{model.TypeAsset}, Credit: []model.AccountType{model.TypeAsset},
		MinDebit: 1, MinCredit: 1, MVP: true,
		Notes: "Обе стороны — листовые активы. Возврат займа — перевод на счёт ветки «Дебиторка».",
	},
	{
		Op: model.OpDebtPayment, Title: "Погашение долга",
		Debit: []model.AccountType{model.TypeLiability, model.TypeExpense}, Credit: []model.AccountType{model.TypeAsset},
		MinDebit: 1, MinCredit: 1,
		Notes: "Σ(погашение) = Σ(источники); уменьшает обязательство. Проценты — отдельной строкой расхода.",
	},
	{
		Op: model.OpLoan, Title: "Выдача займа",
		Debit: []model.AccountType{model.TypeAsset}, Credit: []model.AccountType{model.TypeAsset},
		MinDebit: 1, MinCredit: 1,
		Notes: "DEBIT-счёт обязан лежать в ветке «Дебиторка»; источник — листовой актив.",
	},
	{
		Op: model.OpInitial, Title: "Начальные остатки",
		Debit:    []model.AccountType{model.TypeAsset, model.TypeCapital},
		Credit:   []model.AccountType{model.TypeLiability, model.TypeCapital},
		MinDebit: 1, MinCredit: 1, MVP: true,
		Notes: "Ровно один раз (флаг в settings); строка «Капитал:Начальные» генерируется движком.",
	},
	{
		Op: model.OpRevaluation, Title: "Переоценка",
		Debit:    []model.AccountType{model.TypeAsset, model.TypeCapital},
		Credit:   []model.AccountType{model.TypeAsset, model.TypeCapital},
		MinDebit: 1, MinCredit: 1,
		Notes: "Ровно 2 строки: актив (не денежный) и «Капитал:Переоценка» с противоположных сторон.",
	},
	{
		Op: model.OpAdjustment, Title: "Корректировка",
		Debit:    []model.AccountType{model.TypeAsset, model.TypeLiability, model.TypeCapital},
		Credit:   []model.AccountType{model.TypeAsset, model.TypeLiability, model.TypeCapital},
		MinDebit: 1, MinCredit: 1,
		Notes: "Ровно 2 строки: счёт пользователя и «Капитал:Корректировка» с противоположных сторон.",
	},
	{
		Op: model.OpInterestAccrual, Title: "Начисление процентов",
		Debit:    []model.AccountType{model.TypeExpense, model.TypeAsset},
		Credit:   []model.AccountType{model.TypeLiability, model.TypeIncome},
		MinDebit: 1, MinCredit: 1,
		Notes: "Проценты по кредиту: расход DEBIT, пассив CREDIT. По вкладу: актив DEBIT, доход CREDIT.",
	},
}

// matrixByOp — матрица, проиндексированная по типу операции.
var matrixByOp = func() map[model.OpType]Rule {
	m := make(map[model.OpType]Rule, len(matrix))
	for _, r := range matrix {
		m[r.Op] = r
	}
	return m
}()

// Rules возвращает все строки матрицы в порядке эпика.
func Rules() []Rule {
	out := make([]Rule, len(matrix))
	copy(out, matrix)
	return out
}

// RuleFor возвращает строку матрицы для типа операции.
func RuleFor(op model.OpType) (Rule, bool) {
	r, ok := matrixByOp[op]
	return r, ok
}

// RoleInfo — описание роли для клиентов: сторона, допустимые типы счетов и
// необходимость явно задать направление.
type RoleInfo struct {
	// Role — семантическая роль строки.
	Role Role
	// Side — сторона, которую назначит движок; пусто для операций по направлению.
	Side model.Side
	// AccountTypes — допустимые типы счетов для роли.
	AccountTypes []model.AccountType
	// DirectionRequired — клиент обязан передать направление (вверх/вниз).
	DirectionRequired bool
	// Reserved — роль относится к операции post-MVP и в MVP скрывается.
	Reserved bool
}

// RolesFor возвращает роли операции в порядке «дебет → кредит».
//
// Для переоценки и корректировки сторона зависит от направления, поэтому
// возвращается одна роль RoleAdjust с пустой стороной и DirectionRequired.
func RolesFor(op model.OpType) ([]RoleInfo, bool) {
	rule, ok := RuleFor(op)
	if !ok {
		return nil, false
	}
	if op == model.OpRevaluation || op == model.OpAdjustment {
		return []RoleInfo{{
			Role:              RoleAdjust,
			Side:              "",
			AccountTypes:      []model.AccountType{model.TypeAsset, model.TypeLiability, model.TypeCapital},
			DirectionRequired: true,
			Reserved:          !rule.MVP,
		}}, true
	}

	specs, ok := roleSpecs[op]
	if !ok {
		return nil, false
	}
	roles := make([]RoleInfo, 0, len(specs))
	for role, spec := range specs {
		types := make([]model.AccountType, len(spec.types))
		copy(types, spec.types)
		roles = append(roles, RoleInfo{
			Role:         role,
			Side:         spec.side,
			AccountTypes: types,
			Reserved:     !rule.MVP,
		})
	}
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Side != roles[j].Side {
			return roles[i].Side == model.SideDebit
		}
		return roles[i].Role < roles[j].Role
	})
	return roles, true
}

// sideOfType сообщает, допустим ли тип счёта на стороне строки матрицы.
func (r Rule) sideOfType(t model.AccountType) (model.Side, bool) {
	if containsType(r.Debit, t) {
		return model.SideDebit, true
	}
	if containsType(r.Credit, t) {
		return model.SideCredit, true
	}
	return "", false
}

// Allows сообщает, допустим ли тип счёта в ячейке матрицы, и на какой стороне.
//
// Для типов, допустимых на обеих сторонах (начальные остатки, переоценка,
// корректировка), возвращается сторона DEBIT: фактическую сторону движок
// назначает по роли и направлению.
func (r Rule) Allows(t model.AccountType) (model.Side, bool) {
	return r.sideOfType(t)
}

// Reserved сообщает, зарезервирована ли ячейка для типа счёта (post-MVP).
func (r Rule) Reserved(t model.AccountType) bool {
	return containsType(r.ReservedDebit, t) || containsType(r.ReservedCredit, t)
}

// CellName возвращает имя ячейки матрицы для сообщений об ошибках.
func CellName(op model.OpType, side model.Side, t model.AccountType) string {
	return fmt.Sprintf("%s/%s/%s", op, side, t)
}

// roleSpec — сторона и допустимые типы счёта для роли в операции.
type roleSpec struct {
	side  model.Side
	types []model.AccountType
}

// roleSpecs — правила назначения сторон по ролям для операций с фиксированными
// сторонами. Переоценка и корректировка зависят от направления и обрабатываются
// отдельно (см. buildDirectional).
var roleSpecs = map[model.OpType]map[Role]roleSpec{
	model.OpExpense: {
		RoleExpense: {side: model.SideDebit, types: []model.AccountType{model.TypeExpense}},
		RoleSource:  {side: model.SideCredit, types: []model.AccountType{model.TypeAsset}},
	},
	model.OpIncome: {
		RoleReceipt: {side: model.SideDebit, types: []model.AccountType{model.TypeAsset}},
		RoleIncome:  {side: model.SideCredit, types: []model.AccountType{model.TypeIncome}},
	},
	model.OpTransfer: {
		RoleFrom: {side: model.SideCredit, types: []model.AccountType{model.TypeAsset}},
		RoleTo:   {side: model.SideDebit, types: []model.AccountType{model.TypeAsset}},
	},
	model.OpDebtPayment: {
		RoleLiability: {side: model.SideDebit, types: []model.AccountType{model.TypeLiability}},
		RoleInterest:  {side: model.SideDebit, types: []model.AccountType{model.TypeExpense}},
		RoleSource:    {side: model.SideCredit, types: []model.AccountType{model.TypeAsset}},
	},
	model.OpLoan: {
		RoleDebtor: {side: model.SideDebit, types: []model.AccountType{model.TypeAsset}},
		RoleSource: {side: model.SideCredit, types: []model.AccountType{model.TypeAsset}},
	},
	model.OpInitial: {
		RoleAsset:     {side: model.SideDebit, types: []model.AccountType{model.TypeAsset}},
		RoleLiability: {side: model.SideCredit, types: []model.AccountType{model.TypeLiability}},
	},
	model.OpInterestAccrual: {
		RoleInterestExpense: {side: model.SideDebit, types: []model.AccountType{model.TypeExpense}},
		RoleAsset:           {side: model.SideDebit, types: []model.AccountType{model.TypeAsset}},
		RoleInterestIncome:  {side: model.SideCredit, types: []model.AccountType{model.TypeIncome}},
		RoleLiability:       {side: model.SideCredit, types: []model.AccountType{model.TypeLiability}},
	},
}

// containsType сообщает, входит ли тип счёта в список.
func containsType(types []model.AccountType, t model.AccountType) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

// DebtorBranch — ветка дебиторской задолженности для операции выдачи займа.
const DebtorBranch = "Активы:Дебиторка"

// moneyMarkers — фрагменты имён веток, обозначающие денежные счета.
//
// До появления явного признака в модели денежность определяется соглашением об
// именах: денежными считаются активы, в пути которых встречается один из этих
// фрагментов. Переоценка денежных счетов запрещена (для этого корректировка).
var moneyMarkers = []string{"денеж", "наличн", "карт", "кошел", "счёт", "счет", "счета", "дебиторк"}

// IsMoneyAccount сообщает, относится ли счёт к денежным (по соглашению об именах).
func IsMoneyAccount(fullPath string) bool {
	for _, segment := range strings.Split(fullPath, ":") {
		lower := strings.ToLower(segment)
		for _, marker := range moneyMarkers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

// InDebtorBranch сообщает, лежит ли счёт в ветке дебиторской задолженности.
func InDebtorBranch(fullPath string) bool {
	return fullPath == DebtorBranch || strings.HasPrefix(fullPath, DebtorBranch+":")
}
