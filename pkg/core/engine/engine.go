package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// Пути служебных счетов, строки которых генерирует движок.
const (
	// InitialCapitalPath — строка капитала для начальных остатков.
	InitialCapitalPath = "Капитал:Начальные"
	// RevaluationCapitalPath — строка капитала для переоценки.
	RevaluationCapitalPath = "Капитал:Переоценка"
	// AdjustmentCapitalPath — строка капитала для корректировки.
	AdjustmentCapitalPath = "Капитал:Корректировка"
)

// Engine — движок проводок: собирает строки по матрице, проверяет инварианты
// и сохраняет проводку через слой данных.
type Engine struct {
	db  *store.DB
	now func() time.Time
}

// New создаёт движок поверх книги.
func New(db *store.DB) *Engine {
	return &Engine{db: db, now: time.Now}
}

// Direction — направление движения счёта пользователя.
type Direction string

// Направления для переоценки и корректировки.
const (
	// DirectionNone — направление не задано.
	DirectionNone Direction = ""
	// DirectionUp — счёт увеличивается.
	DirectionUp Direction = "вверх"
	// DirectionDown — счёт уменьшается.
	DirectionDown Direction = "вниз"
)

// Entry — строка запроса: счёт, положительная сумма и роль. Сторону строки
// здесь задать нельзя — её назначает движок.
type Entry struct {
	// AccountID — счёт строки.
	AccountID int64
	// Amount — положительная сумма в копейках.
	Amount money.Kop
	// Role — семантическая роль строки.
	Role Role
}

// Operation — запрос пользователя на проводку.
type Operation struct {
	// OpType — тип операции.
	OpType model.OpType
	// UUID — уникальный идентификатор проводки (генерирует клиент).
	UUID string
	// Date — дата операции.
	Date time.Time
	// Description — описание.
	Description string
	// Source — источник ('web', 'api:<client>', 'hledger').
	Source string
	// SourceHash — хеш содержимого строки импорта.
	SourceHash string
	// Direction — направление для переоценки и корректировки.
	Direction Direction
	// Entries — строки операции.
	Entries []Entry
}

// Prepared — результат подготовки проводки без записи.
type Prepared struct {
	// Transaction — собранная и проверенная проводка.
	Transaction *model.Transaction
	// Existing — проводка с таким uuid уже есть в книге (идемпотентность).
	Existing bool
}

// Prepare собирает и проверяет проводку, не записывая её.
//
// Используется одиночной записью (Post) и пакетным коммитом синхронизации:
// пакет проверяет все операции через Prepare, а записывает их одной
// транзакцией БД. Если uuid уже существует, возвращается готовая проводка с
// признаком Existing — повтор не создаёт дубль.
func (e *Engine) Prepare(ctx context.Context, op Operation, actorID int64) (*Prepared, error) {
	if !op.OpType.Valid() {
		return nil, NewError(CodeOpTypeUnknown, "неизвестный тип операции «"+string(op.OpType)+"»")
	}
	if strings.TrimSpace(op.UUID) == "" {
		return nil, NewError(CodeUUIDEmpty, "не задан uuid проводки")
	}

	existing, err := e.db.TransactionByUUID(ctx, op.UUID)
	switch {
	case err == nil:
		return &Prepared{Transaction: existing, Existing: true}, nil
	case !errors.Is(err, store.ErrNotFound):
		return nil, err
	}

	if op.OpType == model.OpInitial {
		if err := e.ensureInitialAllowed(ctx); err != nil {
			return nil, err
		}
	}

	lines, err := e.buildLines(ctx, op)
	if err != nil {
		return nil, err
	}

	tr := &model.Transaction{
		UUID:        op.UUID,
		OpType:      op.OpType,
		Date:        op.Date,
		Description: op.Description,
		ActorID:     actorID,
		Source:      op.Source,
		SourceHash:  op.SourceHash,
		Lines:       lines,
	}
	if err := e.validate(ctx, tr, true); err != nil {
		return nil, err
	}
	return &Prepared{Transaction: tr}, nil
}

