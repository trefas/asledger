package export

import (
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// testAccounts — план счетов для проверки выгрузки.
func testAccounts() []model.Account {
	return []model.Account{
		{ID: 1, FullPath: "Активы:Наличные", Type: model.TypeAsset},
		{ID: 2, FullPath: "Расходы:Продукты", Type: model.TypeExpense},
		{ID: 3, FullPath: "Обязательства:Банк", Type: model.TypeLiability},
	}
}

// testTransactions — проводки для проверки выгрузки.
func testTransactions() []model.Transaction {
	date := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	return []model.Transaction{
		{
			ID: 2, UUID: "u-2", OpType: model.OpIncome, Date: date.AddDate(0, 0, 1),
			Description: "Зарплата", ActorID: 7, Source: "web",
			Lines: []model.TransactionLine{
				{AccountID: 1, Amount: money.FromKopecks(123456), Side: model.SideDebit},
			},
		},
		{
			ID: 1, UUID: "u-1", OpType: model.OpExpense, Date: date,
			Description: "Продукты; молочные", ActorID: 7, Source: "hledger",
			Lines: []model.TransactionLine{
				{AccountID: 2, Amount: money.FromKopecks(25050), Side: model.SideDebit},
				{AccountID: 1, Amount: money.FromKopecks(25050), Side: model.SideCredit},
			},
		},
	}
}

// readCSV читает CSV с меткой UTF-8 и разделителем «;».
func readCSV(t *testing.T, text string) [][]string {
	t.Helper()
	if !strings.HasPrefix(text, "\ufeff") {
		t.Fatalf("CSV записан без метки порядка байтов UTF-8: %q", text[:min(8, len(text))])
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	reader.Comma = ';'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("чтение CSV: %v", err)
	}
	return rows
}

// TestASL_8_ExportJournalCSV проверяет журнал проводок в CSV.
func TestASL_8_ExportJournalCSV(t *testing.T) {
	var buf strings.Builder
	if err := Journal(&buf, testAccounts(), testTransactions()); err != nil {
		t.Fatalf("Journal: %v", err)
	}
	rows := readCSV(t, buf.String())
	if len(rows) != 4 {
		t.Fatalf("строк в журнале %d, ожидалось 4: %v", len(rows), rows)
	}
	wantHeader := []string{"проводка", "uuid", "дата", "тип операции", "описание", "автор", "источник", "счёт", "сторона", "сумма"}
	if strings.Join(rows[0], "|") != strings.Join(wantHeader, "|") {
		t.Errorf("заголовок журнала: %v", rows[0])
	}
	// Проводки идут по возрастанию даты, строки — в порядке записи.
	first := rows[1]
	if first[0] != "1" || first[2] != "2026-01-15" || first[7] != "Расходы:Продукты" ||
		first[8] != "DEBIT" || first[9] != "250,50" {
		t.Errorf("первая строка журнала: %v", first)
	}
	if first[4] != "Продукты; молочные" {
		t.Errorf("описание с разделителем искажено: %q", first[4])
	}
	if rows[3][7] != "Активы:Наличные" || rows[3][9] != "1234,56" {
		t.Errorf("строка дохода: %v", rows[3])
	}
	if strings.Contains(buf.String(), "₽") {
		t.Error("суммы выгружены со знаком рубля: электронная таблица прочитает их как текст")
	}
}

// TestASL_8_ExportBalancesCSV проверяет отчёт об остатках в CSV.
func TestASL_8_ExportBalancesCSV(t *testing.T) {
	balances := map[int64]money.Kop{
		1: money.FromKopecks(985000),
		2: money.FromKopecks(-25050),
		3: money.FromKopecks(-960000),
	}
	var buf strings.Builder
	if err := Balances(&buf, testAccounts(), balances); err != nil {
		t.Fatalf("Balances: %v", err)
	}
	rows := readCSV(t, buf.String())
	if len(rows) != 4 {
		t.Fatalf("строк в отчёте %d, ожидалось 4: %v", len(rows), rows)
	}
	if strings.Join(rows[0], "|") != "счёт|тип|остаток" {
		t.Errorf("заголовок отчёта об остатках: %v", rows[0])
	}
	want := [][]string{
		{"Активы:Наличные", "актив", "9850,00"},
		{"Обязательства:Банк", "пассив", "-9600,00"},
		{"Расходы:Продукты", "расход", "-250,50"},
	}
	for i, row := range want {
		if strings.Join(rows[i+1], "|") != strings.Join(row, "|") {
			t.Errorf("строка остатков %d: %v, ожидалось %v", i, rows[i+1], row)
		}
	}
}

// TestASL_8_ExportTurnoversCSV проверяет отчёт об оборотах за период.
func TestASL_8_ExportTurnoversCSV(t *testing.T) {
	turnovers := map[int64]store.Turnover{
		1: {
			AccountID:    1,
			OpeningDebit: money.FromKopecks(1000000), OpeningCredit: money.FromKopecks(0),
			PeriodDebit: money.FromKopecks(123456), PeriodCredit: money.FromKopecks(25050),
		},
		2: {
			AccountID:   2,
			PeriodDebit: money.FromKopecks(25050),
		},
		3: {
			AccountID:     3,
			OpeningCredit: money.FromKopecks(1000000),
			PeriodDebit:   money.FromKopecks(40000),
		},
	}
	var buf strings.Builder
	if err := Turnovers(&buf, testAccounts(), turnovers); err != nil {
		t.Fatalf("Turnovers: %v", err)
	}
	rows := readCSV(t, buf.String())
	if len(rows) != 4 {
		t.Fatalf("строк в отчёте %d, ожидалось 4: %v", len(rows), rows)
	}
	if strings.Join(rows[0], "|") != "счёт|тип|остаток на начало|дебет|кредит|остаток на конец" {
		t.Errorf("заголовок отчёта об оборотах: %v", rows[0])
	}
	// Актив: остаток на начало 10 000,00, за период +984,06 → 10 984,06.
	if rows[1][2] != "10000,00" || rows[1][3] != "1234,56" || rows[1][4] != "250,50" || rows[1][5] != "10984,06" {
		t.Errorf("строка оборотов актива: %v", rows[1])
	}
	// Пассив: остаток на начало 10 000,00, за период погашено 400,00 → 9 600,00.
	if rows[2][2] != "10000,00" || rows[2][3] != "400,00" || rows[2][5] != "9600,00" {
		t.Errorf("строка оборотов пассива: %v", rows[2])
	}
}

// TestASL_8_ExportJournalUnknownAccount проверяет отказ при неизвестном счёте.
func TestASL_8_ExportJournalUnknownAccount(t *testing.T) {
	transactions := []model.Transaction{{
		ID: 1, Date: time.Now(),
		Lines: []model.TransactionLine{{AccountID: 99, Amount: money.FromKopecks(100), Side: model.SideDebit}},
	}}
	var buf strings.Builder
	err := Journal(&buf, testAccounts(), transactions)
	if err == nil {
		t.Fatal("выгрузка журнала с неизвестным счётом не дала ошибки")
	}
	if !strings.Contains(err.Error(), "неизвестен счёт") {
		t.Errorf("ошибка выгрузки: %v", err)
	}
}

// failingWriter имитирует сбой записи.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("сбой записи") }

func TestASL_8_ExportWriterFailure(t *testing.T) {
	if err := Journal(failingWriter{}, testAccounts(), testTransactions()); err == nil {
		t.Error("сбой записи не замечен")
	}
}

// TestASL_8_Rubles проверяет формат сумм для электронных таблиц.
func TestASL_8_Rubles(t *testing.T) {
	cases := map[money.Kop]string{
		money.FromKopecks(0):          "0,00",
		money.FromKopecks(1):          "0,01",
		money.FromKopecks(100):        "1,00",
		money.FromKopecks(123456):     "1234,56",
		money.FromKopecks(-123456):    "-1234,56",
		money.FromKopecks(1000000000): "10000000,00",
	}
	for value, want := range cases {
		if got := Rubles(value); got != want {
			t.Errorf("Rubles(%d) = %q, ожидалось %q", value, got, want)
		}
	}
}
