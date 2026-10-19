// Package export формирует файлы выгрузки книги: CSV журнала проводок,
// остатков и оборотов.
//
// Файлы пригодны для электронных таблиц: разделитель полей — точка с запятой,
// первым байтом идёт метка порядка байтов UTF-8 (Excel открывает кириллицу без
// искажений), суммы записываются в рублях с копейками через запятую.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// bom — метка порядка байтов UTF-8 (U+FEFF).
const bom = "\ufeff"

// separator — разделитель полей CSV: точка с запятой, как ожидает Excel в
// русской локали.
const separator = ';'

// Rubles возвращает сумму в рублях с копейками: «1234,56», «-1234,56».
//
// Разряды не разделяются, чтобы значение читалось электронной таблицей как
// число, а не как текст.
func Rubles(value money.Kop) string {
	sign := ""
	absolute := int64(value)
	if absolute < 0 {
		sign = "-"
		absolute = -absolute
	}
	return fmt.Sprintf("%s%d,%02d", sign, absolute/100, absolute%100)
}

// Journal пишет CSV журнала проводок: по строке на каждую строку проводки.
func Journal(w io.Writer, accounts []model.Account, transactions []model.Transaction) error {
	writer, err := newWriter(w)
	if err != nil {
		return err
	}
	if err := writer.Write([]string{
		"проводка", "uuid", "дата", "тип операции", "описание",
		"автор", "источник", "счёт", "сторона", "сумма",
	}); err != nil {
		return csvError(err)
	}

	paths := pathsByID(accounts)

	ordered := append([]model.Transaction(nil), transactions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].Date.Equal(ordered[j].Date) {
			return ordered[i].Date.Before(ordered[j].Date)
		}
		return ordered[i].ID < ordered[j].ID
	})

	for _, tr := range ordered {
		for _, line := range tr.Lines {
			path, ok := paths[line.AccountID]
			if !ok {
				return fmt.Errorf("export: у строки проводки %d неизвестен счёт %d", tr.ID, line.AccountID)
			}
			row := []string{
				strconv.FormatInt(tr.ID, 10),
				tr.UUID,
				tr.Date.Format("2006-01-02"),
				string(tr.OpType),
				tr.Description,
				strconv.FormatInt(tr.ActorID, 10),
				tr.Source,
				path,
				string(line.Side),
				Rubles(line.Amount),
			}
			if err := writer.Write(row); err != nil {
				return csvError(err)
			}
		}
	}
	return flush(writer)
}

// Balances пишет CSV остатков счетов.
func Balances(w io.Writer, accounts []model.Account, balances map[int64]money.Kop) error {
	writer, err := newWriter(w)
	if err != nil {
		return err
	}
	if err := writer.Write([]string{"счёт", "тип", "остаток"}); err != nil {
		return csvError(err)
	}
	for _, account := range sortedAccounts(accounts) {
		row := []string{account.FullPath, string(account.Type), Rubles(balances[account.ID])}
		if err := writer.Write(row); err != nil {
			return csvError(err)
		}
	}
	return flush(writer)
}

// Turnovers пишет CSV оборотов счетов за период.
func Turnovers(w io.Writer, accounts []model.Account, turnovers map[int64]store.Turnover) error {
	writer, err := newWriter(w)
	if err != nil {
		return err
	}
	if err := writer.Write([]string{
		"счёт", "тип", "остаток на начало", "дебет", "кредит", "остаток на конец",
	}); err != nil {
		return csvError(err)
	}
	for _, account := range sortedAccounts(accounts) {
		turnover := turnovers[account.ID]
		closing := store.BalanceFor(account.Type,
			turnover.OpeningDebit+turnover.PeriodDebit,
			turnover.OpeningCredit+turnover.PeriodCredit)
		row := []string{
			account.FullPath,
			string(account.Type),
			Rubles(store.BalanceFor(account.Type, turnover.OpeningDebit, turnover.OpeningCredit)),
			Rubles(turnover.PeriodDebit),
			Rubles(turnover.PeriodCredit),
			Rubles(closing),
		}
		if err := writer.Write(row); err != nil {
			return csvError(err)
		}
	}
	return flush(writer)
}

// newWriter создаёт CSV-писатель с меткой UTF-8 и разделителем «;».
func newWriter(w io.Writer) (*csv.Writer, error) {
	if _, err := io.WriteString(w, bom); err != nil {
		return nil, fmt.Errorf("export: не удалось записать метку UTF-8: %w", err)
	}
	writer := csv.NewWriter(w)
	writer.Comma = separator
	return writer, nil
}

// flush завершает запись и возвращает накопленную ошибку.
func flush(writer *csv.Writer) error {
	writer.Flush()
	if err := writer.Error(); err != nil {
		return csvError(err)
	}
	return nil
}

// csvError оборачивает ошибку записи CSV.
func csvError(err error) error {
	return fmt.Errorf("export: не удалось записать CSV: %w", err)
}

// pathsByID строит карту «идентификатор счёта → полный путь».
func pathsByID(accounts []model.Account) map[int64]string {
	paths := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		paths[account.ID] = account.FullPath
	}
	return paths
}

// sortedAccounts возвращает счета по возрастанию пути.
func sortedAccounts(accounts []model.Account) []model.Account {
	ordered := append([]model.Account(nil), accounts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].FullPath < ordered[j].FullPath })
	return ordered
}
