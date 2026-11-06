// Package accrual реализует ручное и периодическое начисление процентов
// по кредитам и вкладам.
//
// Сервис читает настройки продукта, рассчитывает проценты и бонусы и
// фиксирует их в субледжере interest_accruals. Проценты по кредиту и
// капитализированные проценты по вкладу дополнительно проводятся отдельной
// проводкой типа «начисление_процентов»; проценты по вкладу без капитализации
// остаются в субледжере до выплаты при снятии.
package accrual

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/interest"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/uuid"
)

// Service — сервис начисления процентов поверх книги и движка проводок.
type Service struct {
	db      *store.DB
	engine  *engine.Engine
	deposit interest.DepositCalculator
	credit  interest.CreditCalculator
}

// New создаёт сервис начислений.
func New(db *store.DB) *Service {
	return NewWithEngine(db, engine.New(db))
}

// NewWithEngine создаёт сервис начислений с готовым движком проводок.
func NewWithEngine(db *store.DB, eng *engine.Engine) *Service {
	return &Service{db: db, engine: eng}
}

// Input — запрос на расчёт или проведение начисления.
//
// Периодическое начисление задаётся парой PeriodFrom/PeriodTo; ручное —
// суммой Amount и датой Date. Если Kind пуст, вид определяется типом счёта:
// для пассива — проценты по кредиту, для актива — проценты по вкладу.
type Input struct {
	// UUID — идентификатор ручного начисления; пустой означает «сгенерировать».
	UUID string
	// AccountID — счёт кредита или вклада.
	AccountID int64
	// Kind — вид начисления; пустой означает «по типу счёта».
	Kind model.InterestAccrualKind
	// Date — дата начисления; для периода по умолчанию равна PeriodTo.
	Date time.Time
	// PeriodFrom и PeriodTo — период периодического начисления.
	PeriodFrom time.Time
	PeriodTo   time.Time
	// Amount — сумма ручного начисления.
	Amount money.Kop
	// IncomeAccountID — статья доходов для вклада (капитализация).
	IncomeAccountID int64
	// ExpenseAccountID — статья расходов для кредита.
	ExpenseAccountID int64
	// Description — описание проводки; пустое заменяется стандартным.
	Description string
	// ActorID — автор проводки.
	ActorID int64
	// Source — источник проводки ('web', 'api:<client_id>').
	Source string
}

// Item — одно рассчитанное начисление.
type Item struct {
	// UUID — идентификатор начисления.
	UUID string
	// Date — дата начисления.
	Date time.Time
	// Kind — вид начисления.
	Kind model.InterestAccrualKind
	// Amount — сумма начисления.
	Amount money.Kop
	// Base — база расчёта.
	Base money.Kop
	// RatePPM — ставка расчёта в ppm.
	RatePPM int64
	// PeriodFrom и PeriodTo — период начисления; нули означают ручное.
	PeriodFrom time.Time
	PeriodTo   time.Time
	// Status — статус, который получит начисление.
	Status model.InterestAccrualStatus
	// Existing — начисление уже есть в книге; повтор ничего не запишет.
	Existing bool
}

// Preview — результат расчёта без записи.
type Preview struct {
	// AccountID — счёт начисления.
	AccountID int64
	// Kind — запрошенный вид начисления; пустой означает «все виды».
	Kind model.InterestAccrualKind
	// PeriodFrom и PeriodTo — период периодического начисления.
	PeriodFrom time.Time
	PeriodTo   time.Time
	// Capitalization — проценты добавляются к вкладу.
	Capitalization bool
	// Total — сумма всех начислений, включая уже существующие.
	Total money.Kop
	// Accruals — рассчитанные начисления.
	Accruals []Item
}

// PostedItem — сохранённое начисление.
type PostedItem struct {
	// Accrual — запись субледжера.
	Accrual *model.InterestAccrual
	// Transaction — проводка начисления; nil для процентов вклада без
	// капитализации.
	Transaction *model.Transaction
	// Existing — начисление уже было в книге.
	Existing bool
}

