package httpui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// entryOp описывает одну операцию MVP на экране ввода: две стороны формы и
// роли строк. Сторону DEBIT/CREDIT здесь задавать нельзя — её назначает
// движок по матрице ASL-2.2.
type entryOp struct {
	Type   model.OpType
	Title  string
	LabelA string
	RoleA  engine.Role
	LabelB string
	RoleB  engine.Role
}

// entryOps — операции, доступные на экране ввода.
var entryOps = []entryOp{
	{
		Type: model.OpExpense, Title: "Трата",
		LabelA: "Статьи расходов", RoleA: engine.RoleExpense,
		LabelB: "Источники средств", RoleB: engine.RoleSource,
	},
	{
		Type: model.OpIncome, Title: "Доход",
		LabelA: "Поступления на актив", RoleA: engine.RoleReceipt,
		LabelB: "Статьи доходов", RoleB: engine.RoleIncome,
	},
	{
		Type: model.OpTransfer, Title: "Перевод",
		LabelA: "Откуда списать", RoleA: engine.RoleFrom,
		LabelB: "Куда зачислить", RoleB: engine.RoleTo,
	},
}

// entryOpByType возвращает описание операции MVP.
func entryOpByType(op model.OpType) (entryOp, bool) {
	for _, item := range entryOps {
		if item.Type == op {
			return item, true
		}
	}
	return entryOp{}, false
}

// entryRow — строка формы ввода (восстанавливается при ошибке).
type entryRow struct {
	AccountID   int64
	AccountPath string
	Amount      string
}

// entryView — данные страницы ввода проводки.
type entryView struct {
	Ops         []entryOp
	Users       []model.User
	Selected    model.OpType
	Current     entryOp
	Date        string
	Description string
	ActorID     int64
	RowsA       []entryRow
	RowsB       []entryRow
	Error       string
	// RolesJSON — карта «тип операции → роли сторон» для скрипта формы.
	RolesJSON json.RawMessage
}

// handleEntryForm показывает форму ввода проводки.
func (s *Server) handleEntryForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	selected := model.OpExpense
	if op := r.URL.Query().Get("op"); op != "" {
		if _, ok := entryOpByType(model.OpType(op)); ok {
			selected = model.OpType(op)
		}
	}

	view := entryView{
		Ops:      entryOps,
		Users:    users,
		Selected: selected,
		Date:     time.Now().Format("2006-01-02"),
		ActorID:  users[0].ID,
		RowsA:    []entryRow{{}},
		RowsB:    []entryRow{{}},
	}
	view.Current, _ = entryOpByType(selected)
	view.RolesJSON = rolesJSON()
	s.render(w, r, "entry", pageData{Title: "Ввод", Active: "entry", Data: view})
}

