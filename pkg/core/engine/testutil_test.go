package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// newTestEngine открывает временную книгу с миграциями и создаёт движок.
func newTestEngine(t *testing.T) (*Engine, *store.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "book.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return New(db), db
}

// createTestUser создаёт пользователя и завершает тест при ошибке.
func createTestUser(t *testing.T, db *store.DB, name string) int64 {
	t.Helper()
	user, err := db.CreateUser(context.Background(), name, "секрет-"+name)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return user.ID
}

// createTestAccount создаёт счёт и завершает тест при ошибке.
func createTestAccount(t *testing.T, db *store.DB, name string, typ model.AccountType, parent *int64) *model.Account {
	t.Helper()
	acc, err := db.CreateAccount(context.Background(), store.AccountInput{
		Name: name, Type: typ, ParentID: parent,
	})
	if err != nil {
		t.Fatalf("CreateAccount(%q): %v", name, err)
	}
	return acc
}

// accountByPath читает счёт по пути и завершает тест при ошибке.
func accountByPath(t *testing.T, db *store.DB, path string) *model.Account {
	t.Helper()
	acc, err := db.AccountByPath(context.Background(), path)
	if err != nil {
		t.Fatalf("AccountByPath(%q): %v", path, err)
	}
	return acc
}

// fixture — общая книга тестов движка: пользователь, типовые счета и движок.
type fixture struct {
	t       *testing.T
	engine  *Engine
	db      *store.DB
	actor   int64
	wallet  *model.Account // Активы:Наличные
	card    *model.Account // Активы:Карта
	debtor  *model.Account // Активы:Дебиторка (служебный)
	credit  *model.Account // Обязательства:Кредит
	expense *model.Account // Расходы:Продукты
	income  *model.Account // Доходы:Зарплата
}

// newFixture собирает книгу с типовым планом счетов.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	eng, db := newTestEngine(t)

	assets := accountByPath(t, db, "Активы")
	wallet := createTestAccount(t, db, "Наличные", model.TypeAsset, &assets.ID)
	card := createTestAccount(t, db, "Карта", model.TypeAsset, &assets.ID)

	liabilities := createTestAccount(t, db, "Обязательства", model.TypeLiability, nil)
	credit := createTestAccount(t, db, "Кредит", model.TypeLiability, &liabilities.ID)

	expenses := createTestAccount(t, db, "Расходы", model.TypeExpense, nil)
	expense := createTestAccount(t, db, "Продукты", model.TypeExpense, &expenses.ID)

	incomes := createTestAccount(t, db, "Доходы", model.TypeIncome, nil)
	income := createTestAccount(t, db, "Зарплата", model.TypeIncome, &incomes.ID)

	return &fixture{
		t:       t,
		engine:  eng,
		db:      db,
		actor:   createTestUser(t, db, "Андрей"),
		wallet:  wallet,
		card:    card,
		debtor:  accountByPath(t, db, DebtorBranch),
		credit:  credit,
		expense: expense,
		income:  income,
	}
}

// opDate возвращает дату проводки в допустимом диапазоне (сейчас).
func opDate() time.Time { return time.Now() }

// balance возвращает остаток счёта и завершает тест при ошибке.
func (f *fixture) balance(t *testing.T, acc *model.Account) money.Kop {
	t.Helper()
	bal, err := f.db.Balance(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("Balance(%q): %v", acc.FullPath, err)
	}
	return bal
}

// codeOf возвращает код ошибки движка.
func codeOf(err error) string { return CodeOf(err) }

// post выполняет проводку через движок от имени тестового пользователя.
func (f *fixture) post(t *testing.T, op Operation) (*model.Transaction, error) {
	t.Helper()
	return f.engine.Post(context.Background(), op, f.actor)
}

// expenseOp собирает простую трату «одна статья — один источник».
func (f *fixture) expenseOp(uuid string, amount money.Kop) Operation {
	return Operation{
		OpType: model.OpExpense,
		UUID:   uuid,
		Date:   opDate(),
		Entries: []Entry{
			{AccountID: f.expense.ID, Amount: amount, Role: RoleExpense},
			{AccountID: f.wallet.ID, Amount: amount, Role: RoleSource},
		},
	}
}
