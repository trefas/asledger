package hledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/sync"
)

// Source — значение источника проводок, импортированных из hledger.
//
// Колонка source отделяет импорт от web- и API-проводок, а пара
// «источник + source_hash» делает повторный импорт идемпотентным.
const Source = "hledger"

// Mode — режим импорта (раздел 8 эпика).
type Mode string

// Режимы импорта.
const (
	// ModeStrict — строгий: любая проблема (неизвестный счёт, ошибка разбора,
	// невалидная проводка) отклоняет импорт целиком, книга не меняется.
	ModeStrict Mode = "строгий"
	// ModeFlexible — гибкий: недостающие счета создаются, невалидные проводки
	// пропускаются с причиной и попадают в отчёт.
	ModeFlexible Mode = "гибкий"
)

// ParseMode разбирает режим импорта; пустая строка — «гибкий».
func ParseMode(text string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "", string(ModeFlexible), "flexible":
		return ModeFlexible, nil
	case string(ModeStrict), "strict":
		return ModeStrict, nil
	default:
		return "", fmt.Errorf("hledger: неизвестный режим импорта %q (ожидается «строгий» или «гибкий»)", text)
	}
}

// Options — параметры импорта.
type Options struct {
	// Mode — режим импорта; пустая строка означает «гибкий».
	Mode Mode
	// ActorID — автор импортируемых проводок.
	ActorID int64
	// FileName — имя исходного файла (для отчёта и коммита).
	FileName string
}

// Problem — одна проблема импорта: место в файле и понятная причина.
type Problem struct {
	// Line — номер строки исходного файла (0, если место неизвестно).
	Line int
	// Message — причина.
	Message string
	// Snippet — текст исходной строки для ручной правки.
	Snippet string
}

// Error возвращает причину с номером строки.
func (p Problem) Error() string {
	if p.Line == 0 {
		return p.Message
	}
	return fmt.Sprintf("строка %d: %s", p.Line, p.Message)
}

// Report — отчёт об импорте.
type Report struct {
	// Mode — применённый режим импорта.
	Mode Mode
	// FileName — имя исходного файла.
	FileName string
	// Accepted — число принятых проводок.
	Accepted int
	// Skipped — число пропущенных проводок с причиной.
	Skipped int
	// Duplicates — число проводок, пропущенных как уже импортированные.
	Duplicates int
	// CreatedAccounts — созданные счета (гибкий режим), по возрастанию пути.
	CreatedAccounts []string
	// Problems — проблемы импорта: ошибки разбора, причины пропуска.
	Problems []Problem
	// Rejected — импорт отклонён целиком (книга не изменена).
	Rejected bool
	// Message — короткое пояснение к исходу импорта.
	Message string
}

// Importer — импорт книги hledger в asledger.
//
// Импорт — единственная точка записи истории: разобранные проводки проходят
// валидатор движка и записываются одним коммитом в одной транзакции БД.
type Importer struct {
	db     *store.DB
	engine *engine.Engine
}

// NewImporter создаёт импортёр поверх книги.
func NewImporter(db *store.DB) *Importer {
	return &Importer{db: db, engine: engine.New(db)}
}

