package hledger

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// importBook — книга миграции: трата, доход, перевод и заём.
const importBook = `; книга домашней бухгалтерии
2026-01-15 * Пятёрочка
    Расходы:Продукты          1 234,56 ₽
    Активы:Наличные

2026-01-16 ! Зарплата за январь
    Активы:Карта            100 000,00
    Доходы:Зарплата

2026-01-17 Перевод на карту
    Активы:Карта              5 000,00
    Активы:Наличные

2026-01-20 Взяли в долг
    Активы:Наличные          10 000,00
    Обязательства:Банк
`

// newTestBook поднимает книгу с одним пользователем (в книге только служебные счета).
func newTestBook(t *testing.T) (*Importer, *store.DB, int64) {
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
	user, err := db.CreateUser(ctx, "Андрей", "секрет")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return NewImporter(db), db, user.ID
}

// countUserAccounts считает счета книги, не являющиеся служебными.
func countUserAccounts(t *testing.T, db *store.DB) int {
	t.Helper()
	accounts, err := db.ListAccounts(context.Background(), store.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	count := 0
	for _, account := range accounts {
		if !account.IsSystem {
			count++
		}
	}
	return count
}

// lineTexts возвращает строки проводки в виде «путь счёта: сторона сумма».
func lineTexts(t *testing.T, db *store.DB, tr *model.Transaction) []string {
	t.Helper()
	ctx := context.Background()
	texts := make([]string, 0, len(tr.Lines))
	for _, line := range tr.Lines {
		account, err := db.Account(ctx, line.AccountID)
		if err != nil {
			t.Fatalf("Account(%d): %v", line.AccountID, err)
		}
		texts = append(texts, fmt.Sprintf("%s: %s %s", account.FullPath, line.Side, line.Amount))
	}
	return texts
}

func TestASL_8_ImportFlexibleCreatesAccounts(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	report, err := im.Import(ctx, []byte(importBook), Options{
		Mode: ModeFlexible, ActorID: actorID, FileName: "book.journal",
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Rejected {
		t.Fatalf("импорт отклонён: %v", report.Problems)
	}
	if report.Accepted != 4 || report.Skipped != 0 || report.Duplicates != 0 {
		t.Fatalf("отчёт: принято %d, пропущено %d, дублей %d; проблемы: %v",
			report.Accepted, report.Skipped, report.Duplicates, report.Problems)
	}

	want := []string{
		"Доходы", "Обязательства", "Расходы",
		"Активы:Карта", "Активы:Наличные", "Доходы:Зарплата", "Обязательства:Банк", "Расходы:Продукты",
	}
	if got := strings.Join(report.CreatedAccounts, ","); got != strings.Join(want, ",") {
		t.Errorf("созданные счета %q, ожидались %q", got, want)
	}
	if count := countUserAccounts(t, db); count != len(want) {
		t.Errorf("счетов в книге %d, ожидалось %d", count, len(want))
	}

	types := map[string]model.AccountType{
		"Активы:Карта":       model.TypeAsset,
		"Доходы:Зарплата":    model.TypeIncome,
		"Обязательства:Банк": model.TypeLiability,
		"Расходы:Продукты":   model.TypeExpense,
	}
	for path, wantType := range types {
		account, err := db.AccountByPath(ctx, path)
		if err != nil {
			t.Fatalf("AccountByPath(%q): %v", path, err)
		}
		if account.Type != wantType {
			t.Errorf("тип счёта %q равен %q, ожидался %q", path, account.Type, wantType)
		}
	}

	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{Source: Source})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 4 {
		t.Fatalf("записано проводок %d, ожидалось 4", len(transactions))
	}

	// Тип операции выводится из типов счетов сторон, суммы строк положительны.
	byDescription := map[string]*model.Transaction{}
	for i := range transactions {
		byDescription[transactions[i].Description] = &transactions[i]
	}
	cases := []struct {
		description string
		opType      model.OpType
		lines       []string
	}{
		{
			description: "Пятёрочка",
			opType:      model.OpExpense,
			lines: []string{
				"Расходы:Продукты: DEBIT 1 234,56 ₽",
				"Активы:Наличные: CREDIT 1 234,56 ₽",
			},
		},
		{
			description: "Зарплата за январь",
			opType:      model.OpIncome,
			lines: []string{
				"Активы:Карта: DEBIT 100 000,00 ₽",
				"Доходы:Зарплата: CREDIT 100 000,00 ₽",
			},
		},
		{
			description: "Перевод на карту",
			opType:      model.OpTransfer,
			lines: []string{
				"Активы:Карта: DEBIT 5 000,00 ₽",
				"Активы:Наличные: CREDIT 5 000,00 ₽",
			},
		},
		{
			description: "Взяли в долг",
			opType:      model.OpLoan,
			lines: []string{
				"Активы:Наличные: DEBIT 10 000,00 ₽",
				"Обязательства:Банк: CREDIT 10 000,00 ₽",
			},
		},
	}
	for _, c := range cases {
		tr, ok := byDescription[c.description]
		if !ok {
			t.Fatalf("не найдена проводка %q", c.description)
		}
		if tr.OpType != c.opType {
			t.Errorf("тип операции проводки %q равен %q, ожидался %q", c.description, tr.OpType, c.opType)
		}
		if tr.SourceHash == "" || tr.UUID == "" {
			t.Errorf("проводка %q записана без source_hash или uuid", c.description)
		}
		if got := lineTexts(t, db, tr); !equalSets(got, c.lines) {
			t.Errorf("строки проводки %q: %v, ожидались %v", c.description, got, c.lines)
		}
	}
}

func TestASL_8_ImportStrictRejectsUnknownAccount(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	report, err := im.Import(ctx, []byte(importBook), Options{
		Mode: ModeStrict, ActorID: actorID, FileName: "book.journal",
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !report.Rejected {
		t.Fatal("строгий режим не отклонил импорт с неизвестными счетами")
	}
	if report.Accepted != 0 {
		t.Errorf("принято проводок %d, ожидалось 0", report.Accepted)
	}
	if count := countUserAccounts(t, db); count != 0 {
		t.Errorf("строгий режим создал %d счетов", count)
	}
	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 0 {
		t.Errorf("записано проводок %d, ожидалось 0", len(transactions))
	}

	var messages []string
	for _, problem := range report.Problems {
		messages = append(messages, problem.Error())
	}
	for _, want := range []string{"Расходы:Продукты", "Активы:Карта", "Обязательства:Банк"} {
		found := false
		for _, message := range messages {
			if strings.Contains(message, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("в отчёте нет счёта %q: %v", want, messages)
		}
	}
}

func TestASL_8_ImportStrictWithExistingPlan(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	_, _, err := db.EnsureAccounts(ctx, []store.AccountSpec{
		{Path: "Активы:Наличные", Type: model.TypeAsset},
		{Path: "Активы:Карта", Type: model.TypeAsset},
		{Path: "Расходы", Type: model.TypeExpense},
		{Path: "Расходы:Продукты", Type: model.TypeExpense},
		{Path: "Доходы", Type: model.TypeIncome},
		{Path: "Доходы:Зарплата", Type: model.TypeIncome},
		{Path: "Обязательства", Type: model.TypeLiability},
		{Path: "Обязательства:Банк", Type: model.TypeLiability},
	})
	if err != nil {
		t.Fatalf("EnsureAccounts: %v", err)
	}

	report, err := im.Import(ctx, []byte(importBook), Options{Mode: ModeStrict, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Rejected || report.Accepted != 4 {
		t.Fatalf("строгий импорт: отклонён=%v, принято=%d, проблемы=%v",
			report.Rejected, report.Accepted, report.Problems)
	}
	if len(report.CreatedAccounts) != 0 {
		t.Errorf("строгий режим создал счета: %v", report.CreatedAccounts)
	}
}

func TestASL_8_ImportIdempotent(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()
	opts := Options{Mode: ModeFlexible, ActorID: actorID, FileName: "book.journal"}

	first, err := im.Import(ctx, []byte(importBook), opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	second, err := im.Import(ctx, []byte(importBook), opts)
	if err != nil {
		t.Fatalf("повторный Import: %v", err)
	}
	if second.Accepted != 0 || second.Duplicates != first.Accepted {
		t.Fatalf("повторный импорт: принято %d, дублей %d (ожидалось 0 и %d)",
			second.Accepted, second.Duplicates, first.Accepted)
	}
	if len(second.CreatedAccounts) != 0 {
		t.Errorf("повторный импорт создал счета: %v", second.CreatedAccounts)
	}

	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{Source: Source})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != first.Accepted {
		t.Errorf("после повторного импорта проводок %d, ожидалось %d", len(transactions), first.Accepted)
	}
}

func TestASL_8_ImportDuplicateLinesInBook(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	// Две одинаковые проводки (например, два одинаковых кофе) остаются двумя.
	journal := `2026-05-01 Кофе
    Расходы:Продукты     250,00
    Активы:Наличные

2026-05-01 Кофе
    Расходы:Продукты     250,00
    Активы:Наличные
`
	opts := Options{Mode: ModeFlexible, ActorID: actorID}
	report, err := im.Import(ctx, []byte(journal), opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 2 {
		t.Fatalf("принято проводок %d, ожидалось 2 (проблемы: %v)", report.Accepted, report.Problems)
	}

	again, err := im.Import(ctx, []byte(journal), opts)
	if err != nil {
		t.Fatalf("повторный Import: %v", err)
	}
	if again.Accepted != 0 || again.Duplicates != 2 {
		t.Errorf("повторный импорт: принято %d, дублей %d", again.Accepted, again.Duplicates)
	}

	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{Source: Source})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 2 {
		t.Errorf("записано проводок %d, ожидалось 2", len(transactions))
	}
}

func TestASL_8_ImportSkipsInvalidWithReason(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	journal := `2026-06-01 Верная проводка
    Расходы:Продукты        100,00
    Активы:Наличные

2026-06-02 Несбалансированная проводка
    Расходы:Продукты        100,00
    Активы:Наличные          50,00

2026-06-03 Пропущенная сумма в строке
    Расходы:Продукты        сто рублей
    Активы:Наличные
`
	report, err := im.Import(ctx, []byte(journal), Options{Mode: ModeFlexible, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 1 || report.Skipped != 2 {
		t.Fatalf("принято %d, пропущено %d (проблемы: %v)", report.Accepted, report.Skipped, report.Problems)
	}
	var messages []string
	lines := map[int]bool{}
	for _, problem := range report.Problems {
		messages = append(messages, problem.Error())
		lines[problem.Line] = true
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "ERR_UNBALANCED") {
		t.Errorf("нет причины ERR_UNBALANCED: %s", joined)
	}
	if !strings.Contains(joined, "строка 10") {
		t.Errorf("нет причины с номером строки 10: %s", joined)
	}
	if !lines[5] {
		t.Errorf("нет проблемы для строки 5 (несбалансированная проводка): %v", report.Problems)
	}

	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{Source: Source})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("записано проводок %d, ожидалась 1", len(transactions))
	}
}

func TestASL_8_ImportTypeFromAnnotation(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	// Корень «Траты» не распознаётся по имени — тип берётся из директивы.
	journal := `account Траты ; тип: расход

2026-07-01 Кофе
    Траты:Кафе          250,00
    Активы:Наличные
`
	report, err := im.Import(ctx, []byte(journal), Options{Mode: ModeFlexible, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 1 {
		t.Fatalf("принято %d, проблемы: %v", report.Accepted, report.Problems)
	}
	account, err := db.AccountByPath(ctx, "Траты:Кафе")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if account.Type != model.TypeExpense {
		t.Errorf("тип счёта %q равен %q, ожидался «расход»", account.FullPath, account.Type)
	}

	// Без директивы тип не угадывается: проводка пропускается с причиной.
	unknown := `2026-07-02 Кофе
    Неведомо:Кафе       250,00
    Активы:Наличные
`
	report, err = im.Import(ctx, []byte(unknown), Options{Mode: ModeFlexible, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 0 || report.Skipped != 1 {
		t.Fatalf("принято %d, пропущено %d", report.Accepted, report.Skipped)
	}
	joined := strings.Join(problemTexts(report.Problems), "\n")
	if !strings.Contains(joined, "не удалось определить тип счёта") {
		t.Errorf("нет причины о неизвестном типе счёта: %s", joined)
	}
}

func TestASL_8_ImportEnglishBookWithAnnotations(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	journal := `account assets:cash ; тип: актив
account expenses:food ; тип: расход

2026-08-01 Lunch
    expenses:food       25.50
    assets:cash
`
	report, err := im.Import(ctx, []byte(journal), Options{Mode: ModeFlexible, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 1 {
		t.Fatalf("принято %d, проблемы: %v", report.Accepted, report.Problems)
	}
	food, err := db.AccountByPath(ctx, "expenses:food")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if food.Type != model.TypeExpense {
		t.Errorf("тип счёта %q равен %q", food.FullPath, food.Type)
	}
	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{Source: Source})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("записано проводок %d, ожидалась 1", len(transactions))
	}
	if got := lineTexts(t, db, &transactions[0]); !equalSets(got, []string{
		"expenses:food: DEBIT 25,50 ₽",
		"assets:cash: CREDIT 25,50 ₽",
	}) {
		t.Errorf("строки проводки: %v", got)
	}
}

func TestASL_8_ImportWritesSingleCommit(t *testing.T) {
	im, db, actorID := newTestBook(t)
	ctx := context.Background()

	before, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	report, err := im.Import(ctx, []byte(importBook), Options{Mode: ModeFlexible, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 4 {
		t.Fatalf("принято %d, проблемы: %v", report.Accepted, report.Problems)
	}

	commit, err := db.LastCommit(ctx)
	if err != nil {
		t.Fatalf("LastCommit: %v", err)
	}
	if commit.ClientID != "import:"+Source {
		t.Errorf("идентификатор коммита импорта %q", commit.ClientID)
	}
	if commit.Hash == "" || commit.OpsJSON == "" {
		t.Error("коммит импорта записан без звена хэш-цепочки или содержимого")
	}
	if commit.ResultSeq <= before {
		t.Errorf("result_seq коммита %d не больше исходного %d", commit.ResultSeq, before)
	}

	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{Source: Source})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	for _, tr := range transactions {
		if tr.CommitID != commit.ID {
			t.Errorf("проводка %q записана коммитом %d, ожидался %d", tr.Description, tr.CommitID, commit.ID)
		}
	}
}

func TestASL_8_ImportReportIncludesParseErrors(t *testing.T) {
	im, _, actorID := newTestBook(t)
	ctx := context.Background()

	journal := `2026-09-01 Верная
    Расходы:Продукты     100,00
    Активы:Наличные

2026-13-40 Неверная дата
    Расходы:Продукты     100,00
    Активы:Наличные
`
	report, err := im.Import(ctx, []byte(journal), Options{Mode: ModeFlexible, ActorID: actorID})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Accepted != 1 {
		t.Fatalf("принято %d, проблемы: %v", report.Accepted, report.Problems)
	}
	joined := strings.Join(problemTexts(report.Problems), "\n")
	if !strings.Contains(joined, "строка 5") || !strings.Contains(joined, "неверная дата") {
		t.Errorf("ошибка разбора без строки и причины: %s", joined)
	}
}

func TestInferOpTypeAndTypeByName(t *testing.T) {
	if got := inferTypeByName("Расходы"); got != model.TypeExpense {
		t.Errorf("тип по имени «Расходы» = %q", got)
	}
	if got := inferTypeByName("EXpenses"); got != model.TypeExpense {
		t.Errorf("тип по имени «EXpenses» = %q", got)
	}
	if got := inferTypeByName("Что-то"); got != "" {
		t.Errorf("неизвестное имя дало тип %q", got)
	}

	lines := []model.TransactionLine{
		{Amount: money.FromKopecks(100), Side: model.SideDebit},
		{Amount: money.FromKopecks(100), Side: model.SideCredit},
	}
	cases := []struct {
		types []model.AccountType
		want  model.OpType
	}{
		{[]model.AccountType{model.TypeAsset, model.TypeAsset}, model.OpTransfer},
		{[]model.AccountType{model.TypeExpense, model.TypeAsset}, model.OpExpense},
		{[]model.AccountType{model.TypeAsset, model.TypeIncome}, model.OpIncome},
		{[]model.AccountType{model.TypeAsset, model.TypeLiability}, model.OpLoan},
		{[]model.AccountType{model.TypeLiability, model.TypeAsset}, model.OpDebtPayment},
		{[]model.AccountType{model.TypeAsset, model.TypeCapital}, model.OpInitial},
		{[]model.AccountType{model.TypeAsset, model.TypeExpense}, model.OpAdjustment},
	}
	for _, c := range cases {
		if got := inferOpType(lines, c.types); got != c.want {
			t.Errorf("тип операции для счетов %v = %q, ожидался %q", c.types, got, c.want)
		}
	}
}

// problemTexts возвращает причины проблем в виде строк.
func problemTexts(problems []Problem) []string {
	texts := make([]string, 0, len(problems))
	for _, problem := range problems {
		texts = append(texts, problem.Error())
	}
	return texts
}

// equalSets сравнивает два набора строк без учёта порядка.
func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA := append([]string(nil), a...)
	sortedB := append([]string(nil), b...)
	sort.Strings(sortedA)
	sort.Strings(sortedB)
	for i := range sortedA {
		if sortedA[i] != sortedB[i] {
			return false
		}
	}
	return true
}
