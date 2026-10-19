package report

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// fixture — книга для тестов отчётов.
type fixture struct {
	db      *store.DB
	actor1  int64
	actor2  int64
	janFrom int64
	janTo   int64
}

// newFixture собирает книгу: план счетов и пять проводок за январь-февраль.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()

	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "book.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	user1, err := db.CreateUser(ctx, "Андрей", "секрет-1")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	user2, err := db.CreateUser(ctx, "Мария", "секрет-2")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	type spec struct {
		name       string
		typ        model.AccountType
		parentPath string
	}
	accounts := []spec{
		{"Наличные", model.TypeAsset, "Активы"},
		{"Карта", model.TypeAsset, "Активы"},
		{"Обязательства", model.TypeLiability, ""},
		{"Кредит", model.TypeLiability, "Обязательства"},
		{"Доходы", model.TypeIncome, ""},
		{"Зарплата", model.TypeIncome, "Доходы"},
		{"Расходы", model.TypeExpense, ""},
		{"Продукты", model.TypeExpense, "Расходы"},
		{"Кафе", model.TypeExpense, "Расходы"},
	}
	ids := map[string]int64{}
	existing, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	for i := range existing {
		ids[existing[i].FullPath] = existing[i].ID
	}
	for _, s := range accounts {
		var parent *int64
		if s.parentPath != "" {
			id := ids[s.parentPath]
			parent = &id
		}
		acc, err := db.CreateAccount(ctx, store.AccountInput{Name: s.name, Type: s.typ, ParentID: parent})
		if err != nil {
			t.Fatalf("CreateAccount(%q): %v", s.name, err)
		}
		ids[acc.FullPath] = acc.ID
	}

	insert := func(uuid string, date time.Time, op model.OpType, actor int64, reverses *int64, lines ...model.TransactionLine) int64 {
		t.Helper()
		id, err := db.InsertTransaction(ctx, &model.Transaction{
			UUID: uuid, OpType: op, Date: date, ActorID: actor, ReversesID: reverses, Lines: lines,
		})
		if err != nil {
			t.Fatalf("InsertTransaction(%s): %v", uuid, err)
		}
		return id
	}
	line := func(path string, amount money.Kop, side model.Side) model.TransactionLine {
		return model.TransactionLine{AccountID: ids[path], Amount: amount, Side: side}
	}
	date := func(day int) time.Time { return time.Date(2026, time.January, day, 0, 0, 0, 0, time.UTC) }

	insert("t1", date(15), model.OpExpense, user1.ID, nil,
		line("Расходы:Продукты", money.FromKopecks(100_000), model.SideDebit),
		line("Активы:Наличные", money.FromKopecks(100_000), model.SideCredit))
	insert("t2", date(16), model.OpIncome, user1.ID, nil,
		line("Активы:Карта", money.FromKopecks(5_000_000), model.SideDebit),
		line("Доходы:Зарплата", money.FromKopecks(5_000_000), model.SideCredit))
	insert("t3", date(17), model.OpExpense, user2.ID, nil,
		line("Расходы:Кафе", money.FromKopecks(20_000), model.SideDebit),
		line("Активы:Карта", money.FromKopecks(20_000), model.SideCredit))
	insert("t4", time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC), model.OpIncome, user1.ID, nil,
		line("Активы:Карта", money.FromKopecks(1_000_000), model.SideDebit),
		line("Доходы:Зарплата", money.FromKopecks(1_000_000), model.SideCredit))
	txn6 := insert("t6", date(18), model.OpExpense, user1.ID, nil,
		line("Расходы:Кафе", money.FromKopecks(100_000), model.SideDebit),
		line("Активы:Наличные", money.FromKopecks(100_000), model.SideCredit))
	insert("t5", date(18), model.OpAdjustment, user1.ID, &txn6,
		line("Активы:Наличные", money.FromKopecks(100_000), model.SideDebit),
		line("Расходы:Кафе", money.FromKopecks(100_000), model.SideCredit))

	return &fixture{
		db:      db,
		actor1:  user1.ID,
		actor2:  user2.ID,
		janFrom: date(1).Unix(),
		janTo:   time.Date(2026, time.January, 31, 23, 59, 59, 0, time.UTC).Unix(),
	}
}

