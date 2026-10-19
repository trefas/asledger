package hledger

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// roundTripBook — книга с разнообразными случаями: многострочная проводка,
// отрицательные суммы, служебный счёт капитала и начальные остатки.
const roundTripBook = `; книга для проверки круга экспорт → импорт
2026-01-01 Начальные остатки
    Активы:Наличные           10 000,00
    Активы:Карта              90 000,00
    Обязательства:Ипотека    -30 000,00
    Капитал:Начальные

2026-01-05 Продукты по трём категориям
    Расходы:Продукты             500,00
    Расходы:Быт                  300,00
    Расходы:Связь                200,00
    Активы:Наличные

2026-01-10 Зарплата за декабрь
    Активы:Карта              50 000,00
    Доходы:Зарплата

2026-01-12 Перевод на наличные
    Активы:Наличные            5 000,00
    Активы:Карта

2026-01-15 Погашение части долга
    Обязательства:Ипотека      2 000,00
    Активы:Карта

2026-01-20 Корректировка курса
    Активы:Карта                 250,00
    Капитал:Переоценка
`

// TestASL_8_ExportImportRoundTrip проверяет критерий эпика: выгрузка книги в
// hledger и повторный импорт дают идентичное состояние.
func TestASL_8_ExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()

	source, sourceDB, sourceActor := newTestBook(t)
	report, err := source.Import(ctx, []byte(roundTripBook), Options{
		Mode: ModeFlexible, ActorID: sourceActor, FileName: "round-trip.journal",
	})
	if err != nil {
		t.Fatalf("Import в исходную книгу: %v", err)
	}
	if report.Accepted != 6 || report.Skipped != 0 {
		t.Fatalf("исходная книга: принято %d, пропущено %d, проблемы: %v",
			report.Accepted, report.Skipped, report.Problems)
	}

	var exported bytes.Buffer
	if err := Export(ctx, sourceDB, &exported, WriteOptions{Comment: "выгрузка книги"}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Выгрузка читается собственным парсером без ошибок.
	parsed := Parse(exported.Bytes())
	if len(parsed.Errors) != 0 {
		t.Fatalf("выгрузка не читается собственным парсером: %v", parsed.Errors)
	}
	if len(parsed.Transactions) != 6 {
		t.Fatalf("в выгрузке проводок %d, ожидалось 6", len(parsed.Transactions))
	}

	target, targetDB, targetActor := newTestBook(t)
	second, err := target.Import(ctx, exported.Bytes(), Options{
		Mode: ModeFlexible, ActorID: targetActor, FileName: "round-trip.journal",
	})
	if err != nil {
		t.Fatalf("Import выгрузки: %v", err)
	}
	if second.Rejected || second.Accepted != 6 || second.Skipped != 0 {
		t.Fatalf("импорт выгрузки: отклонён=%v, принято=%d, пропущено=%d, проблемы: %v",
			second.Rejected, second.Accepted, second.Skipped, second.Problems)
	}

	compareBooks(t, sourceDB, targetDB)

	// Второй цикл импорт/экспорт не порождает дублей и не меняет остатки.
	before, err := targetDB.Balances(ctx)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	third, err := target.Import(ctx, exported.Bytes(), Options{
		Mode: ModeFlexible, ActorID: targetActor, FileName: "round-trip.journal",
	})
	if err != nil {
		t.Fatalf("повторный импорт: %v", err)
	}
	if third.Accepted != 0 || third.Duplicates != 6 {
		t.Fatalf("повторный импорт: принято %d, дублей %d", third.Accepted, third.Duplicates)
	}
	after, err := targetDB.Balances(ctx)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	if len(before) != len(after) {
		t.Fatalf("после повторного импорта счетов %d, было %d", len(after), len(before))
	}
	for id, balance := range before {
		if after[id] != balance {
			t.Errorf("остаток счёта %d изменился повторным импортом: %s → %s", id, balance, after[id])
		}
	}
}

// compareBooks сравнивает план счетов, проводки и остатки двух книг.
func compareBooks(t *testing.T, want, got *store.DB) {
	t.Helper()

	// План счетов: пути и типы (архивных счетов в книгах нет).
	wantAccounts := accountTypes(t, want)
	gotAccounts := accountTypes(t, got)
	if len(wantAccounts) != len(gotAccounts) {
		t.Errorf("счетов в книгах %d и %d", len(wantAccounts), len(gotAccounts))
	}
	for path, wantType := range wantAccounts {
		if gotType, ok := gotAccounts[path]; !ok {
			t.Errorf("во второй книге нет счёта %q", path)
		} else if gotType != wantType {
			t.Errorf("тип счёта %q: %q против %q", path, gotType, wantType)
		}
	}

	wantTransactions := signatures(t, want)
	gotTransactions := signatures(t, got)
	if strings.Join(wantTransactions, "\n") != strings.Join(gotTransactions, "\n") {
		t.Errorf("проводки книг различаются:\nисходная:\n%s\nвосстановленная:\n%s",
			strings.Join(wantTransactions, "\n"), strings.Join(gotTransactions, "\n"))
	}

	// Остатки по путям счетов.
	wantBalances := balancesByPath(t, want)
	gotBalances := balancesByPath(t, got)
	for path, wantBalance := range wantBalances {
		gotBalance, ok := gotBalances[path]
		if !ok {
			t.Errorf("во второй книге нет остатка счёта %q", path)
			continue
		}
		if gotBalance != wantBalance {
			t.Errorf("остаток счёта %q равен %s, ожидался %s", path, gotBalance, wantBalance)
		}
	}
}

// accountTypes возвращает типы всех счетов книги по пути.
func accountTypes(t *testing.T, db *store.DB) map[string]model.AccountType {
	t.Helper()
	accounts, err := db.ListAccounts(context.Background(), store.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	types := make(map[string]model.AccountType, len(accounts))
	for _, account := range accounts {
		types[account.FullPath] = account.Type
	}
	return types
}

// balancesByPath возвращает остатки книги по путям счетов.
func balancesByPath(t *testing.T, db *store.DB) map[string]money.Kop {
	t.Helper()
	ctx := context.Background()
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	balances, err := db.Balances(ctx)
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	byPath := make(map[string]money.Kop, len(accounts))
	for _, account := range accounts {
		byPath[account.FullPath] = balances[account.ID]
	}
	return byPath
}

// signatures описывает проводки книги строками, пригодными для сравнения.
func signatures(t *testing.T, db *store.DB) []string {
	t.Helper()
	ctx := context.Background()
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	paths := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		paths[account.ID] = account.FullPath
	}

	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	signatures := make([]string, 0, len(transactions))
	for _, tr := range transactions {
		lines := make([]string, 0, len(tr.Lines))
		for _, line := range tr.Lines {
			path, ok := paths[line.AccountID]
			if !ok {
				t.Fatalf("неизвестный счёт строки %d", line.AccountID)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", path, line.Side, line.Amount))
		}
		sort.Strings(lines)
		signatures = append(signatures, fmt.Sprintf("%s|%s|%s|%s",
			tr.Date.Format("2006-01-02"), tr.Description, tr.OpType, strings.Join(lines, "; ")))
	}
	sort.Strings(signatures)
	return signatures
}