// handleEntrySubmit сохраняет проводку через движок.
func (s *Server) handleEntrySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(users) == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	opType := model.OpType(r.FormValue("op_type"))
	op, ok := entryOpByType(opType)
	if !ok {
		s.entryFailure(w, users, "выберите тип операции", nil, nil)
		return
	}

	view := entryView{
		Ops:         entryOps,
		Users:       users,
		Selected:    opType,
		Date:        r.FormValue("date"),
		Description: strings.TrimSpace(r.FormValue("description")),
		ActorID:     parseID(r.FormValue("actor_id"), users[0].ID),
	}
	view.Current = op
	view.RolesJSON = rolesJSON()

	entriesA, rowsA, err := collectEntries(r.PostForm, "a", op.RoleA)
	if err != nil {
		view.RowsA, view.RowsB = rowsA, nil
		s.entryFailureView(w, view, err.Error())
		return
	}
	entriesB, rowsB, err := collectEntries(r.PostForm, "b", op.RoleB)
	if err != nil {
		view.RowsA, view.RowsB = rowsA, rowsB
		s.entryFailureView(w, view, err.Error())
		return
	}
	view.RowsA, view.RowsB = rowsA, rowsB

	paths, err := s.accountPaths(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	for i := range view.RowsA {
		view.RowsA[i].AccountPath = accountName(paths, view.RowsA[i].AccountID)
	}
	for i := range view.RowsB {
		view.RowsB[i].AccountPath = accountName(paths, view.RowsB[i].AccountID)
	}

	date, err := parseDate(view.Date)
	if err != nil {
		s.entryFailureView(w, view, err.Error())
		return
	}

	operation := engine.Operation{
		OpType:      opType,
		UUID:        newUUID(),
		Date:        date,
		Description: view.Description,
		Source:      "web",
		Entries:     append(entriesA, entriesB...),
	}
	if _, err := s.engine.Post(ctx, operation, view.ActorID); err != nil {
		s.entryFailureView(w, view, statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/", "Проводка добавлена в журнал")
}

// entryFailure перерисовывает форму без сохранённых строк (общая ошибка).
func (s *Server) entryFailure(w http.ResponseWriter, users []model.User, message string, rowsA, rowsB []entryRow) {
	view := entryView{Ops: entryOps, Users: users, Selected: model.OpExpense, Date: time.Now().Format("2006-01-02"), ActorID: users[0].ID}
	view.RolesJSON = rolesJSON()
	view.RowsA, view.RowsB = rowsA, rowsB
	s.entryFailureView(w, view, message)
}

// entryFailureView показывает форму с сообщением об ошибке и кодом 422.
func (s *Server) entryFailureView(w http.ResponseWriter, view entryView, message string) {
	if view.RolesJSON == nil {
		view.RolesJSON = rolesJSON()
	}
	if len(view.RowsA) == 0 {
		view.RowsA = []entryRow{{}}
	}
	if len(view.RowsB) == 0 {
		view.RowsB = []entryRow{{}}
	}
	view.Error = message
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	data := pageData{Title: "Ввод", Active: "entry", Version: s.cfg.Version, Data: view}
	if err := s.pages["entry"].ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать форму ввода", "err", err)
	}
}

// collectEntries собирает строки операции из формы.
//
// Ключи формы: <prefix>_account, <prefix>_amount. Полностью пустые строки
// пропускаются; частично заполненные отклоняются с понятным сообщением.
// Повтор одного счёта на одной стороне складывается в одну строку: в проводке
// не может быть двух строк по одному счёту одной стороны.
func collectEntries(form url.Values, prefix string, role engine.Role) ([]engine.Entry, []entryRow, error) {
	accounts := form[prefix+"_account"]
	amounts := form[prefix+"_amount"]

	var entries []engine.Entry
	var rows []entryRow
	index := make(map[int64]int)
	for i := range accounts {
		idRaw := strings.TrimSpace(accounts[i])
		amountRaw := ""
		if i < len(amounts) {
			amountRaw = strings.TrimSpace(amounts[i])
		}
		if idRaw == "" && amountRaw == "" {
			continue
		}
		if idRaw == "" {
			return nil, rows, errors.New("выберите счёт для строки с суммой " + amountRaw)
		}
		if amountRaw == "" {
			return nil, rows, errors.New("укажите сумму для выбранного счёта")
		}
		id, err := strconv.ParseInt(idRaw, 10, 64)
		if err != nil || id <= 0 {
			return nil, rows, errors.New("не удалось определить выбранный счёт")
		}
		amount, err := parseAmount(amountRaw)
		if err != nil {
			return nil, rows, err
		}
		if at, ok := index[id]; ok {
			entries[at].Amount += amount
			continue
		}
		index[id] = len(entries)
		entries = append(entries, engine.Entry{AccountID: id, Amount: amount, Role: role})
		rows = append(rows, entryRow{AccountID: id, Amount: amountRaw})
	}
	return entries, rows, nil
}

// handleSuggestJSON подсказывает счета для помощника ввода.
func (s *Server) handleSuggestJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 12
	}

	accounts, err := s.db.SuggestAccounts(ctx, query, limit)
	if err != nil {
		s.fail(w, err)
		return
	}

	opType, role := r.URL.Query().Get("op_type"), r.URL.Query().Get("role")
	var rule engine.Rule
	hasRule := false
	if opType != "" {
		if parsed, ok := engine.RuleFor(model.OpType(opType)); ok {
			rule, hasRule = parsed, true
		}
	}
	var side model.Side
	hasSide := false
	if role != "" {
		side, hasSide = roleSide(engine.Role(role))
	}

	items := make([]map[string]any, 0, len(accounts))
	for _, acc := range accounts {
		usable := acc.IsLeaf
		var cell string
		if hasRule && hasSide {
			if containsSide(rule, side, acc.Type) {
				cell = engine.CellName(model.OpType(opType), side, acc.Type)
			} else {
				usable = false
			}
		} else if hasRule {
			if _, allowed := rule.Allows(acc.Type); !allowed {
				usable = false
			}
		}
		items = append(items, map[string]any{
			"id":          acc.ID,
			"path":        acc.FullPath,
			"type":        string(acc.Type),
			"is_leaf":     acc.IsLeaf,
			"usable":      usable,
			"cell":        cell,
			"reserved":    hasRule && rule.Reserved(acc.Type),
			"min_balance": acc.MinBalance.String(),
		})
	}
	writeWebJSON(w, map[string]any{"items": items})
}