// MarkInitialApplied фиксирует, что начальные остатки внесены.
//
// Флаг запрещает повторный ввод стартовых остатков (критерий эпика).
func (e *Engine) MarkInitialApplied(ctx context.Context) error {
	return e.db.SetSetting(ctx, store.SettingInitialBalances, "1")
}

// Post собирает, проверяет и сохраняет проводку.
//
// Повтор с тем же uuid идемпотентен: возвращается уже существующая проводка
// с тем же ответом.
func (e *Engine) Post(ctx context.Context, op Operation, actorID int64) (*model.Transaction, error) {
	prepared, err := e.Prepare(ctx, op, actorID)
	if err != nil {
		return nil, err
	}
	if prepared.Existing {
		return prepared.Transaction, nil
	}

	id, err := e.db.InsertTransaction(ctx, prepared.Transaction)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if op.OpType == model.OpInitial {
		if err := e.MarkInitialApplied(ctx); err != nil {
			return nil, err
		}
	}
	return e.db.Transaction(ctx, id)
}

// buildLines назначает стороны строкам операции по матрице правил.
func (e *Engine) buildLines(ctx context.Context, op Operation) ([]model.TransactionLine, error) {
	switch op.OpType {
	case model.OpInitial:
		return e.buildInitial(ctx, op)
	case model.OpRevaluation, model.OpAdjustment:
		return e.buildDirectional(ctx, op)
	default:
		return e.buildByRoles(ctx, op)
	}
}

// buildByRoles собирает строки операций с фиксированными сторонами ролей.
func (e *Engine) buildByRoles(ctx context.Context, op Operation) ([]model.TransactionLine, error) {
	specs, ok := roleSpecs[op.OpType]
	if !ok {
		return nil, NewError(CodeOpTypeUnknown, "операция «"+string(op.OpType)+"» не поддерживает роли")
	}
	rule, _ := RuleFor(op.OpType)
	if len(op.Entries) == 0 {
		return nil, NewError(CodeEntryMissing, "не заданы строки операции")
	}

	lines := make([]model.TransactionLine, 0, len(op.Entries))
	for _, entry := range op.Entries {
		spec, ok := specs[entry.Role]
		if !ok {
			return nil, NewError(CodeRoleUnknown,
				fmt.Sprintf("роль «%s» недопустима для операции «%s»", entry.Role, op.OpType))
		}
		if err := checkAmount(entry.Amount); err != nil {
			return nil, err
		}
		acc, err := e.account(ctx, entry.AccountID)
		if err != nil {
			return nil, err
		}
		if !containsType(spec.types, acc.Type) {
			if rule.Reserved(acc.Type) {
				return nil, NewError(CodeReservedCell,
					fmt.Sprintf("операция «%s» с типом счёта «%s» зарезервирована под post-MVP", op.OpType, acc.Type)).
					WithAccount(acc.FullPath).WithCell(CellName(op.OpType, spec.side, acc.Type))
			}
			return nil, NewError(CodeMatrixCell,
				fmt.Sprintf("тип счёта «%s» недопустим на стороне %s операции «%s»", acc.Type, spec.side, op.OpType)).
				WithAccount(acc.FullPath).WithCell(CellName(op.OpType, spec.side, acc.Type))
		}
		if op.OpType == model.OpLoan && entry.Role == RoleDebtor && !InDebtorBranch(acc.FullPath) {
			return nil, NewError(CodeMatrixCell,
				"выдача займа допускается только на счёт ветки «"+DebtorBranch+"»").WithAccount(acc.FullPath)
		}
		lines = append(lines, model.TransactionLine{AccountID: acc.ID, Amount: entry.Amount, Side: spec.side})
	}
	return lines, nil
}

