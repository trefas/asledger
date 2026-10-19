package sync

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// nowTime возвращает текущее время — допустимую дату операции.
func nowTime() time.Time { return time.Now() }

// kop переводит копейки в money.Kop.
func kop(v int64) money.Kop { return money.FromKopecks(v) }

// chartJSON — минимальный план счетов для тестов синхронизации.
const chartJSON = `{"accounts":[
  {"name":"Активы","type":"актив","children":[{"name":"Наличные","type":"актив"}]},
  {"name":"Расходы","type":"расход","children":[{"name":"Продукты","type":"расход"}]}
]}`

// newTestService поднимает книгу, пользователя и сервис синхронизации.
func newTestService(t *testing.T) (*Service, int64, int64, int64) {
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
	user, err := db.CreateUser(ctx, "Андрей", "секрет")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	chart, err := store.ParseChart([]byte(chartJSON))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}
	if _, err := db.ImportChart(ctx, chart); err != nil {
		t.Fatalf("ImportChart: %v", err)
	}
	expense, err := db.AccountByPath(ctx, "Расходы:Продукты")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	wallet, err := db.AccountByPath(ctx, "Активы:Наличные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	return New(db), user.ID, expense.ID, wallet.ID
}

// expenseOp собирает операцию траты для пакета.
func expenseOp(uuid string, amount int64, expenseID, walletID int64) Operation {
	return Operation{
		OpType: model.OpExpense,
		UUID:   uuid,
		Date:   nowTime(),
		Entries: []Entry{
			{AccountID: expenseID, Amount: kop(amount), Role: "расход"},
			{AccountID: walletID, Amount: kop(amount), Role: "источник"},
		},
	}
}

// TestASL_7_HashChain проверяет хэш-цепочку коммитов.
func TestASL_7_HashChain(t *testing.T) {
	ctx := context.Background()
	svc, actor, expenseID, walletID := newTestService(t)
	db := svc.db

	ops1 := []Operation{expenseOp("chain-1", 10000, expenseID, walletID)}
	payload1 := CommitPayload{CommitID: "c1", Operations: ops1}
	opsJSON1, _ := json.Marshal(payload1)

	first, err := svc.Push(ctx, PushRequest{
		ClientID: "dev-1", ActorID: actor, BaseSeq: 0, CommitID: "c1", Operations: ops1,
	})
	if err != nil {
		t.Fatalf("первый push: %v", err)
	}
	commit1, err := db.CommitByClientPayload(ctx, "dev-1", string(opsJSON1))
	if err != nil {
		t.Fatalf("CommitByClientPayload: %v", err)
	}
	if commit1.Hash != ChainHash("", string(opsJSON1)) {
		t.Fatalf("первое звено цепочки = %q", commit1.Hash)
	}

	ops2 := []Operation{expenseOp("chain-2", 20000, expenseID, walletID)}
	payload2 := CommitPayload{CommitID: "c2", Operations: ops2}
	opsJSON2, _ := json.Marshal(payload2)
	if _, err := svc.Push(ctx, PushRequest{
		ClientID: "dev-1", ActorID: actor, BaseSeq: first.ResultSeq, CommitID: "c2", Operations: ops2,
	}); err != nil {
		t.Fatalf("второй push: %v", err)
	}
	commit2, err := db.CommitByClientPayload(ctx, "dev-1", string(opsJSON2))
	if err != nil {
		t.Fatalf("CommitByClientPayload: %v", err)
	}
	if commit2.Hash != ChainHash(commit1.Hash, string(opsJSON2)) {
		t.Fatalf("второе звено цепочки = %q, ожидалось %q",
			commit2.Hash, ChainHash(commit1.Hash, string(opsJSON2)))
	}
	if commit1.Hash == commit2.Hash {
		t.Fatal("звенья цепочки совпали")
	}

	t.Run("верификация совпадает", func(t *testing.T) {
		pull, err := svc.Pull(ctx, commit1.ResultSeq, commit1.Hash)
		if err != nil {
			t.Fatalf("Pull: %v", err)
		}
		if pull.ResyncRequired {
			t.Fatal("верная цепочка потребовала ресинка")
		}
	})

	t.Run("расхождение требует ресинка", func(t *testing.T) {
		pull, err := svc.Pull(ctx, commit1.ResultSeq, "чужой-хеш")
		if err != nil {
			t.Fatalf("Pull: %v", err)
		}
		if !pull.ResyncRequired {
			t.Fatal("расхождение хешей не потребовало ресинка")
		}
	})
}

// TestASL_7_ChainHashDeterministic проверяет воспроизводимость хеша.
func TestASL_7_ChainHashDeterministic(t *testing.T) {
	a := ChainHash("prev", `{"a":1}`)
	b := ChainHash("prev", `{"a":1}`)
	c := ChainHash("prev", `{"a":2}`)
	if a != b {
		t.Fatal("хеш не воспроизводим при одинаковых данных")
	}
	if a == c {
		t.Fatal("изменение ops_json не изменило хеш")
	}
	if len(a) != 64 {
		t.Fatalf("длина хеша %d, ожидалось 64", len(a))
	}
}