// handleMatrixJSON проверяет допустимость счёта для операции и роли и
// возвращает ячейку матрицы для подсветки.
func (s *Server) handleMatrixJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opType := model.OpType(r.URL.Query().Get("op_type"))
	role := engine.Role(r.URL.Query().Get("role"))

	rule, ok := engine.RuleFor(opType)
	if !ok {
		writeWebJSON(w, map[string]any{"allowed": false, "message": "неизвестный тип операции"})
		return
	}
	side, ok := roleSide(role)
	if !ok {
		writeWebJSON(w, map[string]any{"allowed": false, "message": "неизвестная роль строки"})
		return
	}

	id, err := strconv.ParseInt(r.URL.Query().Get("account_id"), 10, 64)
	if err != nil || id <= 0 {
		writeWebJSON(w, map[string]any{"allowed": false, "side": side, "message": "счёт не выбран"})
		return
	}

	acc, err := s.db.Account(ctx, id)
	if err != nil {
		writeWebJSON(w, map[string]any{"allowed": false, "side": side, "message": "счёт не найден"})
		return
	}

	allowed := containsSide(rule, side, acc.Type)
	cell := engine.CellName(opType, side, acc.Type)
	reserved := rule.Reserved(acc.Type)
	message := ""
	if !allowed {
		if reserved {
			message = fmt.Sprintf("Счёт «%s» (тип «%s») зарезервирован для этой операции и появится позже: ячейка %s", acc.FullPath, acc.Type, cell)
		} else {
			message = fmt.Sprintf("Счёт «%s» имеет тип «%s», недопустимый для роли «%s»: ячейка %s", acc.FullPath, acc.Type, role, cell)
		}
	}
	writeWebJSON(w, map[string]any{
		"allowed":  allowed,
		"side":     string(side),
		"cell":     cell,
		"reserved": reserved,
		"path":     acc.FullPath,
		"message":  message,
	})
}

// containsSide сообщает, входит ли тип счёта в допустимые типы указанной
// стороны строки матрицы.
func containsSide(rule engine.Rule, side model.Side, t model.AccountType) bool {
	if side == model.SideDebit {
		return containsType(rule.Debit, t)
	}
	return containsType(rule.Credit, t)
}

// containsType проверяет наличие типа счёта в списке.
func containsType(types []model.AccountType, t model.AccountType) bool {
	for _, item := range types {
		if item == t {
			return true
		}
	}
	return false
}

// roleSide возвращает сторону строки матрицы для семантической роли.
func roleSide(role engine.Role) (model.Side, bool) {
	switch role {
	case engine.RoleExpense, engine.RoleReceipt, engine.RoleTo, engine.RoleDebtor:
		return model.SideDebit, true
	case engine.RoleSource, engine.RoleIncome, engine.RoleFrom, engine.RoleLiability:
		return model.SideCredit, true
	case engine.RoleAsset:
		return model.SideDebit, true
	case engine.RoleAdjust:
		return model.SideDebit, true
	default:
		return "", false
	}
}

// rolesJSON отдаёт карту «тип операции → роли сторон» для скрипта формы.
func rolesJSON() json.RawMessage {
	ops := make([]map[string]any, 0, len(entryOps))
	for _, op := range entryOps {
		sideA, _ := roleSide(op.RoleA)
		sideB, _ := roleSide(op.RoleB)
		ops = append(ops, map[string]any{
			"type":   string(op.Type),
			"title":  op.Title,
			"labelA": op.LabelA,
			"labelB": op.LabelB,
			"roleA":  string(op.RoleA),
			"roleB":  string(op.RoleB),
			"sideA":  string(sideA),
			"sideB":  string(sideB),
		})
	}
	data, err := json.Marshal(map[string]any{"ops": ops})
	if err != nil {
		return json.RawMessage(`{"ops":[]}`)
	}
	return data
}

// parseAmount разбирает сумму в рублях («1 234,56», «1234.5») в копейки без
// чисел с плавающей точкой.
func parseAmount(raw string) (money.Kop, error) {
	s := strings.TrimSpace(raw)
	s = strings.NewReplacer("\u00a0", "", " ", "", "₽", "", "р.", "", "руб.", "").Replace(s)
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, errors.New("не задана сумма")
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if frac == "" {
		frac = "0"
	}
	if len(frac) > 2 {
		return 0, errors.New("в сумме больше двух знаков после запятой")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if !allDigits(whole) || !allDigits(frac) {
		return 0, errors.New("сумма должна быть числом, например «1 234,56»")
	}
	rub, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, errors.New("сумма слишком велика")
	}
	kop, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errors.New("сумма слишком велика")
	}
	value := money.Kop(rub*100 + kop)
	if negative {
		value = -value
	}
	if value <= 0 {
		return 0, errors.New("сумма должна быть больше нуля")
	}
	return value, nil
}

// allDigits сообщает, что строка состоит только из цифр.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseDate разбирает дату формы (ГГГГ-ММ-ДД).
func parseDate(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Now(), nil
	}
	date, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, errors.New("неверная дата, ожидается формат ГГГГ-ММ-ДД")
	}
	return date, nil
}

// parseID разбирает идентификатор, возвращая запасное значение при ошибке.
func parseID(raw string, fallback int64) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return fallback
	}
	return id
}

// newUUID возвращает новый UUID версии 4 для проводки.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "web-" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// writeWebJSON отдаёт JSON-ответ web-интерфейса (всегда 200).
//
// Ошибки проверки помощника возвращаются в самом теле ответа полем allowed,
// чтобы форма могла показать пояснение, а не полагаться на код HTTP.
func writeWebJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}
