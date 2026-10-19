package money

import (
	"errors"
	"testing"
)

// TestASL_14_money_123_45 проверяет критерий приёмки эпика:
// сумма «123.45» хранится как «12345».
func TestASL_14_money_123_45(t *testing.T) {
	got, err := Parse("123.45")
	if err != nil {
		t.Fatalf("Parse(«123.45») вернул ошибку: %v", err)
	}
	if want := Kop(12345); got != want {
		t.Fatalf("Parse(«123.45») = %d, ожидалось %d", got, want)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Kop
	}{
		{"точка как разделитель", "123.45", 12345},
		{"запятая как разделитель", "123,45", 12345},
		{"рубль со знаками разрядов", "1 250 ₽", 125000},
		{"явный плюс", "+10", 1000},
		{"отрицательная с одним знаком после запятой", "-5,5", -550},
		{"минус U+2212", "−5,5", -550},
		{"ноль", "0", 0},
		{"без дробной части", "42", 4200},
		{"два знака после запятой", "1 250,7", 125070},
		{"ведущие нули", "0007,05", 705},
		{"неразрывный пробел", "1\u00a0000,00 ₽", 100000},
		{"словесное обозначение рублей", "15 руб.", 1500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q) вернул ошибку: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("Parse(%q) = %d, ожидалось %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"пустая строка", "", ErrEmpty},
		{"только символ валюты", "₽", ErrEmpty},
		{"текст", "abc", ErrFormat},
		{"буквы в дробной части", "12,4x", ErrFormat},
		{"три знака после запятой", "12.345", ErrPrecision},
		{"два разделителя", "1.234,56", ErrFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse(%q) = %v, ожидалась ошибка %v", tc.in, err, tc.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		in   Kop
		want string
	}{
		{0, "0,00 ₽"},
		{12345, "123,45 ₽"},
		{125000, "1 250,00 ₽"},
		{-550, "-5,50 ₽"},
		{100000000, "1 000 000,00 ₽"},
		{5, "0,05 ₽"},
	}
	for _, tc := range tests {
		if got := tc.in.String(); got != tc.want {
			t.Fatalf("%d.String() = %q, ожидалось %q", tc.in, got, tc.want)
		}
	}
}

// TestParseStringRoundTrip проверяет обратимость разбора и форматирования.
func TestParseStringRoundTrip(t *testing.T) {
	for _, v := range []Kop{0, 5, 12345, 125000, -550, 999999999} {
		got, err := Parse(v.String())
		if err != nil {
			t.Fatalf("Parse(%q) вернул ошибку: %v", v.String(), err)
		}
		if got != v {
			t.Fatalf("обратимость нарушена: %d → %q → %d", v, v.String(), got)
		}
	}
}

func TestArithmetic(t *testing.T) {
	a, b := Kop(125000), Kop(12345)

	if got := a.Add(b); got != 137345 {
		t.Fatalf("Add = %d, ожидалось 137345", got)
	}
	if got := a.Sub(b); got != 112655 {
		t.Fatalf("Sub = %d, ожидалось 112655", got)
	}
	if got := b.Neg(); got != -12345 {
		t.Fatalf("Neg = %d, ожидалось -12345", got)
	}
	if got := Sum(a, b, b); got != 149690 {
		t.Fatalf("Sum = %d, ожидалось 149690", got)
	}
	if a.Cmp(b) != 1 || b.Cmp(a) != -1 || a.Cmp(a) != 0 {
		t.Fatal("Cmp вернул неверный порядок")
	}
	if !Kop(0).IsZero() || Kop(-1).IsZero() {
		t.Fatal("IsZero работает неверно")
	}
	if !Kop(-1).IsNegative() || Kop(1).IsNegative() {
		t.Fatal("IsNegative работает неверно")
	}
}

// TestNoFloatArithmetic подтверждает отсутствие потерь точности на дробях,
// которые часто ломают арифметику с плавающей точкой: 0,1 + 0,2.
func TestNoFloatArithmetic(t *testing.T) {
	a, _ := Parse("0,1")
	b, _ := Parse("0,2")
	if got, want := a.Add(b), Kop(30); got != want {
		t.Fatalf("0,1 + 0,2 = %d, ожидалось %d", got, want)
	}
}
