package hledger

import (
	"bufio"
	"fmt"
	"io"
	"sort"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// accountColumn — колонка, в которой начинаются комментарии директив account.
const accountColumn = 46

// postingColumn — колонка, в которой начинаются суммы строк проводок.
const postingColumn = 40

// WriteOptions — параметры выгрузки книги в plaintext-формат hledger.
type WriteOptions struct {
	// Comment — текст комментария в первой строке файла.
	Comment string
	// IncludeArchived — выгружать архивные счета в директивах account.
	IncludeArchived bool
}

// Write выгружает книгу в plaintext-формат hledger.
//
// Файл состоит из директив account (сохраняют план счетов вместе с типами в
// комментарии «; тип: …») и проводок. Стороны строк кодируются знаком суммы:
// дебет — положительная сумма, кредит — отрицательная. Такой файл читается
// внешним hledger, а повторный импорт в asledger даёт идентичное состояние.
func Write(w io.Writer, accounts []model.Account, transactions []model.Transaction, opts WriteOptions) error {
	buf := bufio.NewWriter(w)

	if opts.Comment != "" {
		writeLine(buf, "; "+opts.Comment)
	}
	writeLine(buf, "; asledger: plaintext-книга hledger")

	sorted := append([]model.Account(nil), accounts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].FullPath < sorted[j].FullPath })
	declared := false
	for _, account := range sorted {
		if account.IsArchived && !opts.IncludeArchived {
			continue
		}
		line := "account " + account.FullPath
		if account.Type.Valid() {
			line = padRight(line, accountColumn) + "; тип: " + string(account.Type)
		}
		writeLine(buf, line)
		declared = true
	}
	if declared {
		writeLine(buf, "")
	}

	paths := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		paths[account.ID] = account.FullPath
	}

	ordered := append([]model.Transaction(nil), transactions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].Date.Equal(ordered[j].Date) {
			return ordered[i].Date.Before(ordered[j].Date)
		}
		return ordered[i].ID < ordered[j].ID
	})

	for i, tr := range ordered {
		if i > 0 {
			writeLine(buf, "")
		}
		header := tr.Date.Format("2006-01-02")
		if tr.Description != "" {
			header += " " + tr.Description
		}
		writeLine(buf, header)

		lines := append([]model.TransactionLine(nil), tr.Lines...)
		sort.SliceStable(lines, func(i, j int) bool {
			if lines[i].Side != lines[j].Side {
				return lines[i].Side == model.SideDebit
			}
			return lines[i].ID < lines[j].ID
		})
		for _, line := range lines {
			path, ok := paths[line.AccountID]
			if !ok {
				return fmt.Errorf("hledger: у строки проводки %d неизвестен счёт %d", tr.ID, line.AccountID)
			}
			amount := line.Amount
			if line.Side == model.SideCredit {
				amount = -amount
			}
			writeLine(buf, padRight("    "+path, postingColumn)+formatAmount(amount))
		}
	}
	return buf.Flush()
}

// formatAmount возвращает сумму в форме hledger: знак, рубли, точка, копейки.
// Разряды не разделяются, чтобы файл читался внешними инструментами в любой
// локали.
func formatAmount(value money.Kop) string {
	negative := value < 0
	abs := int64(value)
	if negative {
		abs = -abs
	}
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%d.%02d", sign, abs/100, abs%100)
}

// writeLine пишет строку в буфер; ошибку записи возвращает Flush.
func writeLine(buf *bufio.Writer, text string) {
	_, _ = buf.WriteString(text + "\n")
}

// padRight дополняет строку пробелами до нужной длины.
func padRight(text string, width int) string {
	padding := width - len([]rune(text))
	if padding < 2 {
		padding = 2
	}
	for i := 0; i < padding; i++ {
		text += " "
	}
	return text
}