// buildInitial собирает начальные остатки и генерирует строку капитала.
func (e *Engine) buildInitial(ctx context.Context, op Operation) ([]model.TransactionLine, error) {
	if len(op.Entries) == 0 {
		return nil, NewError(CodeEntryMissing, "не заданы строки начальных остатков")
	}

	var debitTotal, creditTotal money.Kop
	lines := make([]model.TransactionLine, 0, len(op.Entries)+1)
	for _, entry := range op.Entries {
		if err := checkAmount(entry.Amount); err != nil {
			return nil, err
		}
		acc, err := e.account(ctx, entry.AccountID)
		if err != nil {
			return nil, err
		}
		if acc.Type == model.TypeCapital {
			return nil, NewError(CodeSystemAccount,
				"строка капитала в начальных остатках генерируется движком автоматически").WithAccount(acc.FullPath)
		}
		switch entry.Role {
		case RoleAsset:
			if acc.Type != model.TypeAsset {
				return nil, matrixConflict(op.OpType, model.SideDebit, acc)
			}
			lines = append(lines, model.TransactionLine{AccountID: acc.ID, Amount: entry.Amount, Side: model.SideDebit})
			debitTotal += entry.Amount
		case RoleLiability:
			if acc.Type != model.TypeLiability {
				return nil, matrixConflict(op.OpType, model.SideCredit, acc)
			}
			lines = append(lines, model.TransactionLine{AccountID: acc.ID, Amount: entry.Amount, Side: model.SideCredit})
			creditTotal += entry.Amount
		default:
			return nil, NewError(CodeRoleUnknown,
				fmt.Sprintf("роль «%s» недопустима для начальных остатков", entry.Role))
		}
	}

	// Капитал = Σактивов − Σобязательств: положительный — CREDIT, отрицательный — DEBIT.
	diff := debitTotal - creditTotal
	if diff != 0 {
		capital, err := e.db.AccountByPath(ctx, InitialCapitalPath)
		if err != nil {
			return nil, NewError(CodeAccountNotFound, "служебный счёт «"+InitialCapitalPath+"» не найден").WithCause(err)
		}
		side := model.SideCredit
		if diff < 0 {
			side = model.SideDebit
			diff = -diff
		}
		lines = append(lines, model.TransactionLine{AccountID: capital.ID, Amount: diff, Side: side})
	}
	return lines, nil
}

// buildDirectional собирает переоценку и корректировку: одна строка пользователя
// и служебный счёт капитала с противоположной стороны.
func (e *Engine) buildDirectional(ctx context.Context, op Operation) ([]model.TransactionLine, error) {
	if len(op.Entries) != 1 {
		return nil, NewError(CodeEntryMissing,
			fmt.Sprintf("операция «%s» принимает ровно одну строку пользователя", op.OpType))
	}
	if op.Direction != DirectionUp && op.Direction != DirectionDown {
		return nil, NewError(CodeDirectionRequired,
			"для операции «"+string(op.OpType)+"» нужно задать направление (вверх или вниз)")
	}

	entry := op.Entries[0]
	if err := checkAmount(entry.Amount); err != nil {
		return nil, err
	}
	acc, err := e.account(ctx, entry.AccountID)
	if err != nil {
		return nil, err
	}

	var (
		side        model.Side
		servicePath string
	)
	switch op.OpType {
	case model.OpRevaluation:
		servicePath = RevaluationCapitalPath
		if acc.Type != model.TypeAsset {
			return nil, matrixConflict(op.OpType, model.SideDebit, acc)
		}
		if IsMoneyAccount(acc.FullPath) {
			return nil, NewError(CodeMatrixCell,
				"денежный счёт нельзя переоценивать — используйте корректировку").WithAccount(acc.FullPath)
		}
	case model.OpAdjustment:
		servicePath = AdjustmentCapitalPath
		if acc.Type == model.TypeIncome || acc.Type == model.TypeExpense {
			return nil, matrixConflict(op.OpType, model.SideDebit, acc)
		}
	default:
		return nil, NewError(CodeOpTypeUnknown, "операция «"+string(op.OpType)+"» не поддерживает направление")
	}

	if op.Direction == DirectionUp {
		side, err = IncreaseSide(acc.Type)
	} else {
		side, err = DecreaseSide(acc.Type)
	}
	if err != nil {
		return nil, err
	}

	service, err := e.db.AccountByPath(ctx, servicePath)
	if err != nil {
		return nil, NewError(CodeAccountNotFound, "служебный счёт «"+servicePath+"» не найден").WithCause(err)
	}
	return []model.TransactionLine{
		{AccountID: acc.ID, Amount: entry.Amount, Side: side},
		{AccountID: service.ID, Amount: entry.Amount, Side: side.Opposite()},
	}, nil
}

