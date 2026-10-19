// Package hledger реализует обмен книгой в plaintext-формате hledger:
// разбор импортируемой книги, конвертацию её в модель asledger и выгрузку
// книги обратно (round-trip).
//
// Разбор намеренно строг к неоднозначностям: в hledger счёт и сумма разделены
// двумя пробелами или табуляцией. Если файл записан одним пробелом, сумма
// принимается только тогда, когда она недвусмысленна (есть знак, десятичный
// разделитель или обозначение рубля) — иначе строка считается частью имени
// счёта, и проводка получает понятную ошибку с номером строки вместо тихого
// искажения данных.
package hledger

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// Posting — строка проводки hledger.
type Posting struct {
	// Line — номер строки в файле (1-based).
	Line int
	// Account — полное имя счёта, как оно записано в файле.
	Account string
	// Amount — сумма со знаком: минус означает противоположную сторону.
	// Значима только при Has = true.
	Amount money.Kop
	// Has — сумма указана в файле (или восстановлена по остальным строкам).
	Has bool
	// Implied — сумма не была указана и восстановлена как остаток проводки.
	Implied bool
}

// Transaction — проводка hledger.
type Transaction struct {
	// Line — номер строки заголовка (1-based).
	Line int
	// Date — дата проводки.
	Date time.Time
	// Description — описание (может быть пустым).
	Description string
	// Postings — строки проводки в порядке файла.
	Postings []Posting
}

// Canonical возвращает каноничный текст проводки для расчёта source_hash.
//
// Номер строки и порядок строк в файле не учитываются: строки сортируются,
// поэтому перестановка проводок в файле не меняет хеш, а правка даты,
// описания, счёта или суммы его меняет.
func (t Transaction) Canonical() string {
	parts := make([]string, 0, len(t.Postings))
	for _, posting := range t.Postings {
		parts = append(parts, fmt.Sprintf("%s=%d", posting.Account, int64(posting.Amount)))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%s|%s|%s", t.Date.Format("2006-01-02"), t.Description, strings.Join(parts, ";"))
}

// DeclaredAccount — счёт из директивы account.
type DeclaredAccount struct {
	// Line — номер строки директивы.
	Line int
	// Path — полный путь счёта.
	Path string
	// Type — тип, если он указан в комментарии директивы («; тип: расход»).
	Type model.AccountType
}

// ParseError — ошибка разбора с местом в файле.
//
// Строка и её текст сохраняются, чтобы отчёт об импорте был пригоден для
// ручной правки исходного файла.
type ParseError struct {
	Line    int
	Message string
	Snippet string
}

// Error возвращает сообщение с номером строки.
func (e ParseError) Error() string {
	if e.Line == 0 {
		return e.Message
	}
	return fmt.Sprintf("строка %d: %s", e.Line, e.Message)
}

// Book — результат разбора файла.
//
// Разбор не прерывается на первой ошибке: ошибки собираются, а нечитаемые
// проводки пропускаются, чтобы отчёт об импорте показал все проблемы сразу.
type Book struct {
	// Transactions — успешно разобранные проводки.
	Transactions []Transaction
	// Declared — счета из директив account.
	Declared []DeclaredAccount
	// Errors — ошибки разбора с номерами строк.
	Errors []ParseError
	// Dropped — число блоков, которые не стали проводками из-за ошибок.
	Dropped int
}

// Accounts возвращает все имена счетов книги (объявленные и использованные),
// отсортированные по пути.
func (b *Book) Accounts() []string {
	seen := make(map[string]bool)
	var paths []string
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	for _, declared := range b.Declared {
		add(declared.Path)
	}
	for _, tr := range b.Transactions {
		for _, posting := range tr.Postings {
			add(posting.Account)
		}
	}
	sort.Strings(paths)
	return paths
}

// Parse разбирает книгу hledger в plaintext-формате.
func Parse(data []byte) *Book {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	book := &Book{}
	declared := make(map[string]bool)
	inCommentBlock := false

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		case inCommentBlock:
			if strings.EqualFold(trimmed, "end comment") {
				inCommentBlock = false
			}
			i++
		case trimmed == "":
			i++
		case strings.HasPrefix(trimmed, ";"):
			i++
		case strings.EqualFold(trimmed, "comment"):
			inCommentBlock = true
			i++
		case isIndented(line):
			book.Errors = append(book.Errors, ParseError{
				Line: i + 1, Message: "строка проводки без заголовка проводки", Snippet: line,
			})
			i++
		case trimmed[0] >= '0' && trimmed[0] <= '9':
			tr, errs, next := parseTransaction(lines, i)
			book.Errors = append(book.Errors, errs...)
			if tr != nil {
				book.Transactions = append(book.Transactions, *tr)
			} else {
				book.Dropped++
			}
			i = next
		default:
			field, rest, ok := directive(trimmed)
			if !ok {
				book.Errors = append(book.Errors, ParseError{
					Line: i + 1, Message: "не удалось разобрать строку", Snippet: line,
				})
				i++
				continue
			}
			switch field {
			case "account":
				path, comment := splitComment(rest)
				path = strings.TrimSpace(path)
				if path == "" {
					book.Errors = append(book.Errors, ParseError{
						Line: i + 1, Message: "в директиве account не указан счёт", Snippet: line,
					})
					break
				}
				if !declared[path] {
					declared[path] = true
					book.Declared = append(book.Declared, DeclaredAccount{
						Line: i + 1, Path: path, Type: TypeFromComment(comment),
					})
				}
			case "include":
				book.Errors = append(book.Errors, ParseError{
					Line:    i + 1,
					Message: "директива include не поддерживается: подключённый файл нужно импортировать отдельно",
					Snippet: line,
				})
			}
			i++
		}
	}
	return book
}

