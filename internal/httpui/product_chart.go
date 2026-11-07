package httpui

import (
	"github.com/trefas/asledger/pkg/core/money"
)

// chartBar — столбец простого SVG-графика.
type chartBar struct {
	// Label — подпись под столбцом.
	Label string
	// Value — значение столбца.
	Value money.Kop
	// ValueLabel — форматированная сумма над столбцом.
	ValueLabel string
	// X, Y, Width и Height — геометрия столбца в системе координат SVG.
	X      int
	Y      int
	Width  int
	Height int
	// LabelX и LabelY — позиция подписи.
	LabelX int
	LabelY int
	// ValueX и ValueY — позиция суммы над столбцом.
	ValueX int
	ValueY int
}

// barChart — простой столбчатый график без внешних зависимостей.
type barChart struct {
	// Width и Height — размеры области SVG.
	Width  int
	Height int
	// Caption — описание графика для подписи и aria-label.
	Caption string
	// Bars — столбцы; пусто, если данных нет.
	Bars []chartBar
	// ShowValues — выводить суммы над столбцами.
	ShowValues bool
}

// chartWidth и chartHeight — размеры области графика.
const (
	chartWidth  = 640
	chartHeight = 220
	chartTop    = 18
	chartBottom = 46
	chartSide   = 12
)

// newBarChart строит столбчатый график по подписям и суммам.
//
// Высота столбца считается целочисленно, без плавающей точки: значение
// умножается на доступную высоту и делится на максимум.
func newBarChart(caption string, labels []string, values []money.Kop, showValues bool) barChart {
	chart := barChart{
		Width: chartWidth, Height: chartHeight, Caption: caption, ShowValues: showValues,
	}
	if len(labels) == 0 || len(labels) != len(values) {
		return chart
	}
	var peak money.Kop
	for _, value := range values {
		if value > peak {
			peak = value
		}
	}
	if peak <= 0 {
		return chart
	}
	available := chartHeight - chartTop - chartBottom
	slot := (chartWidth - 2*chartSide) / len(labels)
	if slot <= 0 {
		return chart
	}
	barWidth := slot * 3 / 5
	if barWidth < 8 {
		barWidth = 8
	}
	for i, label := range labels {
		value := values[i]
		height := 0
		if value > 0 {
			height = int(int64(available) * int64(value) / int64(peak))
			if height < 2 {
				height = 2
			}
		}
		x := chartSide + i*slot + (slot-barWidth)/2
		y := chartTop + available - height
		labelX := chartSide + i*slot + slot/2
		valueY := y - 4
		if valueY < 12 {
			valueY = y + 14
		}
		chart.Bars = append(chart.Bars, chartBar{
			Label:      shortChartLabel(label),
			Value:      value,
			ValueLabel: value.String(),
			X:          x,
			Y:          y,
			Width:      barWidth,
			Height:     height,
			LabelX:     labelX,
			LabelY:     chartHeight - 12,
			ValueX:     labelX,
			ValueY:     valueY,
		})
	}
	return chart
}

// shortChartLabel сокращает длинную подпись столбца до 16 символов.
func shortChartLabel(label string) string {
	runes := []rune(label)
	if len(runes) <= 16 {
		return label
	}
	return string(runes[:15]) + "…"
}
