package interest

import (
	"math"
	"testing"

	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_17_AccrueReference проверяет эталонный расчёт: 15 % годовых на
// 100 000 ₽ за 30 дней.
func TestASL_17_AccrueReference(t *testing.T) {
	got := Accrue(money.FromKopecks(100_000_00), Rate(150_000), 30, DaysInYear)
	want := money.FromKopecks(1_232_88)
	if got != want {
		t.Fatalf("Accrue = %s, ожидалось %s", got, want)
	}
}

// TestASL_17_AccrueRounding проверяет округление half-up до копейки.
func TestASL_17_AccrueRounding(t *testing.T) {
	cases := []struct {
		name     string
		balance  money.Kop
		rate     Rate
		days     int
		yearDays int
		want     money.Kop
	}{
		{"ровно половина вверх", 1, 500_000, 1, 1, 1},
		{"чуть меньше половины", 1, 499_999, 1, 1, 0},
		{"полтора вверх", 3, 500_000, 1, 1, 2},
		{"четверть вниз", 1, 250_000, 1, 1, 0},
		{"две четверти вверх", 2, 250_000, 1, 1, 1},
		{"нулевая ставка", 100_000_00, 0, 30, DaysInYear, 0},
		{"нулевой остаток", 0, 150_000, 30, DaysInYear, 0},
		{"отрицательный остаток", -100, 150_000, 30, DaysInYear, 0},
		{"отрицательная ставка", 100, -1, 30, DaysInYear, 0},
		{"ставка выше 100 %", 100, MaxRate + 1, 30, DaysInYear, 0},
		{"нулевые дни", 100, 150_000, 0, DaysInYear, 0},
		{"отрицательные дни", 100, 150_000, -1, DaysInYear, 0},
		{"нулевой год", 100, 150_000, 30, 0, 0},
		{"банковский год 360", money.FromKopecks(100_000_00), 150_000, 30, 360, money.FromKopecks(1_250_00)},
	}
	for _, tc := range cases {
		if got := Accrue(tc.balance, tc.rate, tc.days, tc.yearDays); got != tc.want {
			t.Fatalf("%s: Accrue = %s, ожидалось %s", tc.name, got, tc.want)
		}
	}
}

// TestASL_17_AccrueOverflow проверяет защиту от переполнения.
func TestASL_17_AccrueOverflow(t *testing.T) {
	got := Accrue(money.Kop(math.MaxInt64), MaxRate, math.MaxInt32, 1)
	if got != money.Kop(math.MaxInt64) {
		t.Fatalf("Accrue при переполнении = %d, ожидалось насыщение", got)
	}
}

// BenchmarkAccrue измеряет стоимость расчёта процентов.
func BenchmarkAccrue(b *testing.B) {
	balance := money.FromKopecks(100_000_00)
	for i := 0; i < b.N; i++ {
		_ = Accrue(balance, Rate(150_000), 30, DaysInYear)
	}
}
