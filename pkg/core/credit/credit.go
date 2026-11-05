// Package credit реализует погашение кредита с автоматической разбивкой
// суммы платежа на тело долга и начисленные проценты.
//
// Пользователь выбирает кредитный счёт, счёт-источник, статью процентов и
// сумму. Сервис читает остаток долга и начисления со статусом «accrued»,
// применяет порядок погашения из настроек кредита и формирует одну составную
// проводку: тело — пассив DEBIT, проценты — расход DEBIT, источник — актив
// CREDIT. Суммы — целые копейки, ставки — целые ppm.
package credit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/interest"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/uuid"
)

// Service — сервис погашения кредита поверх книги и движка проводок.
type Service struct {
	db     *store.DB
	engine *engine.Engine
	calc   interest.CreditCalculator
}

// New создаёт сервис погашения кредита.
func New(db *store.DB) *Service {
	return NewWithEngine(db, engine.New(db))
}

// NewWithEngine создаёт сервис с готовым движком проводок.
func NewWithEngine(db *store.DB, eng *engine.Engine) *Service {
	return &Service{db: db, engine: eng}
}

// RepaymentInput — запрос на погашение кредита.
type RepaymentInput struct {
	// UUID — идентификатор проводки; пустой означает «сгенерировать».
	UUID string
	// Date — дата погашения.
	Date time.Time
	// AccountID — кредитный счёт (листовой пассив).
	AccountID int64
	// SourceAccountID — счёт-источник (листовой актив).
	SourceAccountID int64
	// InterestAccountID — статья расходов на проценты (листовой расход).
	// Может быть пустой, если процентов к погашению нет.
	InterestAccountID int64
	// Amount — сумма, которую пользователь готов внести.
	Amount money.Kop
	// Description — описание проводки; пустое заменяется стандартным.
	Description string
	// ActorID — автор проводки.
	ActorID int64
	// Source — источник проводки ('web', 'api:<client_id>').
	Source string
}

// Preview — разбивка платежа до подтверждения.
type Preview struct {
	UUID              string
	Date              time.Time
	AccountID         int64
	SourceAccountID   int64
	InterestAccountID int64
	// Requested — сумма, которую вносит пользователь.
	Requested money.Kop
	// Principal — часть суммы в погашение тела долга.
	Principal money.Kop
	// Interest — часть суммы в погашение начисленных процентов.
	Interest money.Kop
	// Total — фактически распределённая сумма; не больше Requested.
	Total money.Kop
	// Debt — текущий остаток тела долга.
	Debt money.Kop
	// AccruedInterest — текущие начисленные проценты.
	AccruedInterest money.Kop
	// RemainingDebt и RemainingInterest — остатки после платежа.
	RemainingDebt     money.Kop
	RemainingInterest money.Kop
	// InGrace — на дату действует беспроцентный период.
	InGrace bool
}

// Posted — результат проведённого погашения.
type Posted struct {
	// UUID — идентификатор проводки.
	UUID string
	// Transaction — сохранённая проводка.
	Transaction *model.Transaction
	// Principal — погашено тела долга.
	Principal money.Kop
	// Interest — погашено процентов.
	Interest money.Kop
	// Total — общая сумма проводки.
	Total money.Kop
}

// Preview рассчитывает разбивку платежа, не записывая проводку.
//
// Если UUID пуст, возвращается сгенерированный идентификатор: его нужно
// передать в Post, чтобы повторное подтверждение было идемпотентным.
func (s *Service) Preview(ctx context.Context, in RepaymentInput) (*Preview, error) {
	id, err := s.uuid(in.UUID)
	if err != nil {
		return nil, err
	}
	in.UUID = id
	preview, err := s.prepare(ctx, in)
	return preview, err
}

// Post проводит погашение кредита одной составной проводкой.
//
// Повтор с тем же uuid идемпотентен: возвращается уже сохранённая проводка и
// её фактическая разбивка. Суммы строк и остатки повторно не пересчитываются,
// поэтому повтор не зависит от изменившегося остатка долга.
func (s *Service) Post(ctx context.Context, in RepaymentInput) (*Posted, error) {
	if strings.TrimSpace(in.UUID) != "" {
		existing, err := s.db.TransactionByUUID(ctx, in.UUID)
		switch {
		case err == nil:
			return postedFromTransaction(existing, in)
		case !errors.Is(err, store.ErrNotFound):
			return nil, err
		}
	}

	id, err := s.uuid(in.UUID)
	if err != nil {
		return nil, err
	}
	in.UUID = id

	preview, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}

	entries := make([]engine.Entry, 0, 3)
	if preview.Principal > 0 {
		entries = append(entries, engine.Entry{
			AccountID: in.AccountID, Amount: preview.Principal, Role: engine.RoleLiability,
		})
	}
	if preview.Interest > 0 {
		entries = append(entries, engine.Entry{
			AccountID: in.InterestAccountID, Amount: preview.Interest, Role: engine.RoleInterest,
		})
	}
	entries = append(entries, engine.Entry{
		AccountID: in.SourceAccountID, Amount: preview.Total, Role: engine.RoleSource,
	})

	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = "Погашение кредита"
	}
	operation := engine.Operation{
		OpType:      model.OpDebtPayment,
		UUID:        in.UUID,
		Date:        preview.Date,
		Description: description,
		Source:      in.Source,
		Entries:     entries,
	}
	tr, err := s.engine.Post(ctx, operation, in.ActorID)
	if err != nil {
		return nil, err
	}
	return postedFromTransaction(tr, in)
}