// parseTransaction разбирает проводку, начиная со строки заголовка.
//
// Возвращаются проводка (nil при ошибке разбора заголовка), ошибки строк и
// номер следующей неразобранной строки.
func parseTransaction(lines []string, start int) (*Transaction, []ParseError, int) {
	header := lines[start]
	next := skipBlock(lines, start+1)

	date, description, err := parseHeader(header)
	if err != nil {
		return nil, []ParseError{{Line: start + 1, Message: err.Error(), Snippet: header}}, next
	}

	tr := &Transaction{Line: start + 1, Date: date, Description: description}
	var errs []ParseError

	for i := start + 1; i < next; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") {
			continue
		}
		posting, err := parsePosting(line, i+1)
		if err != nil {
			errs = append(errs, ParseError{Line: i + 1, Message: err.Error(), Snippet: line})
			continue
		}
		tr.Postings = append(tr.Postings, posting)
	}

	if len(errs) == 0 {
		errs = append(errs, validateParsed(tr)...)
	}
	if len(errs) > 0 {
		// Проводка с ошибкой в любой строке не импортируется: суммы могут быть
		// восстановлены неверно, поэтому лучше отчёт с причиной и номером строки.
		return nil, errs, next
	}
	return tr, nil, next
}

// skipBlock возвращает номер строки после блока проводки: строки блока
// начинаются с отступа и заканчиваются пустой строкой или строкой в первой
// колонке.
func skipBlock(lines []string, from int) int {
	i := from
	for i < len(lines) && isIndented(lines[i]) {
		i++
	}
	return i
}

