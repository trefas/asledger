package sync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/store/storetest"
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
	db := storetest.OpenTemp(t)
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

// TestASL_7_RebaseDeterministic проверяет детерминизм переигровки: одинаковые
// входные данные дают одинаковый результат и одинаковые предупреждения.
func TestASL_7_RebaseDeterministic(t *testing.T) {
	ctx := context.Background()

	run := func(t *testing.T) []string {
		t.Helper()
		svc, actor, expenseID, walletID := newTestService(t)
		if _, err := svc.Push(ctx, PushRequest{
			ClientID: "dev", ActorID: actor, BaseSeq: 0, CommitID: "first",
			Operations: []Operation{expenseOp("det-1", 1000, expenseID, walletID)},
		}); err != nil {
			t.Fatalf("первый push: %v", err)
		}
		result, err := svc.Push(ctx, PushRequest{
			ClientID: "dev", ActorID: actor, BaseSeq: 0, CommitID: "rebased",
			Operations: []Operation{expenseOp("det-2", 2000, expenseID, walletID)},
		})
		if err != nil {
			t.Fatalf("rebase push: %v", err)
		}
		if result.Status != StatusAcceptedWithWarnings {
			t.Fatalf("статус = %q, ожидался accepted-with-warnings", result.Status)
		}
		return result.Warnings
	}

	first := run(t)
	second := run(t)
	if strings.Join(first, "|") != strings.Join(second, "|") {
		t.Fatalf("переигровка не детерминирована: %v против %v", first, second)
	}
}

// TestASL_7_ChainVerification проверяет критерий эпика: два клиента с
// пересекающимися проводками после pull идентичны, хэш-цепочка совпадает.
func TestASL_7_ChainVerification(t *testing.T) {
	ctx := context.Background()
	svc, actor, expenseID, walletID := newTestService(t)

	if _, err := svc.Push(ctx, PushRequest{
		ClientID: "dev-1", ActorID: actor, BaseSeq: 0, CommitID: "a",
		Operations: []Operation{expenseOp("ver-1", 1000, expenseID, walletID)},
	}); err != nil {
		t.Fatalf("push dev-1: %v", err)
	}
	if _, err := svc.Push(ctx, PushRequest{
		ClientID: "dev-2", ActorID: actor, BaseSeq: 1, CommitID: "b",
		Operations: []Operation{expenseOp("ver-2", 2000, expenseID, walletID)},
	}); err != nil {
		t.Fatalf("push dev-2: %v", err)
	}

	clientA, err := svc.Pull(ctx, 0, "")
	if err != nil {
		t.Fatalf("Pull клиента A: %v", err)
	}
	clientB, err := svc.Pull(ctx, 0, "")
	if err != nil {
		t.Fatalf("Pull клиента B: %v", err)
	}

	if len(clientA.Commits) != 2 || len(clientB.Commits) != len(clientA.Commits) {
		t.Fatalf("клиенты видят разное число коммитов: %d и %d", len(clientA.Commits), len(clientB.Commits))
	}
	if len(clientA.Transactions) != len(clientB.Transactions) {
		t.Fatalf("клиенты видят разное число проводок: %d и %d",
			len(clientA.Transactions), len(clientB.Transactions))
	}

	// Клиент досчитывает цепочку от нуля и сверяет с хешом сервера.
	recompute := func(pull *Pull) string {
		h := ""
		for _, commit := range pull.Commits {
			h = ChainHash(h, commit.OpsJSON)
		}
		return h
	}
	if got := recompute(clientA); got != clientA.ChainHash {
		t.Fatalf("цепочка клиента A = %q, хеш сервера = %q", got, clientA.ChainHash)
	}
	if recompute(clientA) != recompute(clientB) {
		t.Fatal("клиенты досчитали разные цепочки")
	}

	// Клиент на голове подтверждает цепочку без полного ресинка.
	head, err := svc.Pull(ctx, clientA.HeadSeq, clientA.ChainHash)
	if err != nil {
		t.Fatalf("Pull на голове: %v", err)
	}
	if head.ResyncRequired {
		t.Fatal("верная цепочка на голове потребовала ресинка")
	}
}

// TestASL_9_DuplicateHeuristic проверяет эвристику возможных дублей.
func TestASL_9_DuplicateHeuristic(t *testing.T) {
	ctx := context.Background()
	svc, actor, expenseID, walletID := newTestService(t)

	head, err := svc.db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	first, err := svc.Push(ctx, PushRequest{
		ClientID: "dev-1", ActorID: actor, BaseSeq: head, CommitID: "c1",
		Operations: []Operation{expenseOp("dup-1", 15000, expenseID, walletID)},
	})
	if err != nil {
		t.Fatalf("первый push: %v", err)
	}
	if len(first.Duplicates) != 0 || first.Status != StatusAccepted {
		t.Fatalf("первый push: статус %q, дубли %+v", first.Status, first.Duplicates)
	}

	// Вторая операция того же дня с теми же счетами и суммой — возможный дубль.
	head, err = svc.db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	second, err := svc.Push(ctx, PushRequest{
		ClientID: "dev-2", ActorID: actor, BaseSeq: head, CommitID: "c2",
		Operations: []Operation{expenseOp("dup-2", 15000, expenseID, walletID)},
	})
	if err != nil {
		t.Fatalf("второй push: %v", err)
	}
	if second.Status != StatusAcceptedWithWarnings {
		t.Errorf("статус второго push: %q, ожидался accepted-with-warnings", second.Status)
	}
	// Дубль не блокирует приём: операция принята.
	if len(second.Applied) != 1 || second.Applied[0] != "dup-2" {
		t.Errorf("принятые операции: %v", second.Applied)
	}
	if len(second.Duplicates) != 1 {
		t.Fatalf("дубли: %+v", second.Duplicates)
	}
	if second.Duplicates[0].UUID != "dup-2" || len(second.Duplicates[0].Existing) != 1 ||
		second.Duplicates[0].Existing[0] != "dup-1" {
		t.Errorf("содержимое плашки дубля: %+v", second.Duplicates[0])
	}

	t.Run("без ложных срабатываний", func(t *testing.T) {
		// Другая сумма в тот же день.
		head, err := svc.db.HeadSeq(ctx)
		if err != nil {
			t.Fatalf("HeadSeq: %v", err)
		}
		other, err := svc.Push(ctx, PushRequest{
			ClientID: "dev-3", ActorID: actor, BaseSeq: head, CommitID: "c3",
			Operations: []Operation{expenseOp("dup-3", 16000, expenseID, walletID)},
		})
		if err != nil {
			t.Fatalf("push(другая сумма): %v", err)
		}
		if len(other.Duplicates) != 0 {
			t.Errorf("ложный дубль по сумме: %+v", other.Duplicates)
		}

		// Та же сумма, но другой день.
		nextDay := expenseOp("dup-4", 15000, expenseID, walletID)
		nextDay.Date = time.Now().AddDate(0, 0, 1)
		head, err = svc.db.HeadSeq(ctx)
		if err != nil {
			t.Fatalf("HeadSeq: %v", err)
		}
		other, err = svc.Push(ctx, PushRequest{
			ClientID: "dev-4", ActorID: actor, BaseSeq: head, CommitID: "c4",
			Operations: []Operation{nextDay},
		})
		if err != nil {
			t.Fatalf("push(другой день): %v", err)
		}
		if len(other.Duplicates) != 0 {
			t.Errorf("ложный дубль по дате: %+v", other.Duplicates)
		}
	})
}