// prepare проверяет счета и рассчитывает разбивку платежа.
func (s *Service) prepare(ctx context.Context, in RepaymentInput) (*Preview, error) {
	if in.Amount <= 0 {
		return nil, repaymentError(store.CodeCreditAmount, "сумма погашения должна быть положительной")
	}
	date, err := repaymentDate(in.Date)
	if err != nil {
		return nil, err
	}

	account, err := s.db.Account(ctx, in.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, repaymentError(store.CodeCreditAccountInvalid, "счёт кредита не найден")
	}
	if err != nil {
		return nil, err
	}
	if account.IsArchived || !account.IsLeaf || account.Type != model.TypeLiability {
		return nil, repaymentError(store.CodeCreditAccountInvalid,
			"погашение возможно только по листовому счёту типа «пассив»: "+account.FullPath)
	}

	settings, err := s.db.CreditSettings(ctx, in.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, repaymentError(store.CodeCreditSettingsMissing,
			"у счёта нет настроек кредита: "+account.FullPath)
	}
	if err != nil {
		return nil, err
	}

	source, err := s.db.Account(ctx, in.SourceAccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, repaymentError(store.CodeCreditSourceInvalid, "счёт-источник не найден")
	}
	if err != nil {
		return nil, err
	}
	if source.IsArchived || !source.IsLeaf || source.Type != model.TypeAsset {
		return nil, repaymentError(store.CodeCreditSourceInvalid,
			"источник погашения должен быть листовым счётом типа «актив»: "+source.FullPath)
	}

	if in.InterestAccountID != 0 {
		interestAccount, err := s.db.Account(ctx, in.InterestAccountID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, repaymentError(store.CodeCreditInterestAccountInvalid,
				"статья процентов не найдена")
		}
		if err != nil {
			return nil, err
		}
		if interestAccount.IsArchived || !interestAccount.IsLeaf || interestAccount.Type != model.TypeExpense {
			return nil, repaymentError(store.CodeCreditInterestAccountInvalid,
				"статья процентов должна быть листовым счётом типа «расход»: "+interestAccount.FullPath)
		}
	}

	debt, err := s.db.Balance(ctx, in.AccountID)
	if err != nil {
		return nil, err
	}
	accrued, err := s.db.AccruedInterest(ctx, in.AccountID, model.InterestAccrualCredit)
	if err != nil {
		return nil, err
	}

	payment, err := s.calc.Preview(interest.CreditAccount{
		Balance:         debt,
		AccruedInterest: accrued,
		Settings:        *settings,
	}, in.Amount, date)
	if err != nil {
		if errors.Is(err, interest.ErrPaymentAmount) {
			return nil, repaymentError(store.CodeCreditAmount, "сумма погашения должна быть положительной")
		}
		return nil, repaymentError(store.CodeCreditAccountInvalid,
			"не удалось рассчитать погашение: "+err.Error())
	}
	if payment.Total == 0 {
		return nil, repaymentError(store.CodeCreditNothingToPay,
			"погашать нечего: долг и начисленные проценты равны нулю")
	}
	if payment.Interest > 0 && in.InterestAccountID == 0 {
		return nil, repaymentError(store.CodeCreditInterestAccountInvalid,
			"для погашения процентов выберите статью расходов")
	}

	return &Preview{
		UUID:              in.UUID,
		Date:              date,
		AccountID:         in.AccountID,
		SourceAccountID:   in.SourceAccountID,
		InterestAccountID: in.InterestAccountID,
		Requested:         in.Amount,
		Principal:         payment.Principal,
		Interest:          payment.Interest,
		Total:             payment.Total,
		Debt:              debt,
		AccruedInterest:   accrued,
		RemainingDebt:     debt - payment.Principal,
		RemainingInterest: accrued - payment.Interest,
		InGrace:           s.calc.InGrace(*settings, date),
	}, nil
}

// postedFromTransaction восстанавливает разбивку из сохранённой проводки.
//
// Тело долга — дебет кредитного счёта, общая сумма — кредит счёта-источника.
// Проценты вычисляются как разность: так повтор запроса возвращает ту же
// разбивку даже после изменения остатка долга.
func postedFromTransaction(tr *model.Transaction, in RepaymentInput) (*Posted, error) {
	var principal, total money.Kop
	for _, line := range tr.Lines {
		if line.AccountID == in.AccountID && line.Side == model.SideDebit {
			principal += line.Amount
		}
		if line.AccountID == in.SourceAccountID && line.Side == model.SideCredit {
			total += line.Amount
		}
	}
	if total == 0 {
		for _, line := range tr.Lines {
			if line.Side == model.SideCredit {
				total += line.Amount
			}
		}
	}
	if total < principal {
		return nil, repaymentError(store.CodeCreditAccountInvalid,
			"проводка не похожа на погашение кредита")
	}
	if principal == 0 && total == 0 {
		return nil, repaymentError(store.CodeCreditAccountInvalid,
			"проводка не похожа на погашение кредита")
	}
	return &Posted{
		UUID:        tr.UUID,
		Transaction: tr,
		Principal:   principal,
		Interest:    total - principal,
		Total:       total,
	}, nil
}

// uuid возвращает переданный идентификатор или создаёт новый.
func (s *Service) uuid(id string) (string, error) {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id), nil
	}
	generated, err := uuid.New()
	if err != nil {
		return "", fmt.Errorf("credit: не удалось создать uuid проводки: %w", err)
	}
	return generated, nil
}

// repaymentDate проверяет дату и отбрасывает время.
func repaymentDate(t time.Time) (time.Time, error) {
	if t.IsZero() {
		return time.Time{}, repaymentError(store.CodeCreditDate, "не задана дата погашения")
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

// repaymentError создаёт ошибку валидации погашения.
func repaymentError(code, message string) error {
	return &store.ValidationError{Code: code, Message: message}
}