// validateParsed проверяет полноту проводки и восстанавливает пропущенную сумму.
func validateParsed(tr *Transaction) []ParseError {
	var errs []ParseError
	if len(tr.Postings) < 2 {
		return []ParseError{{
			Line:    tr.Line,
			Message: fmt.Sprintf("в проводке %d строк, нужно минимум две", len(tr.Postings)),
		}}
	}

	missing := -1
	var sum money.Kop
	for i := range tr.Postings {
		if !tr.Postings[i].Has {
			if missing >= 0 {
				return append(errs, ParseError{
					Line:    tr.Postings[i].Line,
					Message: "сумма не указана более чем в одной строке — восстановить её нельзя",
				})
			}
			missing = i
			continue
		}
		sum += tr.Postings[i].Amount
	}
	if missing >= 0 {
		tr.Postings[missing].Amount = -sum
		tr.Postings[missing].Has = true
		tr.Postings[missing].Implied = true
	}
	return errs
} // parseHeader разбирает строку заголовка проводки: дату, флаги и описание.
func parseHeader(line string) (time.Time, string, error) {
	text, _ := splitComment(line)
	text = strings.TrimSpace(text)

	dateText, rest := text, ""
	if idx := strings.IndexAny(text, " \t"); idx >= 0 {
		dateText, rest = text[:idx], strings.TrimSpace(text[idx:])
	}
	if idx := strings.Index(dateText, "="); idx >= 0 {
		dateText = dateText[:idx]
	}

	date, err := parseDateText(dateText)
	if err != nil {
		return time.Time{}, "", err
	}

	rest = strings.TrimSpace(rest)
	if rest != "" && (rest[0] == '*' || rest[0] == '!') {
		rest = strings.TrimSpace(rest[1:])
	}
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end >= 0 {
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	return date, rest, nil
}

// parsePosting разбирает строку проводки: счёт, сумму, служебные пометки.
func parsePosting(line string, lineNo int) (Posting, error) {
	posting := Posting{Line: lineNo}

	text, _ := splitComment(line)
	text = cutAnnotation(text)

	account, amountText := splitAccountAmount(text)
	account = strings.TrimSpace(account)
	if account == "" {
		return posting, errors.New("не указан счёт строки")
	}
	if strings.HasPrefix(account, "(") || strings.HasPrefix(account, "[") {
		return posting, errors.New("виртуальные проводки не поддерживаются")
	}
	posting.Account = account

	if amountText == "" {
		return posting, nil
	}
	amount, err := parseAmountText(amountText)
	if err != nil {
		return posting, err
	}
	posting.Amount = amount
	posting.Has = true
	return posting, nil
}

// splitAccountAmount разделяет строку проводки на счёт и сумму.
//
// Сначала применяется правило hledger: два и более пробелов или табуляция.
// Если суммы там нет, ищется самый длинный правый фрагмент, разбираемый как
// недвусмысленная сумма.
func splitAccountAmount(text string) (account, amount string) {
	trimmed := strings.TrimSpace(text)
	if idx := indexSeparator(trimmed); idx >= 0 {
		left := strings.TrimRight(trimmed[:idx], " \t")
		rest := strings.TrimSpace(trimmed[idx:])
		// После разделителя идёт сумма: если она не разбирается, ошибку
		// сообщит parseAmountText с понятным текстом.
		if left != "" && rest != "" {
			return left, rest
		}
	}

	fields := strings.Fields(trimmed)
	for i := 1; i < len(fields); i++ {
		candidate := strings.Join(fields[i:], " ")
		if !unambiguousAmount(candidate) {
			continue
		}
		if _, err := parseAmountText(candidate); err == nil {
			return strings.Join(fields[:i], " "), candidate
		}
	}
	return trimmed, ""
}

// unambiguousAmount сообщает, что фрагмент похож на сумму однозначно: есть
// знак, десятичный разделитель или обозначение рубля. Голое число после одного
// пробела достаточно спорно, поэтому не принимается.
func unambiguousAmount(s string) bool {
	if strings.ContainsAny(s, ".,₽+-") {
		return true
	}
	lower := strings.ToLower(s)
	return strings.Contains(lower, "руб") || strings.Contains(lower, "р.")
}

// parseAmountText разбирает сумму hledger, приводя её к рублям без потери
// точности. Поддерживаются формы «1234.56», «1 234,56», «1,234.56», «1234 ₽»,
// «-1 234,56», «+10».
func parseAmountText(raw string) (money.Kop, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, errors.New("не задана сумма")
	}
	text = currencyReplacer.Replace(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, errors.New("не задана сумма")
	}
	if idx := indexLetter(text); idx >= 0 {
		return 0, fmt.Errorf("валюта «%s» не поддерживается: учёт ведётся в рублях", wordAt(text, idx))
	}
	text = normalizeSeparators(text)
	if text == "" {
		return 0, errors.New("не задана сумма")
	}
	value, err := money.Parse(text)
	if err != nil {
		return 0, fmt.Errorf("не удалось разобрать сумму %q: %w", strings.TrimSpace(raw), err)
	}
	return value, nil
}

// currencyReplacer удаляет обозначения рубля, оставляя цифры и знаки.
var currencyReplacer = strings.NewReplacer(
	"₽", "", "RUR", "", "RUB", "", "руб.", "", "руб", "", "р.", "", "р", "",
)

// normalizeSeparators приводит разделители разрядов и дробной части к форме,
// которую понимает money.Parse.
//
// Разделителем разрядов считается только пробел (форма «1 234 567,89»). Если
// в числе есть и точка, и запятая — западная форма «1,234.56», — разряды
// отделяет тот знак, который стоит левее, а дробную часть — правый. Одиночный
// знак без пробелов всегда считается десятичным: «1,234» даёт ошибку точности,
// а не молчаливое умножение на тысячу.
func normalizeSeparators(s string) string {
	s = strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(s)

	dot := strings.LastIndex(s, ".")
	comma := strings.LastIndex(s, ",")
	if dot >= 0 && comma >= 0 {
		if dot > comma {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ".", "")
		}
	}
	return s
}

