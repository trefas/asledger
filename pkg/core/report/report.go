// Package report считает отчёты по книге asledger: остатки, обороты, расходы
// по членам семьи и журнал проводок.
//
// Отчёты не хранят состояние: остатки всегда выводятся свёрткой журнала, а
// агрегация по поддеревьям — по префиксу полного пути счёта. Пакет работает
// поверх store и не знает про HTTP и шаблоны.
package report

import (
	"context"
	"sort"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// Options — общие параметры отчётов.
type Options struct {
	// IncludeArchived включает заархивированные счета.
	IncludeArchived bool
	// IncludeEmpty включает счета и ветки без остатка и движения.
	IncludeEmpty bool
	// Node ограничивает отчёт поддеревом по полному пути; пусто — вся книга.
	Node string
	// From и To задают период (unix-секунды); нуль — без границы.
	From int64
	To   int64
}

// BalanceRow — строка отчёта об остатках.
type BalanceRow struct {
	// AccountID — счёт.
	AccountID int64
	// Path и Name — полный путь и короткое имя счёта.
	Path string
	Name string
	// Type — тип счёта.
	Type model.AccountType
	// Depth — уровень вложенности (0 для корневых счетов).
	Depth int
	// Own — остаток самого счёта без поддерева.
	Own money.Kop
	// Total — остаток счёта вместе с поддеревом.
	Total money.Kop
	// Leaf — счёт листовой (допустим в проводках).
	Leaf bool
	// Archived — счёт в архиве.
	Archived bool
}

// Negative сообщает, отрицателен ли остаток счёта с поддеревом.
func (r BalanceRow) Negative() bool { return r.Total < 0 }

// BalancesReport — отчёт об остатках по счетам.
type BalancesReport struct {
	// Rows — счета в порядке дерева (обход в глубину).
	Rows []BalanceRow
	// Assets и Liabilities — итоги по корневым ветвям.
	Assets      money.Kop
	Liabilities money.Kop
}

// NetWorth возвращает оценку капитала: активы минус обязательства.
func (r BalancesReport) NetWorth() money.Kop { return r.Assets - r.Liabilities }

// Negatives возвращает строки с отрицательным остатком (для подсветки).
func (r BalancesReport) Negatives() []BalanceRow {
	var out []BalanceRow
	for _, row := range r.Rows {
		if row.Negative() {
			out = append(out, row)
		}
	}
	return out
}

// Balances строит отчёт об остатках с иерархическими итогами по поддеревьям.
//
// Для каждого счёта показывается собственный остаток и итог вместе с потомками;
// итог считается по префиксу полного пути, поэтому совпадает со свёрткой
// журнала по поддереву.
func Balances(ctx context.Context, db *store.DB, opts Options) (*BalancesReport, error) {
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{
		IncludeArchived: opts.IncludeArchived,
	})
	if err != nil {
		return nil, err
	}
	balances, err := db.Balances(ctx)
	if err != nil {
		return nil, err
	}

	children, roots := accountTree(accounts)
	for id := range children {
		sort.Slice(children[id], func(i, j int) bool { return children[id][i].Name < children[id][j].Name })
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })

	report := &BalancesReport{}
	// total считает остаток счёта вместе со всем поддеревом (по префиксу пути).
	var total func(account model.Account) money.Kop
	total = func(account model.Account) money.Kop {
		sum := balances[account.ID]
		for _, child := range children[account.ID] {
			sum += total(child)
		}
		return sum
	}

	var walk func(account model.Account, depth int)
	walk = func(account model.Account, depth int) {
		subtree := total(account)
		row := BalanceRow{
			AccountID: account.ID,
			Path:      account.FullPath,
			Name:      account.Name,
			Type:      account.Type,
			Depth:     depth,
			Own:       balances[account.ID],
			Total:     subtree,
			Leaf:      account.IsLeaf,
			Archived:  account.IsArchived,
		}
		if opts.IncludeEmpty || subtree != 0 || account.IsLeaf {
			report.Rows = append(report.Rows, row)
		}
		for _, child := range children[account.ID] {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}

	for _, row := range report.Rows {
		if row.Depth != 0 {
			continue
		}
		switch row.Type {
		case model.TypeAsset:
			report.Assets += row.Total
		case model.TypeLiability:
			report.Liabilities += row.Total
		}
	}
	return report, nil
}

// TurnoverRow — строка отчёта об оборотах за период.
type TurnoverRow struct {
	// AccountID — счёт.
	AccountID int64
	// Path и Name — полный путь и короткое имя счёта.
	Path string
	Name string
	// Type — тип счёта.
	Type model.AccountType
	// Depth — уровень вложенности.
	Depth int
	// Opening — остаток на начало периода.
	Opening money.Kop
	// Debit и Credit — обороты за период.
	Debit  money.Kop
	Credit money.Kop
	// Closing — остаток на конец периода.
	Closing money.Kop
}

