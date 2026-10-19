package hledger

import (
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// writeAccounts — небольшой план счетов для проверки выгрузки.
func writeAccounts() []model.Account {
	return []model.Account{
		{ID: 1, Name: "Активы", FullPath: "Активы", Type: model.TypeAsset},
		{ID: 2, Name: "Наличные", FullPath: "Активы:Наличные", Type: model.TypeAsset},
		{ID: 3, Name: "Продукты", FullPath: "Расходы:Продукты", Type: model.TypeExpense},
		{ID: 4, Name: "Архив", FullPath: "Расходы:Архив", Type: model.TypeExpense, IsArchived: true},
		{ID: 5, Name: "Зарплата", FullPath: "Доходы:Зарплата", Type: model.TypeIncome},
	}
}

func TestWriteBook(t *testing.T) {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	transactions := []model.Transaction{
		{
			ID: 1, Date: date, Description: "Пятёрочка",
			Lines: []model.TransactionLine{
				{ID: 2, AccountID: 3, Amount: money.FromKopecks(123456), Side: model.SideDebit},
				{ID: 1, AccountID: 2, Amount: money.FromKopecks(123456), Side: model.SideCredit},
			},
		},
		{
			ID: 2, Date: date, Description: "Зарплата",
			Lines: []model.TransactionLine{
				{ID: 1, AccountID: 2, Amount: money.FromKopecks(10000000), Side: model.SideDebit},
				{ID: 2, AccountID: 5, Amount: money.FromKopecks(10000000), Side: model.SideCredit},
			},
		},
	}

	var buf strings.Builder
	if err := Write(&buf, writeAccounts(), transactions, WriteOptions{Comment: "выгрузка"}); err != nil {
		t.Fatalf("выгрузка: %v", err)
	}
	got := buf.String()

	for _, want := range []string{
		"; выгрузка",
		"account Активы",
		"account Активы:Наличные",
		"account Доходы:Зарплата",
		"account Расходы:Продукты",
		"; тип: расход",
		"2026-01-15 Пятёрочка",
		"2026-01-15 Зарплата",
		"1234.56",
		"-1234.56",
		"100000.00",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("в выгрузке нет %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Расходы:Архив") {
		t.Errorf("архивный счёт попал в выгрузку:\n%s", got)
	}
	// Дебетовая строка идёт перед кредитовой, суммы выровнены по одной колонке.
	lines := strings.Split(got, "\n")
	var postings []string
	for _, line := range lines {
		if strings.HasPrefix(line, "    ") {
			postings = append(postings, line)
		}
	}
	if len(postings) != 4 {
		t.Fatalf("строк проводок %d, ожидалось 4:\n%s", len(postings), got)
	}
	if !strings.HasPrefix(strings.TrimSpace(postings[0]), "Расходы:Продукты") {
		t.Errorf("дебетовая строка должна идти первой: %q", postings[0])
	}
	column := func(line, amount string) int {
		idx := strings.Index(line, amount)
		if idx < 0 {
			t.Fatalf("в строке %q нет суммы %q", line, amount)
		}
		return len([]rune(line[:idx]))
	}
	if column(postings[0], "1234.56") != column(postings[1], "-1234.56") {
		t.Errorf("суммы строк должны начинаться в одной колонке:\n%q\n%q", postings[0], postings[1])
	}
}

func TestWriteIncludeArchived(t *testing.T) {
	var buf strings.Builder
	if err := Write(&buf, writeAccounts(), nil, WriteOptions{IncludeArchived: true}); err != nil {
		t.Fatalf("выгрузка: %v", err)
	}
	if !strings.Contains(buf.String(), "account Расходы:Архив") {
		t.Errorf("архивный счёт отсутствует при IncludeArchived:\n%s", buf.String())
	}
}

func TestWriteUnknownAccount(t *testing.T) {
	transactions := []model.Transaction{{
		ID: 1, Date: time.Now(),
		Lines: []model.TransactionLine{
			{AccountID: 99, Amount: money.FromKopecks(100), Side: model.SideDebit},
		},
	}}
	var buf strings.Builder
	if err := Write(&buf, writeAccounts(), transactions, WriteOptions{}); err == nil {
		t.Error("выгрузка проводки с неизвестным счётом не дала ошибку")
	}
}

func TestASL_8_WriteRoundTrip(t *testing.T) {
	date := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	transactions := []model.Transaction{{
		ID: 1, Date: date, Description: "Перевод и трата",
		Lines: []model.TransactionLine{
			{ID: 1, AccountID: 3, Amount: money.FromKopecks(25050), Side: model.SideDebit},
			{ID: 2, AccountID: 2, Amount: money.FromKopecks(5050), Side: model.SideCredit},
			{ID: 3, AccountID: 1, Amount: money.FromKopecks(20000), Side: model.SideCredit},
		},
	}}

	var buf strings.Builder
	if err := Write(&buf, writeAccounts(), transactions, WriteOptions{}); err != nil {
		t.Fatalf("выгрузка: %v", err)
	}

	book := Parse([]byte(buf.String()))
	if len(book.Errors) != 0 {
		t.Fatalf("повторный разбор выгрузки дал ошибки: %v", book.Errors)
	}
	if len(book.Transactions) != 1 {
		t.Fatalf("повторно разобрано проводок %d, ожидалась 1", len(book.Transactions))
	}
	parsed := book.Transactions[0]
	if got := parsed.Date.Format("2006-01-02"); got != "2026-02-03" {
		t.Errorf("дата после круга %q", got)
	}
	if parsed.Description != "Перевод и трата" {
		t.Errorf("описание после круга %q", parsed.Description)
	}
	if len(parsed.Postings) != 3 {
		t.Fatalf("строк после круга %d, ожидалось 3", len(parsed.Postings))
	}
	want := map[string]money.Kop{
		"Расходы:Продукты": money.FromKopecks(25050),
		"Активы:Наличные":  money.FromKopecks(-5050),
		"Активы":           money.FromKopecks(-20000),
	}
	for _, posting := range parsed.Postings {
		expected, ok := want[posting.Account]
		if !ok {
			t.Errorf("неожиданный счёт после круга: %q", posting.Account)
			continue
		}
		if posting.Amount != expected {
			t.Errorf("сумма счёта %q после круга %s, ожидалось %s", posting.Account, posting.Amount, expected)
		}
		delete(want, posting.Account)
	}
	if len(want) != 0 {
		t.Errorf("не восстановлены строки: %v", want)
	}

	types := make(map[string]model.AccountType)
	for _, declared := range book.Declared {
		types[declared.Path] = declared.Type
	}
	if types["Расходы:Продукты"] != model.TypeExpense || types["Доходы:Зарплата"] != model.TypeIncome {
		t.Errorf("типы счетов не сохранены в директивах: %v", types)
	}
	if types["Активы"] != model.TypeAsset {
		t.Errorf("тип корневого счёта не сохранён: %v", types)
	}
}
