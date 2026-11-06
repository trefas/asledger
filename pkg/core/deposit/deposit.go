// Package deposit реализует снятие средств с вклада с разбивкой на тело и
// начисленные проценты.
//
// Тело вклада переводится с вклада на счёт-получатель отдельной проводкой,
// проценты признаются доходом второй проводкой. Обе проводки сохраняются
// одной транзакцией БД, поэтому частичного снятия не бывает.
package deposit

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

// Service — сервис снятия с вклада поверх книги и движка проводок.
type Service struct {
	db     *store.DB
	engine *engine.Engine
	calc   interest.DepositCalculator
}

// New создаёт сервис снятия с вклада.
func New(db *store.DB) *Service {
	return NewWithEngine(db, engine.New(db))
}

// NewWithEngine создаёт сервис с готовым движком проводок.
func NewWithEngine(db *store.DB, eng *engine.Engine) *Service {
	return &Service{db: db, engine: eng}
}

// WithdrawalInput — запрос на снятие с вклада.
type WithdrawalInput struct {
	// UUID — базовый идентификатор операции; пустой означает «сгенерировать».
	UUID string
	// Date — дата снятия.
	Date time.Time
	// AccountID — счёт вклада (листовой актив).
	AccountID int64
	// DestinationAccountID — счёт-получатель (листовой актив).
	DestinationAccountID int64
	// IncomeAccountID — статья доходов по процентам (листовой доход).
	IncomeAccountID int64
	// Amount — сумма к снятию.
	Amount money.Kop
	// Description — описание проводок; пустое заменяется стандартным.
	Description string
	// ActorID — автор проводок.
	ActorID int64
	// Source — источник проводок ('web', 'api:<client_id>').
	Source string
}

// Preview — разбивка снятия до подтверждения.
type Preview struct {
	UUID                 string
	Date                 time.Time
	AccountID            int64
	DestinationAccountID int64
	IncomeAccountID      int64
	// Requested — запрошенная сумма.
	Requested money.Kop
	// Principal — часть суммы с тела вклада.
	Principal money.Kop
	// Interest — часть суммы с начисленных процентов.
	Interest money.Kop
	// Total — фактически распределённая сумма.
	Total money.Kop
	// Balance и AccruedInterest — остатки до снятия.
	Balance         money.Kop
	AccruedInterest money.Kop
	// RemainingBalance и RemainingInterest — остатки после снятия.
	RemainingBalance  money.Kop
	RemainingInterest money.Kop
}

// Posted — результат проведённого снятия.
type Posted struct {
	// UUID — базовый идентификатор операции.
	UUID string
	// PrincipalTransaction и InterestTransaction — сохранённые проводки.
	PrincipalTransaction *model.Transaction
	InterestTransaction  *model.Transaction
	// Principal и Interest — разбивка.
	Principal money.Kop
	Interest  money.Kop
	// Total — общая сумма.
	Total money.Kop
}

// Preview рассчитывает разбивку снятия, не записывая проводки.
func (s *Service) Preview(ctx context.Context, in WithdrawalInput) (*Preview, error) {
	id, err := s.uuid(in.UUID)
	if err != nil {
		return nil, err
	}
	in.UUID = id
	preview, err := s.prepare(ctx, in)
	return preview, err
}

