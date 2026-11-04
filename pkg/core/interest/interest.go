// Package interest реализует расчёт процентов по кредитам и вкладам.
//
// Ставки задаются в ppm (1 ppm = 0,0001 %), суммы — в копейках. Все
// вычисления ведутся целыми числами: промежуточные произведения считаются
// через math/big.Int, а результат округляется half-up до копейки. Плавающая
// точка не используется.
package interest

import (
	"math"
	"math/big"

	"github.com/trefas/asledger/pkg/core/money"
)

// ppmScale — количество ppm в 100 %.
const ppmScale int64 = 1_000_000

// DaysInYear — базовое число дней в году для расчёта процентов.
const DaysInYear = 365

// Rate — процентная ставка в ppm.
//
// 1 ppm = 0,0001 %, то есть 100 % = 1 000 000 ppm.
type Rate int64

// MaxRate — максимальная допустимая ставка: 100 % годовых.
const MaxRate Rate = Rate(ppmScale)

// Valid сообщает, что ставка неотрицательна и не превышает 100 %.
func (r Rate) Valid() bool { return r >= 0 && r <= MaxRate }

// Accrue считает проценты на остаток за days дней.
//
// Формула: balance × rate × days / (yearDays × 1 000 000). Результат
// округляется half-up до копейки. Отрицательный остаток, неположительное
// число дней, некорректная ставка или yearDays <= 0 дают 0.
//
// Переполнение промежуточных произведений исключено: они считаются в
// math/big.Int. Если итог не помещается в int64, возвращается math.MaxInt64.
func Accrue(balance money.Kop, rate Rate, days, yearDays int) money.Kop {
	if balance <= 0 || !rate.Valid() || rate == 0 || days <= 0 || yearDays <= 0 {
		return 0
	}

	numerator := new(big.Int).SetInt64(int64(balance))
	numerator.Mul(numerator, big.NewInt(int64(rate)))
	numerator.Mul(numerator, big.NewInt(int64(days)))

	denominator := new(big.Int).Mul(big.NewInt(int64(yearDays)), big.NewInt(ppmScale))

	// half-up: (2 × numerator + denominator) / (2 × denominator).
	numerator.Lsh(numerator, 1)
	numerator.Add(numerator, denominator)
	denominator.Lsh(denominator, 1)
	numerator.Quo(numerator, denominator)

	if !numerator.IsInt64() {
		return money.Kop(math.MaxInt64)
	}
	return money.Kop(numerator.Int64())
}