// Posted — результат проведения начислений.
type Posted struct {
	// AccountID — счёт начисления.
	AccountID int64
	// Kind — запрошенный вид начисления; пустой означает «все виды».
	Kind model.InterestAccrualKind
	// PeriodFrom и PeriodTo — период периодического начисления.
	PeriodFrom time.Time
	PeriodTo   time.Time
	// Capitalization — проценты добавлены к вкладу.
	Capitalization bool
	// Total — сумма всех начислений, включая уже существующие.
	Total money.Kop
	// Applied — сумма, записанная этим вызовом.
	Applied money.Kop
	// Accruals — сохранённые начисления.
	Accruals []PostedItem
}

// item — внутреннее представление рассчитанного начисления.
type item struct {
	uuid       string
	date       time.Time
	kind       model.InterestAccrualKind
	amount     money.Kop
	base       money.Kop
	ratePPM    int64
	periodFrom time.Time
	periodTo   time.Time
	existing   *model.InterestAccrual
}

// prepared — проверенный запрос с рассчитанными начислениями.
type prepared struct {
	account        *model.Account
	kind           model.InterestAccrualKind
	periodFrom     time.Time
	periodTo       time.Time
	capitalization bool
	items          []item
}

// Calculate рассчитывает начисления, не записывая их в книгу.
func (s *Service) Calculate(ctx context.Context, in Input) (*Preview, error) {
	p, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	preview := &Preview{
		AccountID:      p.account.ID,
		Kind:           p.kind,
		PeriodFrom:     p.periodFrom,
		PeriodTo:       p.periodTo,
		Capitalization: p.capitalization,
	}
	for _, it := range p.items {
		preview.Total += it.amount
		preview.Accruals = append(preview.Accruals, itemView(it, p.capitalization))
	}
	return preview, nil
}

// Apply рассчитывает и записывает начисления.
//
// Повтор с тем же uuid или за тот же период не создаёт дубль: возвращается
// уже сохранённое начисление и его проводка.
func (s *Service) Apply(ctx context.Context, in Input) (*Posted, error) {
	p, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	posted := &Posted{
		AccountID:      p.account.ID,
		Kind:           p.kind,
		PeriodFrom:     p.periodFrom,
		PeriodTo:       p.periodTo,
		Capitalization: p.capitalization,
	}
	for _, it := range p.items {
		existing := it.existing
		if existing == nil {
			existing, err = s.lookupExisting(ctx, p.account.ID, it)
			if err != nil {
				return nil, err
			}
		}
		if existing != nil {
			posted.Total += existing.Amount
			posted.Accruals = append(posted.Accruals, PostedItem{
				Accrual: existing, Transaction: nil, Existing: true,
			})
			continue
		}

		accrual, transaction, err := s.post(ctx, in, p, it)
		if err != nil {
			return nil, err
		}
		posted.Total += accrual.Amount
		posted.Applied += accrual.Amount
		posted.Accruals = append(posted.Accruals, PostedItem{
			Accrual: accrual, Transaction: transaction,
		})
	}
	return posted, nil
}