// Import разбирает книгу hledger и переносит её в asledger.
//
// Возвращаемая ошибка означает сбой инфраструктуры (БД, движок), а проблемы
// данных возвращаются в отчёте: даже пустой отчёт с проблемами — нормальный
// исход импорта.
func (im *Importer) Import(ctx context.Context, data []byte, opts Options) (*Report, error) {
	mode, err := ParseMode(string(opts.Mode))
	if err != nil {
		return nil, err
	}
	opts.Mode = mode

	book := Parse(data)
	report := &Report{Mode: mode, FileName: opts.FileName, Skipped: book.Dropped}
	for _, parseErr := range book.Errors {
		report.Problems = append(report.Problems, Problem(parseErr))
	}
	if len(book.Transactions) == 0 {
		report.Message = "в книге не найдено ни одной проводки"
		report.Rejected = mode == ModeStrict
		return report, nil
	}

	byPath, err := im.accountsByPath(ctx)
	if err != nil {
		return nil, err
	}

	types, problems := resolveTypes(book, byPath)
	report.Problems = append(report.Problems, problems...)

	var missing []string
	for _, path := range sortedPaths(types) {
		if _, ok := byPath[path]; !ok {
			missing = append(missing, path)
		}
	}

	if mode == ModeStrict {
		for _, path := range missing {
			report.Problems = append(report.Problems, Problem{
				Message: fmt.Sprintf("счёт %q отсутствует в плане счетов; добавьте его или включите гибкий режим", path),
			})
		}
	}

	var created []model.Account
	prepared := true
	if mode == ModeFlexible {
		var problem *Problem
		byPath, created, problem = im.createMissing(ctx, types, byPath, missing)
		if problem != nil {
			prepared = false
			report.Rejected = true
			report.Skipped = len(book.Transactions) + book.Dropped
			report.Problems = append(report.Problems, *problem)
			report.Message = "импорт не применён: книга не изменена"
		}
	}
	if !prepared {
		return report, nil
	}

	for _, account := range created {
		report.CreatedAccounts = append(report.CreatedAccounts, account.FullPath)
	}

	hashes, err := im.db.ImportedHashes(ctx, Source)
	if err != nil {
		return nil, err
	}

	candidates := im.buildAll(ctx, book, byPath, opts, hashes, report)

	if mode == ModeStrict {
		if len(report.Problems) > 0 {
			report.Rejected = true
			report.Skipped = len(book.Transactions) + book.Dropped
			report.Message = "строгий режим: импорт отклонён целиком, книга не изменена"
			return report, nil
		}
	} else if len(report.Problems) > 0 {
		report.Message = "гибкий режим: проблемные проводки пропущены"
	}

	if len(candidates) == 0 {
		if report.Message == "" {
			report.Message = "новых проводок для импорта нет"
		}
		return report, nil
	}

	if err := im.write(ctx, candidates, opts); err != nil {
		return nil, err
	}
	report.Accepted = len(candidates)
	return report, nil
}

// accountsByPath читает действующий план счетов в карту «путь → счёт».
func (im *Importer) accountsByPath(ctx context.Context) (map[string]*model.Account, error) {
	accounts, err := im.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]*model.Account, len(accounts))
	for i := range accounts {
		byPath[accounts[i].FullPath] = &accounts[i]
	}
	return byPath, nil
}

// createMissing создаёт недостающие счета гибкого режима.
//
// Счета создаются до сборки проводок: строки ссылаются на идентификаторы, а
// идентификатор известен только после создания счёта.
func (im *Importer) createMissing(ctx context.Context, types map[string]model.AccountType, byPath map[string]*model.Account, missing []string) (map[string]*model.Account, []model.Account, *Problem) {
	if len(missing) == 0 {
		return byPath, nil, nil
	}
	specs := make([]store.AccountSpec, 0, len(missing))
	for _, path := range missing {
		specs = append(specs, store.AccountSpec{Path: path, Type: types[path]})
	}
	byPath, created, err := im.db.EnsureAccounts(ctx, specs)
	if err != nil {
		return byPath, nil, &Problem{Message: "не удалось дополнить план счетов: " + err.Error()}
	}
	return byPath, created, nil
}

// buildAll собирает и проверяет проводки к записи.
//
// Каждая проводка проходит валидатор движка (ValidateImported). Провалившая
// проверку проводка попадает в проблемы отчёта и не записывается.
func (im *Importer) buildAll(ctx context.Context, book *Book, byPath map[string]*model.Account, opts Options, imported map[string]bool, report *Report) []*model.Transaction {
	occurrences := make(map[string]int, len(book.Transactions))
	candidates := make([]*model.Transaction, 0, len(book.Transactions))

	for _, parsed := range book.Transactions {
		canonical := parsed.Canonical()
		occurrence := occurrences[canonical]
		occurrences[canonical] = occurrence + 1
		hash := sourceHash(Source, canonical, occurrence)

		if imported[hash] {
			report.Duplicates++
			continue
		}

		tr, problems := build(parsed, byPath, opts.ActorID, hash)
		if len(problems) > 0 {
			report.Problems = append(report.Problems, problems...)
			report.Skipped++
			continue
		}
		if err := im.engine.ValidateImported(ctx, tr); err != nil {
			report.Problems = append(report.Problems, Problem{Line: parsed.Line, Message: err.Error()})
			report.Skipped++
			continue
		}
		candidates = append(candidates, tr)
	}
	return candidates
}

