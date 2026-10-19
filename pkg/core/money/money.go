// Package money представляет денежные величины asledger.
//
// Деньги хранятся и обрабатываются только целыми числами в копейках:
// 1 рубль = 100 копеек. Использование float32/float64 в денежной арифметике
// запрещено (проверяется отдельной целью сборки), поэтому в пакете нет
// ни одного значения с плавающей точкой.
//
// Валюта учёта — российский рубль, символ «₽».
package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Kop — денежная сумма в копейках.
type Kop int64

// Коды ошибок разбора денежных строк.
var (
	// ErrEmpty обозначает пустую или состоящую только из служебных символов строку.
	ErrEmpty = errors.New("money: пустая денежная сумма")
	// ErrFormat обозначает строку, которую нельзя разобрать как сумму.
	ErrFormat = errors.New("money: неверный формат денежной суммы")
	// ErrPrecision обозначает более двух знаков после разделителя дробной части.
	ErrPrecision = errors.New("money: более двух знаков после запятой")
)

// separators — символы, удаляемые из входной строки: пробелы-разделители
// разрядов, знак рубля и словесные обозначения валюты.
var separators = strings.NewReplacer(
	" ", "",
	"\u00a0", "", // неразрывный пробел
	"\u202f", "", // узкий неразрывный пробел
	"\u2009", "", // тонкий пробел
	"₽", "",
	"руб.", "", "руб", "",
	"р.", "", "р", "",
)

// FromKopecks создаёт сумму из целого числа копеек.
func FromKopecks(v int64) Kop { return Kop(v) }

// Kopecks возвращает сумму в копейках.
func (k Kop) Kopecks() int64 { return int64(k) }

// Parse разбирает денежную строку в сумму.
//
// Принимаются формы «123.45», «123,45», «1 250 ₽», «+10», «−5,5».
// Разделителем дробной части считается первая точка или запятая;
// допускается не более двух знаков после неё.
func Parse(s string) (Kop, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrEmpty
	}
	s = separators.Replace(s)
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrEmpty
	}

	negative := false
	switch {
	case strings.HasPrefix(s, "-"):
		negative = true
		s = s[1:]
	case strings.HasPrefix(s, "−"): // минус U+2212 занимает три байта
		negative = true
		s = s[len("−"):]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}

	intPart, fracPart := s, ""
	if i := strings.IndexAny(s, ".,"); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
		if strings.ContainsAny(fracPart, ".,") {
			return 0, fmt.Errorf("%w: %q", ErrFormat, s)
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) {
		return 0, fmt.Errorf("%w: %q", ErrFormat, s)
	}
	if fracPart == "" {
		fracPart = "00"
	}
	if !isDigits(fracPart) {
		return 0, fmt.Errorf("%w: %q", ErrFormat, s)
	}
	if len(fracPart) > 2 {
		return 0, fmt.Errorf("%w: %q", ErrPrecision, s)
	}
	if len(fracPart) == 1 {
		fracPart += "0"
	}

	v, err := strconv.ParseInt(intPart+fracPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %w", ErrFormat, s, err)
	}
	if negative {
		v = -v
	}
	return Kop(v), nil
}

func isDigits(s string) bool {
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

// String возвращает каноническое представление суммы: рубли с разделителями
// разрядов и двумя знаками после запятой, например «1 250,00 ₽».
func (k Kop) String() string {
	v := int64(k)
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	rub := v / 100
	kop := v % 100
	return fmt.Sprintf("%s%s,%02d ₽", sign, groupDigits(rub), kop)
}

// groupDigits разделяет разряды числа пробелом: 1250 → «1 250».
func groupDigits(v int64) string {
	s := strconv.FormatInt(v, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	first := len(s) % 3
	if first > 0 {
		b.WriteString(s[:first])
	}
	for i := first; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// Add возвращает сумму двух величин.
func (k Kop) Add(o Kop) Kop { return k + o }

// Sub возвращает разность двух величин.
func (k Kop) Sub(o Kop) Kop { return k - o }

// Neg возвращает величину с противоположным знаком.
func (k Kop) Neg() Kop { return -k }

// Cmp сравнивает величины: -1, 0 или 1.
func (k Kop) Cmp(o Kop) int {
	switch {
	case k < o:
		return -1
	case k > o:
		return 1
	default:
		return 0
	}
}

// IsZero сообщает, равна ли величина нулю.
func (k Kop) IsZero() bool { return k == 0 }

// IsNegative сообщает, отрицательна ли величина.
func (k Kop) IsNegative() bool { return k < 0 }

// Sum складывает произвольное число величин.
func Sum(xs ...Kop) Kop {
	var total Kop
	for _, x := range xs {
		total += x
	}
	return total
}