// TestASL_11_BalancesReport проверяет отчёт об остатках с иерархическими итогами.
func TestASL_11_BalancesReport(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	rep, err := Balances(ctx, fx.db, Options{})
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}

	byPath := map[string]BalanceRow{}
	for _, row := range rep.Rows {
		byPath[row.Path] = row
	}
	if got := byPath["Активы"].Total; got != money.FromKopecks(5_880_000) {
		t.Errorf("итог «Активы»: %s, ожидалось 58 800,00 ₽", got)
	}
	if got := byPath["Активы:Наличные"].Own; got != money.FromKopecks(-100_000) {
		t.Errorf("остаток «Наличные»: %s", got)
	}
	if got := byPath["Активы:Карта"].Total; got != money.FromKopecks(5_980_000) {
		t.Errorf("остаток «Карта»: %s, ожидалось 59 800,00 ₽", got)
	}
	if byPath["Активы"].Depth != 0 || byPath["Активы:Карта"].Depth != 1 {
		t.Errorf("уровни вложенности: %d и %d", byPath["Активы"].Depth, byPath["Активы:Карта"].Depth)
	}
	// Иерархический итог совпадает со свёрткой журнала по поддереву.
	subtree, err := fx.db.SubtreeBalance(ctx, "Активы")
	if err != nil {
		t.Fatalf("SubtreeBalance: %v", err)
	}
	if rep.Assets != subtree {
		t.Errorf("итог активов %s не совпадает со свёрткой поддерева %s", rep.Assets, subtree)
	}
	if rep.Assets != money.FromKopecks(5_880_000) || rep.Liabilities != 0 {
		t.Errorf("итоги: активы %s, обязательства %s", rep.Assets, rep.Liabilities)
	}
	if rep.NetWorth() != money.FromKopecks(5_880_000) {
		t.Errorf("капитал: %s", rep.NetWorth())
	}
	// Валюта выводится как «₽».
	if got := byPath["Активы"].Total.String(); !strings.HasSuffix(got, "₽") {
		t.Errorf("сумма без обозначения рубля: %q", got)
	}

	// Отрицательный остаток помечен и попадает в список для подсветки.
	negatives := rep.Negatives()
	if len(negatives) != 1 || negatives[0].Path != "Активы:Наличные" {
		t.Errorf("подсветка минусов: %+v", negatives)
	}

	// Без IncludeEmpty пустые нелистовые ветки скрыты.
	if _, ok := byPath["Обязательства"]; ok && byPath["Обязательства"].Total == 0 {
		t.Error("ветка без остатка показана без IncludeEmpty")
	}
}

// TestASL_11_TurnoversReportByNode проверяет обороты за период по узлу дерева.
func TestASL_11_TurnoversReportByNode(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	rep, err := Turnovers(ctx, fx.db, Options{From: fx.janFrom, To: fx.janTo})
	if err != nil {
		t.Fatalf("Turnovers: %v", err)
	}
	byPath := map[string]TurnoverRow{}
	for _, row := range rep.Rows {
		byPath[row.Path] = row
	}
	var expensesDebit money.Kop
	for _, row := range rep.Rows {
		if row.Path == "Расходы" || strings.HasPrefix(row.Path, "Расходы:") {
			expensesDebit += row.Debit
		}
	}
	if expensesDebit != money.FromKopecks(220_000) {
		t.Errorf("оборот по дебету поддерева расходов: %s, ожидалось 2 200,00 ₽", expensesDebit)
	}
	if got := byPath["Расходы:Продукты"].Debit; got != money.FromKopecks(100_000) {
		t.Errorf("оборот по дебету продуктов: %s", got)
	}
	if got := byPath["Расходы:Кафе"].Debit; got != money.FromKopecks(120_000) {
		t.Errorf("оборот по дебету кафе: %s, ожидалось 1 200,00 ₽", got)
	}
	if got := byPath["Расходы:Кафе"].Credit; got != money.FromKopecks(100_000) {
		t.Errorf("оборот по кредиту кафе (сторно): %s", got)
	}
	if got := byPath["Активы:Карта"].Debit; got != money.FromKopecks(5_000_000) {
		t.Errorf("оборот по дебету карты: %s", got)
	}
	if got := byPath["Доходы:Зарплата"].Credit; got != money.FromKopecks(5_000_000) {
		t.Errorf("оборот по кредиту доходов: %s", got)
	}
	// Февральская проводка в январь не попала.
	if row, ok := byPath["Активы:Карта"]; ok && row.Credit != money.FromKopecks(20_000) {
		t.Errorf("кредит карты за январь: %s", row.Credit)
	}
	if got := byPath["Активы:Карта"].Closing; got != money.FromKopecks(4_980_000) {
		t.Errorf("остаток карты на конец января: %s, ожидалось 49 800,00 ₽", got)
	}

	// Отчёт по нелистовому узлу «Расходы» не содержит счетов вне поддерева.
	node, err := Turnovers(ctx, fx.db, Options{Node: "Расходы", From: fx.janFrom, To: fx.janTo})
	if err != nil {
		t.Fatalf("Turnovers(Расходы): %v", err)
	}
	for _, row := range node.Rows {
		if row.Path != "Расходы" && !strings.HasPrefix(row.Path, "Расходы:") {
			t.Errorf("в отчёт по узлу попал чужой счёт %q", row.Path)
		}
	}
	// Период полностью вне проводок пуст.
	empty, err := Turnovers(ctx, fx.db, Options{From: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC).Unix(),
		To: time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC).Unix()})
	if err != nil {
		t.Fatalf("Turnovers(пустой период): %v", err)
	}
	for _, row := range empty.Rows {
		if row.Movement() {
			t.Errorf("в пустом периоде есть движение по счёту %q", row.Path)
		}
	}
}