// write записывает принятые проводки одним коммитом в одной транзакции БД.
func (im *Importer) write(ctx context.Context, transactions []*model.Transaction, opts Options) error {
	head, err := im.db.HeadSeq(ctx)
	if err != nil {
		return err
	}
	prevHash, err := im.chainHash(ctx)
	if err != nil {
		return err
	}

	hashes := make([]string, 0, len(transactions))
	for _, tr := range transactions {
		hashes = append(hashes, tr.SourceHash)
	}
	payload, err := json.Marshal(importPayload{
		Source: Source, File: opts.FileName, Mode: opts.Mode, Hashes: hashes,
	})
	if err != nil {
		return fmt.Errorf("hledger: не удалось сериализовать пакет импорта: %w", err)
	}

	commit := &model.Commit{
		ClientID: "import:" + Source,
		ActorID:  opts.ActorID,
		BaseSeq:  head,
		OpsJSON:  string(payload),
		Hash:     sync.ChainHash(prevHash, string(payload)),
	}
	if _, err := im.db.ApplyCommit(ctx, commit, transactions); err != nil {
		return err
	}
	return nil
}

// chainHash возвращает текущее звено хэш-цепочки (пустая строка для пустой истории).
func (im *Importer) chainHash(ctx context.Context) (string, error) {
	commit, err := im.db.LastCommit(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return commit.Hash, nil
}

// importPayload — содержимое коммита импорта (звено хэш-цепочки).
type importPayload struct {
	Source string   `json:"source"`
	File   string   `json:"file,omitempty"`
	Mode   Mode     `json:"mode"`
	Hashes []string `json:"hashes"`
}

// build собирает проводку asledger из разобранной проводки hledger.
//
// Знак суммы определяет сторону строки: положительная — дебет, отрицательная —
// кредит. Строки одного счёта и одной стороны складываются (в книге
// asledger на счёт и сторону приходится одна строка проводки), нулевые строки
// отбрасываются.
func build(parsed Transaction, byPath map[string]*model.Account, actorID int64, hash string) (*model.Transaction, []Problem) {
	type key struct {
		path string
		side model.Side
	}

	sums := make(map[key]money.Kop)
	var order []key
	for _, posting := range parsed.Postings {
		path := strings.TrimSpace(posting.Account)
		side := model.SideDebit
		amount := posting.Amount
		if amount < 0 {
			side = model.SideCredit
			amount = -amount
		}
		k := key{path: path, side: side}
		if _, ok := sums[k]; !ok {
			order = append(order, k)
		}
		sums[k] += amount
	}

	lines := make([]model.TransactionLine, 0, len(order))
	lineTypes := make([]model.AccountType, 0, len(order))
	for _, k := range order {
		if sums[k] == 0 {
			continue
		}
		account, ok := byPath[k.path]
		if !ok {
			return nil, []Problem{{
				Line:    parsed.Line,
				Message: fmt.Sprintf("счёт %q отсутствует в плане счетов", k.path),
			}}
		}
		lines = append(lines, model.TransactionLine{
			AccountID: account.ID, Amount: sums[k], Side: k.side,
		})
		lineTypes = append(lineTypes, account.Type)
	}
	if len(lines) < 2 {
		return nil, []Problem{{
			Line:    parsed.Line,
			Message: fmt.Sprintf("после объединения строк осталось %d — нужно минимум две", len(lines)),
		}}
	}

	return &model.Transaction{
		UUID:        uuidFromHash(hash),
		OpType:      inferOpType(lines, lineTypes),
		Date:        parsed.Date,
		Description: parsed.Description,
		ActorID:     actorID,
		Source:      Source,
		SourceHash:  hash,
		Lines:       lines,
	}, nil
}

// inferOpType определяет тип операции импортируемой проводки по типам счетов
// сторон: это метаданные проводки, стороны строк заданы исходной книгой.
func inferOpType(lines []model.TransactionLine, lineTypes []model.AccountType) model.OpType {
	debit := map[model.AccountType]bool{}
	credit := map[model.AccountType]bool{}
	all := map[model.AccountType]bool{}
	for i, line := range lines {
		typ := lineTypes[i]
		all[typ] = true
		if line.Side == model.SideDebit {
			debit[typ] = true
		} else {
			credit[typ] = true
		}
	}

	switch {
	case debit[model.TypeExpense]:
		return model.OpExpense
	case credit[model.TypeIncome]:
		return model.OpIncome
	case len(all) == 1 && all[model.TypeAsset]:
		return model.OpTransfer
	case credit[model.TypeLiability]:
		return model.OpLoan
	case debit[model.TypeLiability]:
		return model.OpDebtPayment
	case all[model.TypeCapital]:
		return model.OpInitial
	default:
		return model.OpAdjustment
	}
}

// resolveTypes определяет типы всех нужных книге счетов.
//
// Тип существующего счёта берётся из плана счетов и распространяется на всё
// поддерево; для новых счетов тип устанавливает комментарий директивы account
// («; тип: расход»), а при его отсутствии — имя корневого счёта.
func resolveTypes(book *Book, existing map[string]*model.Account) (map[string]model.AccountType, []Problem) {
	rootType := make(map[string]model.AccountType)
	for path, account := range existing {
		if parent, _ := parentPathOf(path); parent == "" {
			rootType[path] = account.Type
		}
	}

	annotations := make(map[string]model.AccountType, len(book.Declared))
	for _, declared := range book.Declared {
		if declared.Type.Valid() {
			annotations[declared.Path] = declared.Type
		}
	}

	types := make(map[string]model.AccountType)
	var problems []Problem

	paths := book.Accounts()
	sort.Slice(paths, func(i, j int) bool {
		di, dj := strings.Count(paths[i], ":"), strings.Count(paths[j], ":")
		if di != dj {
			return di < dj
		}
		return paths[i] < paths[j]
	})

	for _, path := range paths {
		root := path
		if idx := strings.Index(path, ":"); idx >= 0 {
			root = path[:idx]
		}

		typ, ok := rootType[root]
		if !ok {
			typ = annotations[path]
			if !typ.Valid() {
				typ = inferTypeByName(root)
			}
			if !typ.Valid() {
				problems = append(problems, Problem{
					Message: fmt.Sprintf(
						"не удалось определить тип счёта %q: добавьте директиву «account %s ; тип: …»",
						root, root),
				})
				continue
			}
			rootType[root] = typ
		}
		if existingAccount, ok := existing[path]; ok {
			typ = existingAccount.Type
		}
		types[path] = typ
		// Импорт создаёт счёт вместе со всей цепочкой предков.
		for _, ancestor := range ancestorPaths(path) {
			if _, ok := types[ancestor]; !ok {
				types[ancestor] = typ
			}
		}
	}
	return types, problems
}

// ancestorPaths возвращает пути всех предков счёта, от корня к листу.
func ancestorPaths(path string) []string {
	var paths []string
	for i := 0; i < len(path); i++ {
		if path[i] == ':' && i > 0 {
			paths = append(paths, path[:i])
		}
	}
	return paths
}

// inferTypeByName определяет тип счёта по имени корневого счёта.
//
// Поддерживаются русские и английские имена — книги hledger часто ведутся на
// английском. Для незнакомых корней тип нужно указать директивой account.
func inferTypeByName(root string) model.AccountType {
	switch strings.ToLower(strings.TrimSpace(root)) {
	case "активы", "актив", "имущество", "assets", "asset":
		return model.TypeAsset
	case "обязательства", "обязательство", "пассивы", "пассив", "долги", "долг",
		"liabilities", "liability", "debts", "debt":
		return model.TypeLiability
	case "капитал", "собственный капитал", "equity", "capital":
		return model.TypeCapital
	case "доходы", "доход", "income", "revenue", "revenues":
		return model.TypeIncome
	case "расходы", "расход", "expenses", "expense":
		return model.TypeExpense
	default:
		return ""
	}
}

// sourceHash вычисляет хеш содержимого проводки.
//
// В хеш входят источник, каноничный текст проводки и номер повторения
// одинаковых проводок в файле: две одинаковые проводки в книге остаются двумя,
// а повторный импорт того же файла не создаёт ни одной новой.
func sourceHash(source, canonical string, occurrence int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", source, canonical, occurrence)))
	return hex.EncodeToString(sum[:])
}

// uuidFromHash строит устойчивый uuid проводки из хеша её содержимого: повтор
// того же импорта даёт тот же uuid, поэтому дубль отсекается и на уровне
// уникальности uuid.
func uuidFromHash(hash string) string {
	sum := sha256.Sum256([]byte("asledger-import\x00" + hash))
	hexed := hex.EncodeToString(sum[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexed[0:8], hexed[8:12], hexed[12:16], hexed[16:20], hexed[20:32])
}

// sortedPaths возвращает пути карты типов по возрастанию.
func sortedPaths(types map[string]model.AccountType) []string {
	paths := make([]string, 0, len(types))
	for path := range types {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// parentPathOf делит путь счёта на путь родителя и короткое имя.
func parentPathOf(path string) (string, string) {
	if idx := strings.LastIndex(path, ":"); idx >= 0 {
		return path[:idx], path[idx+1:]
	}
	return "", path
}