// validate проверяет сквозные инварианты проводки перед записью.
//
// checkMatrix включает проверку ячеек матрицы; для сторно она отключается, так
// как сторно — инверсия уже принятой операции.
func (e *Engine) validate(ctx context.Context, tr *model.Transaction, checkMatrix bool) error {
	if len(tr.Lines) == 0 {
		return NewError(CodeEntryMissing, "в проводке нет строк")
	}
	rule, ok := RuleFor(tr.OpType)
	if checkMatrix && !ok {
		return NewError(CodeOpTypeUnknown, "неизвестный тип операции «"+string(tr.OpType)+"»")
	}

	accounts := make(map[int64]*model.Account, len(tr.Lines))
	var (
		debit, credit   money.Kop
		debitN, creditN int
	)
	for _, line := range tr.Lines {
		if err := checkAmount(line.Amount); err != nil {
			return err
		}
		if !line.Side.Valid() {
			return NewError(CodeMatrixCell, "недопустимая сторона строки «"+string(line.Side)+"»")
		}
		acc, ok := accounts[line.AccountID]
		if !ok {
			var err error
			acc, err = e.account(ctx, line.AccountID)
			if err != nil {
				return err
			}
			accounts[line.AccountID] = acc
		}
		if acc.IsArchived {
			return NewError(CodeAccountArchived, "счёт заархивирован").WithAccount(acc.FullPath)
		}
		if !acc.IsLeaf {
			return NewError(CodeAccountNotLeaf,
				"счёт — папка, выберите листовой счёт").WithAccount(acc.FullPath)
		}
		if checkMatrix {
			if _, ok := rule.sideOfType(acc.Type); !ok {
				if rule.Reserved(acc.Type) {
					return NewError(CodeReservedCell,
						fmt.Sprintf("операция «%s» с типом счёта «%s» зарезервирована под post-MVP", tr.OpType, acc.Type)).
						WithAccount(acc.FullPath).WithCell(CellName(tr.OpType, line.Side, acc.Type))
				}
				return NewError(CodeMatrixCell,
					fmt.Sprintf("тип счёта «%s» недопустим для операции «%s»", acc.Type, tr.OpType)).
					WithAccount(acc.FullPath).WithCell(CellName(tr.OpType, line.Side, acc.Type))
			}
		}
		if line.Side == model.SideDebit {
			debit += line.Amount
			debitN++
		} else {
			credit += line.Amount
			creditN++
		}
	}

	if debit != credit {
		return NewError(CodeUnbalanced,
			fmt.Sprintf("дебет %d коп. не равен кредиту %d коп.", debit.Kopecks(), credit.Kopecks()))
	}
	if checkMatrix {
		if debitN < rule.MinDebit {
			return NewError(CodeEntryMissing,
				fmt.Sprintf("операция «%s» требует минимум %d строк дебета", tr.OpType, rule.MinDebit))
		}
		if creditN < rule.MinCredit {
			return NewError(CodeEntryMissing,
				fmt.Sprintf("операция «%s» требует минимум %d строк кредита", tr.OpType, rule.MinCredit))
		}
	}
	if err := e.validateDate(tr, accounts); err != nil {
		return err
	}
	return e.validateMinBalance(ctx, tr, accounts)
}