// Movement сообщает, было ли движение по счёту за период.
func (r TurnoverRow) Movement() bool { return r.Debit != 0 || r.Credit != 0 }

// TurnoversReport — отчёт об оборотах по статьям за период.
type TurnoversReport struct {
	// Node — узел дерева, по которому построен отчёт (пусто — вся книга).
	Node string
	// From и To — период отчёта.
	From int64
	To   int64
	// Rows — счета в порядке дерева.
	Rows []TurnoverRow
}

// Turnovers строит отчёт об оборотах по любому узлу дерева за период.
//
// Узел задаётся полным путём в Options.Node: в отчёт попадают счёт узла и всё
// его поддерево (по префиксу пути). Обороты берутся из журнала, остатки на
// начало и конец периода выводятся по природе типа счёта.
func Turnovers(ctx context.Context, db *store.DB, opts Options) (*TurnoversReport, error) {
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{
		Prefix:          opts.Node,
		IncludeArchived: opts.IncludeArchived,
	})
	if err != nil {
		return nil, err
	}
	turnovers, err := db.Turnovers(ctx, opts.From, opts.To)
	if err != nil {
		return nil, err
	}

	children, roots := accountTree(accounts)
	for id := range children {
		sort.Slice(children[id], func(i, j int) bool { return children[id][i].Name < children[id][j].Name })
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })

	report := &TurnoversReport{Node: opts.Node, From: opts.From, To: opts.To}
	var walk func(account model.Account, depth int)
	walk = func(account model.Account, depth int) {
		turnover := turnovers[account.ID]
		row := TurnoverRow{
			AccountID: account.ID,
			Path:      account.FullPath,
			Name:      account.Name,
			Type:      account.Type,
			Depth:     depth,
			Opening:   store.BalanceFor(account.Type, turnover.OpeningDebit, turnover.OpeningCredit),
			Debit:     turnover.PeriodDebit,
			Credit:    turnover.PeriodCredit,
			Closing: store.BalanceFor(account.Type,
				turnover.OpeningDebit+turnover.PeriodDebit,
				turnover.OpeningCredit+turnover.PeriodCredit),
		}
		if opts.IncludeEmpty || row.Movement() || row.Opening != 0 || row.Closing != 0 {
			report.Rows = append(report.Rows, row)
		}
		for _, child := range children[account.ID] {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return report, nil
}

// ArticleExpense — расход по статье (счёту расходов).
type ArticleExpense struct {
	// AccountID — счёт статьи.
	AccountID int64
	// Path — полный путь статьи.
	Path string
	// Amount — сумма расхода.
	Amount money.Kop
}

// ActorExpense — расходы одного члена семьи.
type ActorExpense struct {
	// ActorID — автор проводок.
	ActorID int64
	// Name — имя автора.
	Name string
	// Total — сумма расходов.
	Total money.Kop
	// Articles — разбивка по статьям расходов (по убыванию суммы).
	Articles []ArticleExpense
}

// ActorReport — отчёт о расходах по членам семьи за период.
type ActorReport struct {
	// From и To — период отчёта.
	From int64
	To   int64
	// Actors — расходы по авторам (по убыванию суммы).
	Actors []ActorExpense
	// Total — общий расход за период.
	Total money.Kop
}

// ExpensesByActor строит отчёт о расходах по членам семьи.
//
// Расход считается по строкам счетов типа «расход» (ΣDEBIT − ΣCREDIT): сторно
// и корректировки уменьшают показатели, поэтому суммы по членам семьи в сумме
// дают общий расход за период.
func ExpensesByActor(ctx context.Context, db *store.DB, opts Options) (*ActorReport, error) {
	rows, err := db.ExpensesByActor(ctx, opts.From, opts.To)
	if err != nil {
		return nil, err
	}
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	users, err := db.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	byPath := make(map[int64]string, len(accounts))
	for i := range accounts {
		byPath[accounts[i].ID] = accounts[i].FullPath
	}
	nameOf := make(map[int64]string, len(users))
	for _, user := range users {
		nameOf[user.ID] = user.Name
	}

	type accumulator struct {
		total    money.Kop
		articles map[int64]money.Kop
	}
	acc := map[int64]*accumulator{}
	for _, row := range rows {
		if row.Amount == 0 {
			continue
		}
		a := acc[row.ActorID]
		if a == nil {
			a = &accumulator{articles: map[int64]money.Kop{}}
			acc[row.ActorID] = a
		}
		a.total += row.Amount
		a.articles[row.AccountID] += row.Amount
	}

	report := &ActorReport{From: opts.From, To: opts.To}
	for actorID, a := range acc {
		actor := ActorExpense{ActorID: actorID, Name: nameOf[actorID], Total: a.total}
		for accountID, amount := range a.articles {
			actor.Articles = append(actor.Articles, ArticleExpense{
				AccountID: accountID, Path: byPath[accountID], Amount: amount,
			})
		}
		sort.Slice(actor.Articles, func(i, j int) bool {
			if actor.Articles[i].Amount != actor.Articles[j].Amount {
				return actor.Articles[i].Amount > actor.Articles[j].Amount
			}
			return actor.Articles[i].Path < actor.Articles[j].Path
		})
		report.Actors = append(report.Actors, actor)
		report.Total += actor.Total
	}
	sort.Slice(report.Actors, func(i, j int) bool {
		if report.Actors[i].Total != report.Actors[j].Total {
			return report.Actors[i].Total > report.Actors[j].Total
		}
		return report.Actors[i].ActorID < report.Actors[j].ActorID
	})
	return report, nil
}

// JournalLine — строка проводки в журнале.
type JournalLine struct {
	// AccountID — счёт строки.
	AccountID int64
	// Path — полный путь счёта.
	Path string
	// Side — сторона строки.
	Side model.Side
	// Amount — сумма строки.
	Amount money.Kop
}

// JournalRow — проводка в журнале.
type JournalRow struct {
	// ID и UUID — идентификаторы проводки.
	ID   int64
	UUID string
	// Date — дата проводки.
	Date time.Time
	// OpType — тип операции.
	OpType model.OpType
	// Description — описание.
	Description string
	// ActorID и ActorName — автор проводки.
	ActorID   int64
	ActorName string
	// ReversesID — отменяемая проводка, если это сторно.
	ReversesID *int64
	// Amount — сумма проводки (ΣDEBIT).
	Amount money.Kop
	// Lines — строки проводки.
	Lines []JournalLine
}

// Journal возвращает проводки журнала с разрешёнными именами счетов и авторов.
//
// Фильтры и постраничная выдача задаются store.TransactionFilter: период,
// счёт и его поддерево, тип операции, автор, сумма.
func Journal(ctx context.Context, db *store.DB, filter store.TransactionFilter) ([]JournalRow, error) {
	transactions, err := db.ListTransactions(ctx, filter)
	if err != nil {
		return nil, err
	}
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	users, err := db.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	byPath := make(map[int64]string, len(accounts))
	for i := range accounts {
		byPath[accounts[i].ID] = accounts[i].FullPath
	}
	nameOf := make(map[int64]string, len(users))
	for _, user := range users {
		nameOf[user.ID] = user.Name
	}

	rows := make([]JournalRow, 0, len(transactions))
	for i := range transactions {
		tr := &transactions[i]
		row := JournalRow{
			ID:          tr.ID,
			UUID:        tr.UUID,
			Date:        tr.Date,
			OpType:      tr.OpType,
			Description: tr.Description,
			ActorID:     tr.ActorID,
			ActorName:   nameOf[tr.ActorID],
			ReversesID:  tr.ReversesID,
			Lines:       make([]JournalLine, 0, len(tr.Lines)),
		}
		for _, line := range tr.Lines {
			if line.Side == model.SideDebit {
				row.Amount += line.Amount
			}
			row.Lines = append(row.Lines, JournalLine{
				AccountID: line.AccountID,
				Path:      byPath[line.AccountID],
				Side:      line.Side,
				Amount:    line.Amount,
			})
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// accountTree разбивает список счетов на карту «родитель → дети» и корни.
func accountTree(accounts []model.Account) (map[int64][]model.Account, []model.Account) {
	children := make(map[int64][]model.Account)
	byID := make(map[int64]model.Account, len(accounts))
	for _, account := range accounts {
		byID[account.ID] = account
	}
	var roots []model.Account
	for _, account := range accounts {
		if account.ParentID == nil {
			roots = append(roots, account)
			continue
		}
		if _, ok := byID[*account.ParentID]; !ok {
			// Родитель отфильтрован (например, архивный) — ветка становится корнем.
			roots = append(roots, account)
			continue
		}
		children[*account.ParentID] = append(children[*account.ParentID], account)
	}
	return children, roots
}

// FormatPeriod возвращает подпись периода для заголовков отчётов.
func FormatPeriod(from, to int64) string {
	switch {
	case from == 0 && to == 0:
		return "весь период"
	case from == 0:
		return "по " + formatDate(to)
	case to == 0:
		return "с " + formatDate(from)
	default:
		return formatDate(from) + " — " + formatDate(to)
	}
}

// formatDate переводит unix-секунды в дату ГГГГ-ММ-ДД.
func formatDate(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.DateOnly)
}
