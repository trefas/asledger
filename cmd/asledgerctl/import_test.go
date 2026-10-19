package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// newCLIBook создаёт книгу с одним пользователем и возвращает путь к ней.
func newCLIBook(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "book.db")
	if code := run([]string{"migrate", "-db", dbPath}); code != 0 {
		t.Fatalf("migrate: код %d", code)
	}
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeDB(db)
	if _, err := db.CreateUser(ctx, "Андрей", "секрет"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return dbPath
}

// TestRunImportExportHledger проверяет круг импорта и выгрузки через утилиту.
func TestRunImportExportHledger(t *testing.T) {
	dir := t.TempDir()
	dbPath := newCLIBook(t)
	journalPath := filepath.Join(dir, "book.journal")
	writeFile(t, journalPath, `2026-01-15 Пятёрочка
    Расходы:Продукты        1 234,56
    Активы:Наличные

2026-01-16 Зарплата
    Активы:Карта            50 000,00
    Доходы:Зарплата
`)

	if code := run([]string{"import", "hledger", "-db", dbPath, journalPath}); code != 0 {
		t.Fatalf("import hledger: код %d, ожидался 0", code)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("проводок в книге %d, ожидалось 2", len(transactions))
	}
	if _, err := db.AccountByPath(ctx, "Расходы:Продукты"); err != nil {
		t.Fatalf("счёт импорта не создан: %v", err)
	}
	closeDB(db)

	// Выгрузка hledger в файл.
	outPath := filepath.Join(dir, "out.journal")
	if code := run([]string{"export", "hledger", "-db", dbPath, "-out", outPath}); code != 0 {
		t.Fatalf("export hledger: код %d, ожидался 0", code)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("чтение выгрузки: %v", err)
	}
	if !containsFold(string(data), "Пятёрочка") || !containsFold(string(data), "account Расходы:Продукты") {
		t.Errorf("выгрузка hledger неполна: %s", data)
	}

	// Выгрузка отчёта об остатках в CSV.
	csvPath := filepath.Join(dir, "balances.csv")
	if code := run([]string{"export", "csv", "-db", dbPath, "-report", "balances", "-out", csvPath}); code != 0 {
		t.Fatalf("export csv: код %d, ожидался 0", code)
	}
	csv, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("чтение CSV: %v", err)
	}
	if !containsFold(string(csv), "остаток") || !containsFold(string(csv), "-1234,56") {
		t.Errorf("отчёт об остатках неполон: %s", csv)
	}

	// Полный снапшот JSON.
	jsonPath := filepath.Join(dir, "snapshot.json")
	if code := run([]string{"export", "json", "-db", dbPath, "-out", jsonPath}); code != 0 {
		t.Fatalf("export json: код %d, ожидался 0", code)
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("чтение снапшота: %v", err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("снапшот не является JSON: %v", err)
	}
	if snapshot["chain_hash"] == "" || snapshot["head_seq"] == nil {
		t.Errorf("снапшот без звена цепочки или головы: %s", raw)
	}
}

// TestRunImportHledgerStrict проверяет отказ строгого режима.
func TestRunImportHledgerStrict(t *testing.T) {
	dir := t.TempDir()
	dbPath := newCLIBook(t)
	journalPath := filepath.Join(dir, "book.journal")
	writeFile(t, journalPath, "2026-02-01 Покупка\n    Неведомо:Товар    100,00\n    Активы:Наличные\n")

	code := run([]string{"import", "hledger", "-db", dbPath, "-mode", "строгий", journalPath})
	if code != 1 {
		t.Fatalf("строгий импорт с неизвестным счётом: код %d, ожидался 1", code)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeDB(db)
	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 0 {
		t.Errorf("строгий режим записал проводки: %d", len(transactions))
	}
	if _, err := db.AccountByPath(ctx, "Неведомо"); err == nil {
		t.Error("строгий режим создал счёт")
	}
}

// TestRunImportHledgerNeedsActor проверяет выбор автора проводок.
func TestRunImportHledgerNeedsActor(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")
	if code := run([]string{"migrate", "-db", dbPath}); code != 0 {
		t.Fatalf("migrate: код %d", code)
	}
	journalPath := filepath.Join(dir, "book.journal")
	writeFile(t, journalPath, "2026-03-01 Тест\n    Расходы:Продукты    100,00\n    Активы:Наличные\n")

	// В книге нет пользователей: автора определить нельзя.
	if code := run([]string{"import", "hledger", "-db", dbPath, journalPath}); code != 1 {
		t.Fatalf("импорт без пользователей: код %d, ожидался 1", code)
	}
}

// containsFold проверяет вхождение подстроки без учёта регистра.
func containsFold(text, substring string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(substring))
}