// prepare проверяет запрос и рассчитывает начисления.
func (s *Service) prepare(ctx context.Context, in Input) (*prepared, error) {
	account, err := s.db.Account(ctx, in.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, accrualError(store.CodeInterestAccountInvalid, "счёт начисления не найден")
	}
	if err != nil {
		return nil, err
	}
	if account.IsArchived || !account.IsLeaf {
		return nil, accrualError(store.CodeInterestAccountInvalid,
			"начисление возможно только по листовому счёту: "+account.FullPath)
	}

	if _, err := accrualKind(in.Kind, account.Type); err != nil {
		return nil, err
	}

	p := &prepared{account: account, kind: in.Kind}
	switch account.Type {
	case model.TypeLiability:
		settings, err := s.db.CreditSettings(ctx, account.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, accrualError(store.CodeInterestSettingsMissing,
				"у счёта нет настроек кредита: "+account.FullPath)
		}
		if err != nil {
			return nil, err
		}
		if err := s.checkExpenseAccount(ctx, in.ExpenseAccountID); err != nil {
			return nil, err
		}
		p.kind = model.InterestAccrualCredit
		p.items, err = s.creditItems(ctx, account, *settings, in)
		if err != nil {
			return nil, err
		}
	case model.TypeAsset:
		settings, err := s.db.DepositSettings(ctx, account.ID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, accrualError(store.CodeInterestSettingsMissing,
				"у счёта нет настроек вклада: "+account.FullPath)
		}
		if err != nil {
			return nil, err
		}
		if err := s.checkIncomeAccount(ctx, in.IncomeAccountID); err != nil {
			return nil, err
		}
		p.capitalization = settings.Capitalization
		p.items, err = s.depositItems(ctx, account, *settings, in.Kind, in)
		if err != nil {
			return nil, err
		}
	default:
		return nil, accrualError(store.CodeInterestAccountInvalid,
			"начисление возможно только по счёту кредита или вклада: "+account.FullPath)
	}

	if p.kind == model.InterestAccrualCredit && in.ExpenseAccountID == 0 && len(p.items) > 0 {
		return nil, accrualError(store.CodeInterestExpenseAccount,
			"для начисления процентов по кредиту выберите статью расходов")
	}
	if p.capitalization && in.IncomeAccountID == 0 && len(p.items) > 0 {
		return nil, accrualError(store.CodeInterestIncomeAccount,
			"для капитализации процентов по вкладу выберите статью доходов")
	}
	if len(p.items) == 0 {
		return nil, accrualError(store.CodeInterestNothing,
			"за указанный период нечего начислять")
	}
	return p, nil
}