// Post проводит снятие двумя проводками в одной транзакции БД.
//
// Повтор с тем же uuid идемпотентен: возвращаются уже сохранённые проводки
// и фактическая разбивка.
func (s *Service) Post(ctx context.Context, in WithdrawalInput) (*Posted, error) {
	if strings.TrimSpace(in.UUID) != "" {
		posted, err := s.existing(ctx, in)
		if err == nil {
			return posted, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
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

	var transactions []*model.Transaction
	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = "Снятие с вклада"
	}
	if preview.Principal > 0 {
		prepared, err := s.engine.Prepare(ctx, engine.Operation{
			OpType:      model.OpTransfer,
			UUID:        in.UUID + "-principal",
			Date:        preview.Date,
			Description: description,
			Source:      in.Source,
			Entries: []engine.Entry{
				{AccountID: in.AccountID, Amount: preview.Principal, Role: engine.RoleFrom},
				{AccountID: in.DestinationAccountID, Amount: preview.Principal, Role: engine.RoleTo},
			},
		}, in.ActorID)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, prepared.Transaction)
	}
	if preview.Interest > 0 {
		prepared, err := s.engine.Prepare(ctx, engine.Operation{
			OpType:      model.OpIncome,
			UUID:        in.UUID + "-interest",
			Date:        preview.Date,
			Description: "Проценты по вкладу",
			Source:      in.Source,
			Entries: []engine.Entry{
				{AccountID: in.DestinationAccountID, Amount: preview.Interest, Role: engine.RoleReceipt},
				{AccountID: in.IncomeAccountID, Amount: preview.Interest, Role: engine.RoleIncome},
			},
		}, in.ActorID)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, prepared.Transaction)
	}

	ids, err := s.db.InsertTransactions(ctx, transactions)
	if err != nil {
		return nil, err
	}

	posted := &Posted{UUID: in.UUID, Principal: preview.Principal, Interest: preview.Interest, Total: preview.Total}
	if preview.Principal > 0 {
		tr, err := s.db.Transaction(ctx, ids[0])
		if err != nil {
			return nil, err
		}
		posted.PrincipalTransaction = tr
	}
	if preview.Interest > 0 {
		index := len(ids) - 1
		tr, err := s.db.Transaction(ctx, ids[index])
		if err != nil {
			return nil, err
		}
		posted.InterestTransaction = tr
		if _, err := s.db.PayInterestAccruals(ctx, in.AccountID,
			[]model.InterestAccrualKind{model.InterestAccrualDeposit, model.InterestAccrualBonus},
			preview.Interest, tr.ID, preview.Date); err != nil {
			return nil, err
		}
	}
	return posted, nil
}