// TestASL_11_ExpensesByActor проверяет расходы по членам семьи.
func TestASL_11_ExpensesByActor(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	rep, err := ExpensesByActor(ctx, fx.db, Options{From: fx.janFrom, To: fx.janTo})
	if err != nil {
		t.Fatalf("ExpensesByActor: %v", err)
	}
	byActor := map[int64]ActorExpense{}
	for _, actor := range rep.Actors {
		byActor[actor.ActorID] = actor
	}
	// Сторно обнулило расходы по статье «Кафе» у Андрея, осталась «Продукты».
	if got := byActor[fx.actor1].Total; got != money.FromKopecks(100_000) {
		t.Errorf("расходы Андрея: %s, ожидалось 1 000,00 ₽", got)
	}
	if got := byActor[fx.actor2].Total; got != money.FromKopecks(20_000) {
		t.Errorf("расходы Марии: %s, ожидалось 200,00 ₽", got)
	}
	if byActor[fx.actor2].Name != "Мария" {
		t.Errorf("имя автора: %q", byActor[fx.actor2].Name)
	}
	if len(byActor[fx.actor2].Articles) != 1 || byActor[fx.actor2].Articles[0].Path != "Расходы:Кафе" {
		t.Errorf("разбивка по статьям: %+v", byActor[fx.actor2].Articles)
	}

	// Σ по членам семьи = общий расход за период = остаток счёта «Расходы».
	var sum money.Kop
	for _, actor := range rep.Actors {
		sum += actor.Total
	}
	if sum != rep.Total {
		t.Errorf("сумма по членам %s не равна итогу %s", sum, rep.Total)
	}
	balance, err := fx.db.SubtreeBalance(ctx, "Расходы")
	if err != nil {
		t.Fatalf("SubtreeBalance(Расходы): %v", err)
	}
	if rep.Total != balance {
		t.Errorf("итог расходов %s не совпадает со свёрткой журнала %s", rep.Total, balance)
	}
}

// TestASL_11_JournalFilters проверяет фильтры и постраничную выдачу журнала.
func TestASL_11_JournalFilters(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	all, err := Journal(ctx, fx.db, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("Journal: %v", err)
	}
	if len(all) != 6 {
		t.Fatalf("проводок в журнале %d, ожидалось 6", len(all))
	}
	if all[0].ActorName == "" || all[0].Lines[0].Path == "" {
		t.Errorf("журнал без имён автора или счетов: %+v", all[0])
	}
	if all[0].Amount != money.FromKopecks(1_000_000) {
		t.Errorf("сумма проводки: %s", all[0].Amount)
	}

	subtree, err := Journal(ctx, fx.db, store.TransactionFilter{AccountPrefix: "Расходы"})
	if err != nil {
		t.Fatalf("Journal(Расходы): %v", err)
	}
	if len(subtree) != 4 {
		t.Errorf("проводок по поддереву «Расходы»: %d, ожидалось 4", len(subtree))
	}
	count, err := fx.db.CountTransactions(ctx, store.TransactionFilter{AccountPrefix: "Расходы"})
	if err != nil {
		t.Fatalf("CountTransactions: %v", err)
	}
	if count != len(subtree) {
		t.Errorf("CountTransactions %d, ожидалось %d", count, len(subtree))
	}

	opType := model.OpExpense
	expenses, err := Journal(ctx, fx.db, store.TransactionFilter{OpType: &opType})
	if err != nil {
		t.Fatalf("Journal(трата): %v", err)
	}
	if len(expenses) != 3 {
		t.Errorf("проводок-трат: %d, ожидалось 3", len(expenses))
	}

	actor := fx.actor2
	byActor, err := Journal(ctx, fx.db, store.TransactionFilter{ActorID: &actor})
	if err != nil {
		t.Fatalf("Journal(автор): %v", err)
	}
	if len(byActor) != 1 || byActor[0].ActorName != "Мария" {
		t.Errorf("проводки Марии: %+v", byActor)
	}

	atLeast := money.FromKopecks(1_000_000)
	large, err := Journal(ctx, fx.db, store.TransactionFilter{AmountAtLeast: &atLeast})
	if err != nil {
		t.Fatalf("Journal(сумма): %v", err)
	}
	if len(large) != 2 {
		t.Errorf("крупных проводок: %d, ожидалось 2", len(large))
	}

	page, err := Journal(ctx, fx.db, store.TransactionFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("Journal(страница): %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("проводок на странице: %d", len(page))
	}
	// Свежие первыми: t4 (01.02), затем t5 и t6 (18.01) — страница начинается с t5.
	if page[0].UUID != "t5" || page[1].UUID != "t6" {
		t.Errorf("постраничная выдача: %s, %s", page[0].UUID, page[1].UUID)
	}

	byDate, err := Journal(ctx, fx.db, store.TransactionFilter{From: fx.janFrom, To: fx.janTo})
	if err != nil {
		t.Fatalf("Journal(период): %v", err)
	}
	if len(byDate) != 5 {
		t.Errorf("проводок за январь: %d, ожидалось 5", len(byDate))
	}
}