// creditItems рассчитывает начисления по кредиту.
func (s *Service) creditItems(ctx context.Context, account *model.Account,
	settings model.CreditSettings, in Input) ([]item, error) {
	if periodic(in) {
		from, to, err := accrualPeriod(in)
		if err != nil {
			return nil, err
		}
		balance, err := s.db.BalanceAt(ctx, account.ID, from)
		if err != nil {
			return nil, err
		}
		amount := s.credit.AccrueInterest(interest.CreditAccount{
			Balance:  balance,
			Settings: settings,
		}, to, daysBetween(from, to))
		if amount <= 0 {
			return nil, nil
		}
		id, err := newUUID()
		if err != nil {
			return nil, err
		}
		return []item{{
			uuid:       id,
			date:       to,
			kind:       model.InterestAccrualCredit,
			amount:     amount,
			base:       balance,
			ratePPM:    settings.RatePPM,
			periodFrom: from,
			periodTo:   to,
		}}, nil
	}

	date, err := manualDate(in.Date)
	if err != nil {
		return nil, err
	}
	if in.Amount <= 0 {
		return nil, accrualError(store.CodeInterestAmount,
			"сумма начисления должна быть положительной")
	}
	id, err := manualUUID(in.UUID)
	if err != nil {
		return nil, err
	}
	it := item{
		uuid:   id,
		date:   date,
		kind:   model.InterestAccrualCredit,
		amount: in.Amount,
	}
	if existing, err := s.db.InterestAccrualByUUID(ctx, it.uuid); err == nil {
		it.existing = existing
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return []item{it}, nil
}

// depositItems рассчитывает начисления по вкладу.
func (s *Service) depositItems(ctx context.Context, account *model.Account,
	settings model.DepositSettings, kind model.InterestAccrualKind, in Input) ([]item, error) {
	if periodic(in) {
		from, to, err := accrualPeriod(in)
		if err != nil {
			return nil, err
		}
		balance, err := s.db.BalanceAt(ctx, account.ID, from)
		if err != nil {
			return nil, err
		}
		movements, err := s.movements(ctx, account, from, to)
		if err != nil {
			return nil, err
		}
		calculated, err := s.deposit.Accrue(interest.DepositAccount{
			Balance:   balance,
			Settings:  settings,
			Movements: movements,
		}, from, to)
		if err != nil {
			return nil, accrualError(store.CodeInterestPeriod,
				"не удалось рассчитать проценты по вкладу: "+err.Error())
		}

		items := make([]item, 0, len(calculated))
		for _, calculatedAccrual := range calculated {
			if kind != "" && calculatedAccrual.Kind != kind {
				continue
			}
			id, err := newUUID()
			if err != nil {
				return nil, err
			}
			it := item{
				uuid:       id,
				date:       calculatedAccrual.Date,
				kind:       calculatedAccrual.Kind,
				amount:     calculatedAccrual.Amount,
				base:       calculatedAccrual.Base,
				ratePPM:    int64(calculatedAccrual.Rate),
				periodFrom: calculatedAccrual.PeriodFrom,
				periodTo:   calculatedAccrual.PeriodTo,
			}
			existing, err := s.db.InterestAccrualByPeriod(ctx, account.ID,
				it.kind, it.periodFrom, it.periodTo)
			if err == nil {
				it.existing = existing
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
			items = append(items, it)
		}
		return items, nil
	}

	date, err := manualDate(in.Date)
	if err != nil {
		return nil, err
	}
	if in.Amount <= 0 {
		return nil, accrualError(store.CodeInterestAmount,
			"сумма начисления должна быть положительной")
	}
	if kind == "" {
		kind = model.InterestAccrualDeposit
	}
	id, err := manualUUID(in.UUID)
	if err != nil {
		return nil, err
	}
	it := item{
		uuid:   id,
		date:   date,
		kind:   kind,
		amount: in.Amount,
	}
	if existing, err := s.db.InterestAccrualByUUID(ctx, it.uuid); err == nil {
		it.existing = existing
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return []item{it}, nil
}

// post записывает начисление и, если нужно, его проводку.
func (s *Service) post(ctx context.Context, in Input, p *prepared, it item) (*model.InterestAccrual, *model.Transaction, error) {
	status := model.InterestAccrued
	needsTransaction := it.kind == model.InterestAccrualCredit || p.capitalization
	if needsTransaction {
		if it.kind == model.InterestAccrualCredit {
			status = model.InterestAccrued
		} else {
			status = model.InterestCapitalized
		}
	}

	accrualInput := store.InterestAccrualInput{
		UUID:       it.uuid,
		AccountID:  p.account.ID,
		Date:       it.date,
		Kind:       it.kind,
		Amount:     it.amount,
		BaseAmount: it.base,
		RatePPM:    it.ratePPM,
		PeriodFrom: it.periodFrom,
		PeriodTo:   it.periodTo,
		Status:     status,
	}
	if !needsTransaction {
		accrual, _, err := s.db.EnsureInterestAccrual(ctx, accrualInput)
		return accrual, nil, err
	}

	entries := s.entries(p.account, it, in)
	description := strings.TrimSpace(in.Description)
	if description == "" {
		if it.kind == model.InterestAccrualCredit {
			description = "Начисление процентов по кредиту"
		} else {
			description = "Капитализация процентов по вкладу"
		}
	}
	prepared, err := s.engine.Prepare(ctx, engine.Operation{
		OpType:      model.OpInterestAccrual,
		UUID:        it.uuid,
		Date:        it.date,
		Description: description,
		Source:      in.Source,
		Entries:     entries,
	}, in.ActorID)
	if err != nil {
		return nil, nil, err
	}
	accrual, err := s.db.PostInterestAccrual(ctx, accrualInput, prepared.Transaction)
	if err != nil {
		return nil, nil, err
	}
	if accrual.TransactionID == nil {
		return accrual, nil, nil
	}
	transaction, err := s.db.Transaction(ctx, *accrual.TransactionID)
	if err != nil {
		return nil, nil, err
	}
	return accrual, transaction, nil
}

// entries собирает строки проводки начисления.
func (s *Service) entries(account *model.Account, it item, in Input) []engine.Entry {
	if it.kind == model.InterestAccrualCredit {
		return []engine.Entry{
			{AccountID: in.ExpenseAccountID, Amount: it.amount, Role: engine.RoleInterestExpense},
			{AccountID: account.ID, Amount: it.amount, Role: engine.RoleLiability},
		}
	}
	return []engine.Entry{
		{AccountID: account.ID, Amount: it.amount, Role: engine.RoleAsset},
		{AccountID: in.IncomeAccountID, Amount: it.amount, Role: engine.RoleInterestIncome},
	}
}

// lookupExisting ищет уже сохранённое начисление по uuid или периоду.
func (s *Service) lookupExisting(ctx context.Context, accountID int64, it item) (*model.InterestAccrual, error) {
	existing, err := s.db.InterestAccrualByUUID(ctx, it.uuid)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if it.periodFrom.IsZero() && it.periodTo.IsZero() {
		return nil, nil
	}
	existing, err = s.db.InterestAccrualByPeriod(ctx, accountID, it.kind, it.periodFrom, it.periodTo)
	if err == nil {
		return existing, nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return nil, err
}

// movements возвращает движения по вкладу за период [from, to).
func (s *Service) movements(ctx context.Context, account *model.Account, from, to time.Time) ([]interest.DepositMovement, error) {
	accountID := account.ID
	transactions, err := s.db.ListTransactions(ctx, store.TransactionFilter{
		AccountID: &accountID,
		From:      from.Unix(),
		To:        to.AddDate(0, 0, -1).Unix(),
	})
	if err != nil {
		return nil, err
	}

	var movements []interest.DepositMovement
	for _, transaction := range transactions {
		date := dateOnly(transaction.Date)
		if date.Before(from) || !date.Before(to) {
			continue
		}
		for _, line := range transaction.Lines {
			if line.AccountID != account.ID {
				continue
			}
			amount := line.Amount
			if account.Type == model.TypeAsset && line.Side == model.SideCredit {
				amount = -amount
			}
			if account.Type == model.TypeLiability && line.Side == model.SideDebit {
				amount = -amount
			}
			if amount != 0 {
				movements = append(movements, interest.DepositMovement{
					Date:   date,
					Amount: amount,
				})
			}
		}
	}
	return movements, nil
}

// checkExpenseAccount проверяет статью расходов, если она передана.
func (s *Service) checkExpenseAccount(ctx context.Context, accountID int64) error {
	if accountID == 0 {
		return nil
	}
	account, err := s.db.Account(ctx, accountID)
	if errors.Is(err, store.ErrNotFound) {
		return accrualError(store.CodeInterestExpenseAccount, "статья расходов не найдена")
	}
	if err != nil {
		return err
	}
	if account.IsArchived || !account.IsLeaf || account.Type != model.TypeExpense {
		return accrualError(store.CodeInterestExpenseAccount,
			"статья расходов должна быть листовым счётом типа «расход»: "+account.FullPath)
	}
	return nil
}

// checkIncomeAccount проверяет статью доходов, если она передана.
func (s *Service) checkIncomeAccount(ctx context.Context, accountID int64) error {
	if accountID == 0 {
		return nil
	}
	account, err := s.db.Account(ctx, accountID)
	if errors.Is(err, store.ErrNotFound) {
		return accrualError(store.CodeInterestIncomeAccount, "статья доходов не найдена")
	}
	if err != nil {
		return err
	}
	if account.IsArchived || !account.IsLeaf || account.Type != model.TypeIncome {
		return accrualError(store.CodeInterestIncomeAccount,
			"статья доходов должна быть листовым счётом типа «доход»: "+account.FullPath)
	}
	return nil
}

// itemView переводит внутреннее начисление в данные предпросмотра.
func itemView(it item, capitalization bool) Item {
	status := model.InterestAccrued
	if capitalization && it.kind != model.InterestAccrualCredit {
		status = model.InterestCapitalized
	}
	if it.existing != nil {
		status = it.existing.Status
		return Item{
			UUID:       it.existing.UUID,
			Date:       it.existing.Date,
			Kind:       it.existing.Kind,
			Amount:     it.existing.Amount,
			Base:       it.existing.BaseAmount,
			RatePPM:    it.existing.RatePPM,
			PeriodFrom: it.existing.PeriodFrom,
			PeriodTo:   it.existing.PeriodTo,
			Status:     status,
			Existing:   true,
		}
	}
	return Item{
		UUID:       it.uuid,
		Date:       it.date,
		Kind:       it.kind,
		Amount:     it.amount,
		Base:       it.base,
		RatePPM:    it.ratePPM,
		PeriodFrom: it.periodFrom,
		PeriodTo:   it.periodTo,
		Status:     status,
	}
}

// accrualKind определяет вид начисления по типу счёта.
func accrualKind(kind model.InterestAccrualKind, accountType model.AccountType) (model.InterestAccrualKind, error) {
	if kind == "" {
		switch accountType {
		case model.TypeLiability:
			return model.InterestAccrualCredit, nil
		case model.TypeAsset:
			return model.InterestAccrualDeposit, nil
		default:
			return "", accrualError(store.CodeInterestAccountInvalid,
				"начисление возможно только по счёту кредита или вклада")
		}
	}
	if !kind.Valid() {
		return "", accrualError(store.CodeInterestKind, "недопустимый вид начисления")
	}
	switch {
	case kind == model.InterestAccrualCredit && accountType != model.TypeLiability:
		return "", accrualError(store.CodeInterestKind,
			"проценты по кредиту начисляются только на счёте типа «пассив»")
	case kind != model.InterestAccrualCredit && accountType != model.TypeAsset:
		return "", accrualError(store.CodeInterestKind,
			"проценты и бонусы по вкладу начисляются только на счёте типа «актив»")
	}
	return kind, nil
}

// periodic сообщает, задан ли период начисления.
func periodic(in Input) bool {
	return !in.PeriodFrom.IsZero() || !in.PeriodTo.IsZero()
}

// accrualPeriod проверяет и нормализует период.
func accrualPeriod(in Input) (time.Time, time.Time, error) {
	if in.PeriodFrom.IsZero() || in.PeriodTo.IsZero() {
		return time.Time{}, time.Time{}, accrualError(store.CodeInterestPeriod,
			"для периодического начисления нужны обе границы периода")
	}
	from, to := dateOnly(in.PeriodFrom), dateOnly(in.PeriodTo)
	if !from.Before(to) {
		return time.Time{}, time.Time{}, accrualError(store.CodeInterestPeriod,
			"начало периода должно быть раньше конца")
	}
	return from, to, nil
}

// manualDate проверяет дату ручного начисления.
func manualDate(date time.Time) (time.Time, error) {
	if date.IsZero() {
		return time.Time{}, accrualError(store.CodeInterestDate, "не задана дата начисления")
	}
	return dateOnly(date), nil
}

// dateOnly отбрасывает время и приводит дату к UTC.
func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// daysBetween возвращает число календарных дней между датами.
func daysBetween(from, to time.Time) int {
	days := 0
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		days++
	}
	return days
}

// manualUUID возвращает переданный идентификатор или создаёт новый.
func manualUUID(id string) (string, error) {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id), nil
	}
	return newUUID()
}

// newUUID создаёт идентификатор начисления.
func newUUID() (string, error) {
	generated, err := uuid.New()
	if err != nil {
		return "", accrualError(store.CodeInterestUUID,
			"не удалось создать uuid начисления")
	}
	return generated, nil
}

// accrualError создаёт ошибку валидации начисления.
func accrualError(code, message string) error {
	return &store.ValidationError{Code: code, Message: message}
}