// splitComment отделяет комментарий строки (всё после «;»).
func splitComment(line string) (text, comment string) {
	if idx := strings.Index(line, ";"); idx >= 0 {
		return line[:idx], line[idx+1:]
	}
	return line, ""
}

// cutAnnotation отрезает от строки проводки проверку остатка («= 100») и цену
// («@ 5», «@@ 5»): для модели asledger они не нужны.
func cutAnnotation(text string) string {
	cut := -1
	for i := 0; i < len(text); i++ {
		if text[i] != '=' && text[i] != '@' {
			continue
		}
		if i > 0 && (text[i-1] == ' ' || text[i-1] == '\t') {
			cut = i
			break
		}
	}
	if cut < 0 {
		return text
	}
	return text[:cut]
}

// indexSeparator возвращает позицию начала разделителя «счёт — сумма»:
// двух и более пробелов или табуляции.
func indexSeparator(text string) int {
	for i := 0; i < len(text); i++ {
		if text[i] != ' ' && text[i] != '\t' {
			continue
		}
		j := i
		for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
			j++
		}
		if text[i] == '\t' || j-i >= 2 {
			return i
		}
	}
	return -1
}

// isIndented сообщает, что строка начинается с пробела или табуляции.
func isIndented(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

// parseDateText разбирает дату в форматах 2026-01-15, 2026/01/15, 2026.01.15
// и допускает однозначные части без ведущего нуля.
func parseDateText(text string) (time.Time, error) {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '-' || r == '/' || r == '.'
	})
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("неверная дата %q: ожидается ГГГГ-ММ-ДД", text)
	}
	numbers := make([]int, 3)
	for i, part := range parts {
		if part == "" || !isDigits(part) {
			return time.Time{}, fmt.Errorf("неверная дата %q: ожидается ГГГГ-ММ-ДД", text)
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return time.Time{}, fmt.Errorf("неверная дата %q: ожидается ГГГГ-ММ-ДД", text)
		}
		numbers[i] = value
	}

	year, month, day := numbers[0], numbers[1], numbers[2]
	if len(parts[0]) < 4 || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("неверная дата %q: ожидается ГГГГ-ММ-ДД", text)
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return time.Time{}, fmt.Errorf("неверная дата %q: такой даты не существует", text)
	}
	return date, nil
}

// isDigits сообщает, что строка непустая и состоит только из цифр.
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

// indexLetter возвращает позицию первой буквы в строке или -1.
func indexLetter(s string) int {
	for i, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || r == 'ё' || r == 'Ё' {
			return i
		}
	}
	return -1
}

// wordAt выделяет слово, начинающееся в указанной позиции.
func wordAt(s string, idx int) string {
	rest := s[idx:]
	end := strings.IndexAny(rest, " \t")
	word := rest
	if end >= 0 {
		word = rest[:end]
	}
	return strings.TrimSpace(word)
}

// directive распознаёт директиву hledger в начале строки.
func directive(text string) (field, rest string, ok bool) {
	if text == "" {
		return "", "", false
	}
	field, rest = text, ""
	if idx := strings.IndexAny(text, " \t"); idx >= 0 {
		field, rest = text[:idx], text[idx:]
	}
	switch strings.ToLower(field) {
	case "account", "include", "alias", "payee", "tag", "commodity", "apply",
		"end", "d", "p", "y", "n", "c":
		return strings.ToLower(field), rest, true
	default:
		return "", "", false
	}
}

// TypeFromComment извлекает тип счёта из комментария директивы account.
//
// Форма записи, которую использует экспорт asledger: «; тип: расход».
func TypeFromComment(comment string) model.AccountType {
	text := strings.ToLower(strings.TrimSpace(comment))
	if text == "" {
		return ""
	}
	idx := strings.Index(text, "тип")
	if idx < 0 {
		return ""
	}
	rest := text[idx+len("тип"):]
	rest = strings.TrimSpace(rest)
	rest = strings.TrimLeft(rest, ":=")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	if end := strings.IndexAny(rest, " \t,;"); end >= 0 {
		rest = rest[:end]
	}
	for _, valid := range model.AccountTypes() {
		if string(valid) == rest {
			return valid
		}
	}
	return ""
}