// validateDate проверяет диапазон даты: не раньше даты создания счёта и не
// больше «завтра».
func (e *Engine) validateDate(tr *model.Transaction, accounts map[int64]*model.Account) error {
	if tr.Date.IsZero() {
		return NewError(CodeDateRange, "не задана дата проводки")
	}
	now := e.now().UTC()
	tomorrowEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 2)
	if tr.Date.After(tomorrowEnd) {
		return NewError(CodeDateRange, "дата проводки больше «завтра»")
	}
	for _, acc := range accounts {
		created := acc.CreatedAt.UTC()
		createdDay := time.Date(created.Year(), created.Month(), created.Day(), 0, 0, 0, 0, time.UTC)
		if tr.Date.Before(createdDay) {
			return NewError(CodeDateRange, "дата проводки раньше даты создания счёта").WithAccount(acc.FullPath)
		}
	}
	return nil
}

// validateMinBalance проверяет остаток относительно min_balance в лимитном
// режиме контроля остатка (раздел 9.2). В свободном режиме проверка пропускается.
func (e *Engine) validateMinBalance(ctx context.Context, tr *model.Transaction, accounts map[int64]*model.Account) error {
	mode, ok, err := e.db.Setting(ctx, store.SettingBalanceMode)
	if err != nil {
		return err
	}
	if !ok || mode != store.BalanceModeLimited {
		return nil
	}

	deltas := map[int64]money.Kop{}
	for _, line := range tr.Lines {
		acc := accounts[line.AccountID]
		increase, err := IncreaseSide(acc.Type)
		if err != nil {
			return err
		}
		delta := line.Amount
		if line.Side != increase {
			delta = -delta
		}
		deltas[acc.ID] += delta
	}

	for id, delta := range deltas {
		acc := accounts[id]
		current, err := e.db.Balance(ctx, id)
		if err != nil {
			return err
		}
		if next := current + delta; next < acc.MinBalance {
			return NewError(CodeMinBalance,
				fmt.Sprintf("остаток станет %s, ниже минимума %s", next, acc.MinBalance)).
				WithAccount(acc.FullPath).WithAmount(delta)
		}
	}
	return nil
}

// ensureInitialAllowed проверяет, что начальные остатки ещё не введены.
func (e *Engine) ensureInitialAllowed(ctx context.Context) error {
	value, ok, err := e.db.Setting(ctx, store.SettingInitialBalances)
	if err != nil {
		return err
	}
	if ok && value == "1" {
		return NewError(CodeInitialDone, "начальные остатки уже введены")
	}
	return nil
}

// account читает счёт по идентификатору, переводя ошибку слоя данных в код движка.
func (e *Engine) account(ctx context.Context, id int64) (*model.Account, error) {
	acc, err := e.db.Account(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, NewError(CodeAccountNotFound, fmt.Sprintf("счёт %d не найден", id)).WithCause(err)
	}
	return acc, err
}

// checkAmount проверяет, что сумма строки строго положительна.
func checkAmount(amount money.Kop) error {
	if amount <= 0 {
		return NewError(CodeAmountNotPositive,
			fmt.Sprintf("сумма %d коп. должна быть положительной", amount.Kopecks())).WithAmount(amount)
	}
	return nil
}

// matrixConflict собирает ошибку недопустимой ячейки матрицы.
func matrixConflict(op model.OpType, side model.Side, acc *model.Account) error {
	return NewError(CodeMatrixCell,
		fmt.Sprintf("тип счёта «%s» недопустим на стороне %s операции «%s»", acc.Type, side, op)).
		WithAccount(acc.FullPath).WithCell(CellName(op, side, acc.Type))
}

// mapStoreError переводит ошибки слоя данных в коды движка.
func mapStoreError(err error) error {
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		return &Error{Code: ve.Code, Message: ve.Message, Err: err}
	}
	if errors.Is(err, store.ErrDuplicateUUID) {
		return NewError(CodeUUIDDup, "проводка с таким uuid уже существует").WithCause(err)
	}
	return err
}
