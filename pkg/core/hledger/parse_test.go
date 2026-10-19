package hledger

import (
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// bookExample — учебная книга, покрывающая основные конструкции hledger.
const bookExample = `; книга домашней бухгалтерии
account Активы:Наличные
account Активы:Карта
account Расходы:Продукты ; тип: расход

2026-01-15 * Пятёрочка
    Расходы:Продукты            1 234,56 ₽
    Активы:Наличные

2026-01-16 ! Зарплата ; начисление
    Активы:Карта        100 000,00 ₽  ; за январь
    Доходы:Зарплата

2026-01-17 Перевод на карту
    Активы:Карта          5 000,00 ₽
    Активы:Наличные

2026-01-18 (123) Оплата подписки
    Расходы:Связь       500.00 RUB = 500.00
    Активы:Карта
`

func TestParseBook(t *testing.T) {
	book := Parse([]byte(bookExample))
	if len(book.Errors) != 0 {
		t.Fatalf("ожидались ошибки разбора: %v", book.Errors)
	}
	if len(book.Transactions) != 4 {
		t.Fatalf("разобрано проводок %d, ожидалось 4", len(book.Transactions))
	}

	first := book.Transactions[0]
	if got := first.Date.Format("2006-01-02"); got != "2026-01-15" {
		t.Errorf("дата первой проводки %q", got)
	}
	if first.Description != "Пятёрочка" {
		t.Errorf("описание первой проводки %q", first.Description)
	}
	if len(first.Postings) != 2 {
		t.Fatalf("строк в первой проводке %d, ожидалось 2", len(first.Postings))
	}
	if first.Postings[0].Account != "Расходы:Продукты" {
		t.Errorf("счёт первой строки %q", first.Postings[0].Account)
	}
	if got, want := first.Postings[0].Amount, money.FromKopecks(123456); got != want {
		t.Errorf("сумма первой строки %s, ожидалось %s", got, want)
	}
	if first.Postings[0].Implied {
		t.Error("сумма первой строки не должна быть восстановленной")
	}
	// Сумма второй строки восстановлена как остаток проводки.
	if !first.Postings[1].Implied {
		t.Error("сумма второй строки должна быть восстановлена")
	}
	if got, want := first.Postings[1].Amount, money.FromKopecks(-123456); got != want {
		t.Errorf("восстановленная сумма %s, ожидалось %s", got, want)
	}

	fourth := book.Transactions[3]
	if fourth.Description != "Оплата подписки" {
		t.Errorf("описание четвёртой проводки %q", fourth.Description)
	}
	if got, want := fourth.Postings[0].Amount, money.FromKopecks(50000); got != want {
		t.Errorf("сумма строки с проверкой остатка %s, ожидалось %s", got, want)
	}
}

func TestParseAccountsAndTypes(t *testing.T) {
	book := Parse([]byte(bookExample))
	accounts := book.Accounts()
	want := []string{
		"Активы:Карта",
		"Активы:Наличные",
		"Доходы:Зарплата",
		"Расходы:Продукты",
		"Расходы:Связь",
	}
	if strings.Join(accounts, ",") != strings.Join(want, ",") {
		t.Errorf("счета книги %v, ожидались %v", accounts, want)
	}

	if len(book.Declared) != 3 {
		t.Fatalf("объявленных счетов %d, ожидалось 3", len(book.Declared))
	}
	if book.Declared[2].Type != model.TypeExpense {
		t.Errorf("тип из комментария директивы %q, ожидался «расход»", book.Declared[2].Type)
	}
	if book.Declared[0].Type != "" {
		t.Errorf("тип без комментария должен быть пустым, получено %q", book.Declared[0].Type)
	}
}

func TestParseAmountsInKopecks(t *testing.T) {
	cases := []struct {
		text string
		want money.Kop
	}{
		{"1 234,56", money.FromKopecks(123456)},
		{"1234.56", money.FromKopecks(123456)},
		{"1,234.56", money.FromKopecks(123456)},
		{"1.234,56", money.FromKopecks(123456)},
		{"1 234 567,89", money.FromKopecks(123456789)},
		{"1 234", money.FromKopecks(123400)},
		{"500.00 RUB", money.FromKopecks(50000)},
		{"₽100", money.FromKopecks(10000)},
		{"100 ₽", money.FromKopecks(10000)},
		{"-1 234,56", money.FromKopecks(-123456)},
		{"+10", money.FromKopecks(1000)},
		{"0.5", money.FromKopecks(50)},
		{"0,01", money.FromKopecks(1)},
		{"12345678,90", money.FromKopecks(1234567890)},
	}
	for _, c := range cases {
		got, err := parseAmountText(c.text)
		if err != nil {
			t.Errorf("разбор %q: %v", c.text, err)
			continue
		}
		if got != c.want {
			t.Errorf("разбор %q дал %s, ожидалось %s", c.text, got, c.want)
		}
	}
}

func TestParseAmountErrors(t *testing.T) {
	cases := []string{"100 USD", "€100", "сто рублей", "10,123", "1 234,567", "1,234"}
	for _, text := range cases {
		if got, err := parseAmountText(text); err == nil {
			t.Errorf("разбор %q не дал ошибки, получено %s", text, got)
		}
	}
}

func TestParseErrorsWithLineNumbers(t *testing.T) {
	journal := `2026-01-15 Сломанная сумма
    Расходы:Продукты      сто
    Активы:Наличные

2026-13-40 Несуществующая дата
    Расходы:Продукты      100,00
    Активы:Наличные

2026-02-01 Одна строка
    Активы:Наличные   100,00

2026-02-02 Виртуальная проводка
    (Расходы:Продукты)   100,00
    Активы:Наличные

2026-02-03 Иностранная валюта
    Расходы:Продукты     100,00 USD
    Активы:Наличные

2026-02-04 Две строки без сумм
    Расходы:Продукты
    Активы:Наличные

мусор в книге
`
	book := Parse([]byte(journal))
	if len(book.Transactions) != 0 {
		t.Fatalf("разобрано проводок %d, ожидалось 0", len(book.Transactions))
	}
	if len(book.Errors) != 7 {
		for _, err := range book.Errors {
			t.Logf("ошибка: %v", err)
		}
		t.Fatalf("ошибок разбора %d, ожидалось 7", len(book.Errors))
	}

	var messages []string
	for _, err := range book.Errors {
		if err.Line == 0 {
			t.Errorf("ошибка без номера строки: %v", err)
		}
		messages = append(messages, err.Error())
	}
	for _, want := range []string{
		"строка 2: валюта «сто» не поддерживается",
		"строка 5: неверная дата",
		"строка 9: в проводке 1 строк, нужно минимум две",
		"строка 13: виртуальные проводки не поддерживаются",
		"строка 17: валюта «USD» не поддерживается",
		"строка 22: сумма не указана более чем в одной строке",
		"строка 24: не удалось разобрать строку",
	} {
		found := false
		for _, message := range messages {
			if strings.Contains(message, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("нет ошибки с текстом %q; получено: %v", want, messages)
		}
	}
}

func TestParseSingleSpaceAmountRequirements(t *testing.T) {
	// Одним пробелом отделяется только недвусмысленная сумма.
	journal := `2026-03-01 Кофе
    Расходы:Продукты -250,00
    Активы:Наличные

2026-03-02 Счёт с цифрой
    Активы:Кошелёк 2    100,00
    Активы:Наличные
`
	book := Parse([]byte(journal))
	if len(book.Errors) != 0 {
		t.Fatalf("ошибки разбора: %v", book.Errors)
	}
	if got := book.Transactions[0].Postings[0].Account; got != "Расходы:Продукты" {
		t.Errorf("счёт строки %q", got)
	}
	if got, want := book.Transactions[0].Postings[0].Amount, money.FromKopecks(-25000); got != want {
		t.Errorf("сумма строки %s, ожидалось %s", got, want)
	}
	// «Активы:Кошелёк 2» — имя счёта, а не сумма: голое число после одного
	// пробела суммой не считается.
	if got := book.Transactions[1].Postings[0].Account; got != "Активы:Кошелёк 2" {
		t.Errorf("счёт строки с цифрой %q", got)
	}
}

func TestParseCanonicalIgnoresOrder(t *testing.T) {
	one := Parse([]byte("2026-01-15 Тест\n    Расходы:Продукты   100,00\n    Активы:Наличные\n"))
	two := Parse([]byte("2026-01-15 Тест\n    Активы:Наличные\n    Расходы:Продукты   100,00\n"))
	if one.Transactions[0].Canonical() != two.Transactions[0].Canonical() {
		t.Error("каноничный текст проводки зависит от порядка строк")
	}

	changed := Parse([]byte("2026-01-15 Тест\n    Расходы:Продукты   101,00\n    Активы:Наличные\n"))
	if one.Transactions[0].Canonical() == changed.Transactions[0].Canonical() {
		t.Error("каноничный текст не учитывает изменение суммы")
	}
}

func TestParseCommentBlockAndInclude(t *testing.T) {
	journal := `comment
2026-01-15 Это не проводка
end comment
include other.journal

2026-02-01 Настоящая проводка
    Расходы:Продукты   100,00
    Активы:Наличные
`
	book := Parse([]byte(journal))
	if len(book.Transactions) != 1 {
		t.Fatalf("разобрано проводок %d, ожидалась 1", len(book.Transactions))
	}
	if len(book.Errors) != 1 || !strings.Contains(book.Errors[0].Error(), "include") {
		t.Errorf("ожидалась одна ошибка про include, получено %v", book.Errors)
	}
}

func TestParseSecondaryDateAndDateFormats(t *testing.T) {
	journal := `2026/03/05 Описание
    Расходы:Продукты   100,00
    Активы:Наличные

2026.03.06=2026.03.07 Описание
    Расходы:Продукты   100,00
    Активы:Наличные
`
	book := Parse([]byte(journal))
	if len(book.Errors) != 0 {
		t.Fatalf("ошибки разбора: %v", book.Errors)
	}
	want := []string{"2026-03-05", "2026-03-06"}
	for i, tr := range book.Transactions {
		if got := tr.Date.Format("2006-01-02"); got != want[i] {
			t.Errorf("дата проводки %d равна %s, ожидалась %s", i, got, want[i])
		}
	}
	if !book.Transactions[0].Date.Equal(time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("дата разобрана неверно: %s", book.Transactions[0].Date)
	}
}