// prepare проверяет счета и рассчитывает разбивку.
func (s *Service) prepare(ctx context.Context, in WithdrawalInput) (*Preview, error) {
	if in.Amount <= 0 {
		return nil, withdrawalError(store.CodeDepositAmount, "сумма снятия должна быть положительной")
	}
	date, err := withdrawalDate(in.Date)
	if err != nil {
		return nil, err
	}

	account, err := s.db.Account(ctx, in.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, withdrawalError(store.CodeDepositAccountInvalid, "счёт вклада не найден")
	}
	if err != nil {
		return nil, err
	}
	if account.IsArchived || !account.IsLeaf || account.Type != model.TypeAsset {
		return nil, withdrawalError(store.CodeDepositAccountInvalid,
			"снятие возможно только с листового счёта типа «актив»: "+account.FullPath)
	}

	settings, err := s.db.DepositSettings(ctx, in.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, withdrawalError(store.CodeDepositSettingsMissing,
			"у счёта нет настроек вклада: "+account.FullPath)
	}
	if err != nil {
		return nil, err
	}

	destination, err := s.db.Account(ctx, in.DestinationAccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, withdrawalError(store.CodeDepositDestinationInvalid, "счёт-получатель не найден")
	}
	if err != nil {
		return nil, err
	}
	if destination.IsArchived || !destination.IsLeaf || destination.Type != model.TypeAsset {
		return nil, withdrawalError(store.CodeDepositDestinationInvalid,
			"счёт-получатель должен быть листовым счётом типа «актив»: "+destination.FullPath)
	}
	if destination.ID == account.ID {
		return nil, withdrawalError(store.CodeDepositDestinationInvalid,
			"счёт-получатель совпадает со счётом вклада")
	}

	if in.IncomeAccountID != 0 {
		income, err := s.db.Account(ctx, in.IncomeAccountID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, withdrawalError(store.CodeDepositIncomeInvalid, "статья доходов не найдена")
		}
		if err != nil {
			return nil, err
		}
		if income.IsArchived || !income.IsLeaf || income.Type != model.TypeIncome {
			return nil, withdrawalError(store.CodeDepositIncomeInvalid,
				"статья доходов должна быть листовым счётом типа «доход»: "+income.FullPath)
		}
	}

	balance, err := s.db.BalanceAt(ctx, in.AccountID, date.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	accrued, err := s.accrued(ctx, in.AccountID, date)
	if err != nil {
		return nil, err
	}

	payment, err := s.calc.PreviewWithdrawal(interest.DepositAccount{
		Balance:         balance,
		AccruedInterest: accrued,
		Settings:        *settings,
	}, in.Amount, date)
	if err != nil {
		if errors.Is(err, interest.ErrPaymentAmount) {
			return nil, withdrawalError(store.CodeDepositAmount, "сумма снятия должна быть положительной")
		}
		return nil, withdrawalError(store.CodeDepositAccountInvalid,
			"не удалось рассчитать снятие: "+err.Error())
	}
	if payment.Total == 0 {
		return nil, withdrawalError(store.CodeDepositNothingToWithdraw,
			"снимать нечего: остаток вклада и начисленные проценты равны нулю")
	}
	if payment.Interest > 0 && in.IncomeAccountID == 0 {
		return nil, withdrawalError(store.CodeDepositIncomeInvalid,
			"для снятия процентов выберите статью доходов")
	}

	return &Preview{
		UUID:                 in.UUID,
		Date:                 date,
		AccountID:            in.AccountID,
		DestinationAccountID: in.DestinationAccountID,
		IncomeAccountID:      in.IncomeAccountID,
		Requested:            in.Amount,
		Principal:            payment.Principal,
		Interest:             payment.Interest,
		Total:                payment.Total,
		Balance:              balance,
		AccruedInterest:      accrued,
		RemainingBalance:     balance - payment.Principal,
		RemainingInterest:    accrued - payment.Interest,
	}, nil
}

// existing восстанавливает результат уже проведённого снятия.
func (s *Service) existing(ctx context.Context, in WithdrawalInput) (*Posted, error) {
	principal, err := s.db.TransactionByUUID(ctx, in.UUID+"-principal")
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	interest, interestErr := s.db.TransactionByUUID(ctx, in.UUID+"-interest")
	if interestErr != nil && !errors.Is(interestErr, store.ErrNotFound) {
		return nil, interestErr
	}
	if err != nil && interestErr != nil {
		return nil, fmt.Errorf("%w: снятие %s", store.ErrNotFound, in.UUID)
	}

	posted := &Posted{UUID: in.UUID}
	if err == nil {
		posted.PrincipalTransaction = principal
		for _, line := range principal.Lines {
			if line.AccountID == in.AccountID && line.Side == model.SideCredit {
				posted.Principal += line.Amount
			}
		}
	}
	if interestErr == nil {
		posted.InterestTransaction = interest
		for _, line := range interest.Lines {
			if line.AccountID == in.IncomeAccountID && line.Side == model.SideCredit {
				posted.Interest += line.Amount
			}
		}
	}
	posted.Total = posted.Principal + posted.Interest
	return posted, nil
}

// accrued суммирует начисленные проценты и бонусы по вкладу на дату.
func (s *Service) accrued(ctx context.Context, accountID int64, date time.Time) (money.Kop, error) {
	interestDue, err := s.db.AccruedInterestAt(ctx, accountID, model.InterestAccrualDeposit, date)
	if err != nil {
		return 0, err
	}
	bonus, err := s.db.AccruedInterestAt(ctx, accountID, model.InterestAccrualBonus, date)
	if err != nil {
		return 0, err
	}
	return interestDue + bonus, nil
}

// uuid возвращает переданный идентификатор или создаёт новый.
func (s *Service) uuid(id string) (string, error) {
	if strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id), nil
	}
	generated, err := uuid.New()
	if err != nil {
		return "", fmt.Errorf("deposit: не удалось создать uuid операции: %w", err)
	}
	return generated, nil
}

// withdrawalDate проверяет дату и отбрасывает время.
func withdrawalDate(t time.Time) (time.Time, error) {
	if t.IsZero() {
		return time.Time{}, withdrawalError(store.CodeDepositDate, "не задана дата снятия")
	}
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
}

// withdrawalError создаёт ошибку валидации снятия.
func withdrawalError(code, message string) error {
	return &store.ValidationError{Code: code, Message: message}
}
