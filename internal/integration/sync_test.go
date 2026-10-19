package integration

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// hasWarning сообщает, есть ли предупреждение с подстрокой.
func hasWarning(warnings []string, substring string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substring) {
			return true
		}
	}
	return false
}

// sides суммирует дебет и кредит всех проводок реплики.
func (r *replica) sides() (money.Kop, money.Kop) {
	var debit, credit money.Kop
	for _, tr := range r.transactions {
		for _, line := range tr.Lines {
			if line.Side == model.SideDebit {
				debit += line.Amount
			} else {
				credit += line.Amount
			}
		}
	}
	return debit, credit
}

// TestASL_13_StandTwoClientsClone проверяет интеграционный стенд (#78):
// один сервер на случайном порту, два независимых клиента, клонирование книги.
func TestASL_13_StandTwoClientsClone(t *testing.T) {
	s := newStand(t)
	first := newReplica(t, s, "телефон-андрея")
	second := newReplica(t, s, "телефон-мамы")

	first.clone()
	second.clone()

	head, err := s.db.HeadSeq(context.Background())
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	if first.headSeq != head || second.headSeq != head {
		t.Fatalf("head_seq реплик %d и %d, сервер %d", first.headSeq, second.headSeq, head)
	}
	if first.chainHash != second.chainHash {
		t.Errorf("хеши цепочек различаются: %q и %q", first.chainHash, second.chainHash)
	}
	for _, path := range []string{"Активы:Наличные", "Активы:Карта", "Расходы:Продукты", "Доходы:Зарплата"} {
		serverID := s.accountID(t, path)
		if got := first.account(path); got != serverID {
			t.Errorf("клиент 1: счёт %q = %d, сервер %d", path, got, serverID)
		}
		if got := second.account(path); got != serverID {
			t.Errorf("клиент 2: счёт %q = %d, сервер %d", path, got, serverID)
		}
	}
	if first.state() != second.state() {
		t.Error("реплики пустой книги должны совпадать")
	}
	if err := first.verifyChain(); err != nil {
		t.Errorf("цепочка клиента 1: %v", err)
	}
	if err := second.verifyChain(); err != nil {
		t.Errorf("цепочка клиента 2: %v", err)
	}
}

// TestASL_13_ParallelCommitsConverge проверяет параллельные коммиты двух
// клиентов по одному счёту (#81): сервер сериализует, потерь нет, остатки
// реплик совпадают, балансовое уравнение сохраняется.
func TestASL_13_ParallelCommitsConverge(t *testing.T) {
	s := newStand(t)
	first := newReplica(t, s, "телефон-1")
	second := newReplica(t, s, "телефон-2")
	first.clone()
	second.clone()
	baseHead, err := s.db.HeadSeq(context.Background())
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}

	// Обе реплики стоят на одном base_seq и готовят операции независимо.
	firstOp := first.expense("трата-1", money.FromKopecks(10000))
	secondOp := second.expense("трата-2", money.FromKopecks(25000))

	var wg sync.WaitGroup
	outcomes := make([]pushOutcome, 2)
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		outcomes[0], errs[0] = first.tryPush(baseHead, "коммит-1", firstOp)
	}()
	go func() {
		defer wg.Done()
		outcomes[1], errs[1] = second.tryPush(baseHead, "коммит-2", secondOp)
	}()
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("push %d: %v", i+1, err)
		}
	}
	for i, outcome := range outcomes {
		if outcome.status != http.StatusOK {
			t.Fatalf("push %d: %s", i+1, describeResponse(outcome.status, outcome.raw))
		}
	}

	// Сервер задаёт общий порядок: голова сдвинулась ровно на два коммита.
	head, err := s.db.HeadSeq(context.Background())
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	if head != baseHead+2 {
		t.Fatalf("head_seq = %d, ожидалось %d", head, baseHead+2)
	}
	// Отставший клиент получает rebase-предупреждение — конкуренция видна.
	rebaseWarnings := 0
	for _, outcome := range outcomes {
		if hasWarning(outcome.result.Warnings, "rebase") {
			rebaseWarnings++
		}
	}
	if rebaseWarnings == 0 {
		t.Error("ни один из параллельных коммитов не отмечен rebase")
	}

	first.sync()
	second.sync()
	if len(first.uuids()) != 2 || len(second.uuids()) != 2 {
		t.Fatalf("потеряны операции: у клиентов %d и %d проводок", len(first.uuids()), len(second.uuids()))
	}
	if first.state() != second.state() {
		t.Error("реплики разошлись после параллельных коммитов")
	}
	if err := first.verifyChain(); err != nil {
		t.Errorf("цепочка клиента 1: %v", err)
	}
	if err := second.verifyChain(); err != nil {
		t.Errorf("цепочка клиента 2: %v", err)
	}
	for name, rep := range map[string]*replica{"клиент 1": first, "клиент 2": second} {
		debit, credit := rep.sides()
		if debit != credit {
			t.Errorf("%s: ΣDEBIT %d ≠ ΣCREDIT %d", name, debit.Kopecks(), credit.Kopecks())
		}
	}

	serverBalances, err := s.db.Balances(context.Background())
	if err != nil {
		t.Fatalf("Balances: %v", err)
	}
	for id, want := range serverBalances {
		if got := first.balances()[id]; got != want {
			t.Errorf("остаток счёта %d: у клиента %d, на сервере %d", id, got.Kopecks(), want.Kopecks())
		}
	}
	if got := first.balance("Расходы:Продукты"); got != money.FromKopecks(35000) {
		t.Errorf("расходы: %d, ожидалось 35000", got.Kopecks())
	}
}

// TestASL_13_ConflictsAndWarnings проверяет конфликтные сценарии (#79):
// отставший base_seq даёт rebase-предупреждение, возможный дубль помечается
// плашкой, но пакет принимается, а реплики сходятся к единому состоянию.
func TestASL_13_ConflictsAndWarnings(t *testing.T) {
	s := newStand(t)
	first := newReplica(t, s, "телефон-1")
	second := newReplica(t, s, "телефон-2")
	first.clone()
	second.clone()

	if outcome := first.push("коммит-1", first.expense("трата-а", money.FromKopecks(30000))); outcome.status != http.StatusOK {
		t.Fatalf("первый push: %s", describeResponse(outcome.status, outcome.raw))
	}
	first.sync()

	// Второй клиент отправил пакет, стоя на устаревшем base_seq, и с той же
	// подписью проводки (тип, дата, счета, сумма) — это возможный дубль.
	outcome := second.pushAt(0, "коммит-2", second.expense("трата-б", money.FromKopecks(30000)))
	if outcome.status != http.StatusOK {
		t.Fatalf("конфликтный push: %s", describeResponse(outcome.status, outcome.raw))
	}
	if outcome.result.Status != "accepted-with-warnings" {
		t.Errorf("статус %q, ожидался accepted-with-warnings", outcome.result.Status)
	}
	if !hasWarning(outcome.result.Warnings, "rebase") {
		t.Error("нет предупреждения о rebase")
	}
	if !hasWarning(outcome.result.Warnings, "дубль") {
		t.Error("нет предупреждения о возможном дубле")
	}
	if len(outcome.result.Duplicates) != 1 {
		t.Errorf("плашек дубля %d, ожидалась 1", len(outcome.result.Duplicates))
	}
	// Дубль не блокирует приём: операция применена.
	if len(outcome.result.Applied) != 1 {
		t.Errorf("применено операций %d, ожидалась 1", len(outcome.result.Applied))
	}

	first.sync()
	second.sync()
	if first.state() != second.state() {
		t.Error("реплики разошлись после конфликтного пакета")
	}
	if first.chainHash == "" || first.chainHash != second.chainHash {
		t.Errorf("хеши цепочек не совпали: %q и %q", first.chainHash, second.chainHash)
	}
	if err := second.verifyChain(); err != nil {
		t.Errorf("цепочка клиента 2: %v", err)
	}
}

// TestASL_13_HashChainAndFullResync проверяет хэш-цепочку и полный ресинк (#80).
func TestASL_13_HashChainAndFullResync(t *testing.T) {
	s := newStand(t)
	first := newReplica(t, s, "телефон-1")
	second := newReplica(t, s, "телефон-2")
	first.clone()
	second.clone()

	// Пересекающиеся проводки: каждый клиент вносит свою, затем оба подтягивают.
	if outcome := first.push("коммит-1", first.expense("трата-1", money.FromKopecks(10000))); outcome.status != http.StatusOK {
		t.Fatalf("push 1: %s", describeResponse(outcome.status, outcome.raw))
	}
	first.sync()
	if outcome := second.push("коммит-2", second.income("доход-1", money.FromKopecks(500000))); outcome.status != http.StatusOK {
		t.Fatalf("push 2: %s", describeResponse(outcome.status, outcome.raw))
	}
	second.sync()
	first.sync()

	if first.state() != second.state() {
		t.Fatal("реплики не идентичны после обмена проводками")
	}
	if first.chainHash == "" || first.chainHash != second.chainHash {
		t.Fatalf("хеши цепочек не совпали: %q и %q", first.chainHash, second.chainHash)
	}
	if err := first.verifyChain(); err != nil {
		t.Fatalf("цепочка клиента 1: %v", err)
	}
	if err := second.verifyChain(); err != nil {
		t.Fatalf("цепочка клиента 2: %v", err)
	}

	t.Run("подмена ops_json обнаруживается", func(t *testing.T) {
		commits := first.history()
		if len(commits) < 2 {
			t.Fatalf("коммитов %d, ожидалось не меньше двух", len(commits))
		}
		tampered := append([]model.Commit(nil), commits...)
		tampered[len(tampered)-1].OpsJSON = `{"commit_id":"подмена","operations":[]}`
		if chainOf(tampered) == first.chainHash {
			t.Error("подмена ops_json не изменила хеш-цепочку")
		}
	})

	t.Run("расхождение вызывает полный ресинк", func(t *testing.T) {
		diverged := newReplica(t, s, "сбитый-клиент")
		diverged.clone()
		// Искусственное расхождение: клиент держит неверный хеш цепочки.
		diverged.chainHash = "сбитый-хеш-цепочки"

		pull := diverged.pull()
		if !pull.ResyncRequired {
			t.Fatal("сервер не потребовал полный ресинк")
		}
		diverged.sync()
		if diverged.state() != first.state() {
			t.Error("после полного ресинка состояние не восстановлено")
		}
		if err := diverged.verifyChain(); err != nil {
			t.Errorf("цепочка после ресинка: %v", err)
		}
	})
}

// TestASL_13_IdempotentUUIDAndImport проверяет идемпотентность uuid и импорта (#82).
func TestASL_13_IdempotentUUIDAndImport(t *testing.T) {
	s := newStand(t)
	first := newReplica(t, s, "телефон-1")
	second := newReplica(t, s, "телефон-2")
	first.clone()
	second.clone()

	operation := first.expense("трата-1", money.FromKopecks(10000))
	created := first.push("коммит-1", operation)
	if created.status != http.StatusOK {
		t.Fatalf("первый push: %s", describeResponse(created.status, created.raw))
	}
	first.sync()

	t.Run("повторная отправка коммита даёт тот же ответ", func(t *testing.T) {
		repeated := first.pushAt(0, "коммит-1", operation)
		if repeated.status != http.StatusOK {
			t.Fatalf("повторный push: %s", describeResponse(repeated.status, repeated.raw))
		}
		if repeated.result.ResultSeq != created.result.ResultSeq {
			t.Errorf("result_seq %d ≠ %d", repeated.result.ResultSeq, created.result.ResultSeq)
		}
		if strings.Join(repeated.result.Applied, ",") != strings.Join(created.result.Applied, ",") {
			t.Errorf("applied %v ≠ %v", repeated.result.Applied, created.result.Applied)
		}
	})

	t.Run("операция с тем же uuid у другого клиента пропускается", func(t *testing.T) {
		second.sync()
		// Тот же uuid, но другая сумма: запись остаётся одна — первая.
		outcome := second.push("коммит-2", second.expense("трата-1", money.FromKopecks(20000)))
		if outcome.status != http.StatusOK {
			t.Fatalf("push с повтором uuid: %s", describeResponse(outcome.status, outcome.raw))
		}
		if !hasWarning(outcome.result.Warnings, "уже применена") {
			t.Errorf("нет предупреждения о повторе uuid: %v", outcome.result.Warnings)
		}
		if len(outcome.result.Applied) != 0 {
			t.Errorf("применено операций %d, ожидалось 0", len(outcome.result.Applied))
		}
		second.sync()
		if len(second.uuids()) != 1 || second.uuids()[0] != "трата-1" {
			t.Errorf("в реплике ожидалась одна проводка «трата-1», получено %v", second.uuids())
		}
	})

	t.Run("частично пересекающийся пакет не теряет новые операции", func(t *testing.T) {
		outcome := second.push("коммит-3",
			second.expense("трата-1", money.FromKopecks(20000)),
			second.income("доход-1", money.FromKopecks(500000)))
		if outcome.status != http.StatusOK {
			t.Fatalf("пересекающийся push: %s", describeResponse(outcome.status, outcome.raw))
		}
		if !hasWarning(outcome.result.Warnings, "уже применена") {
			t.Error("нет предупреждения о повторной операции пакета")
		}
		if len(outcome.result.Applied) != 1 || outcome.result.Applied[0] != "доход-1" {
			t.Errorf("применено %v, ожидалась только «доход-1»", outcome.result.Applied)
		}
	})

	t.Run("повторный импорт hledger не создаёт дублей", func(t *testing.T) {
		book := "; домашняя книга\n2026-01-15 Пятёрочка\n    Расходы:Продукты          1 234,56 ₽\n    Активы:Наличные\n"
		body := map[string]any{
			"format": "hledger", "mode": "строгий",
			"content": book, "file_name": "книга.journal",
		}
		status, raw := s.do(t, http.MethodPost, "/api/v1/import", first.token, body)
		if status != http.StatusOK {
			t.Fatalf("импорт: %s", describeResponse(status, raw))
		}
		var firstReport struct {
			Accepted   int `json:"accepted"`
			Duplicates int `json:"duplicates"`
		}
		mustDecode(t, raw, &firstReport)
		if firstReport.Accepted != 1 {
			t.Fatalf("первый импорт принял %d проводок, ожидалась 1 (тело %s)", firstReport.Accepted, raw)
		}

		status, raw = s.do(t, http.MethodPost, "/api/v1/import", first.token, body)
		if status != http.StatusOK {
			t.Fatalf("повторный импорт: %s", describeResponse(status, raw))
		}
		var secondReport struct {
			Accepted   int `json:"accepted"`
			Duplicates int `json:"duplicates"`
		}
		mustDecode(t, raw, &secondReport)
		if secondReport.Duplicates != 1 || secondReport.Accepted != 0 {
			t.Errorf("повторный импорт: принято %d, дублей %d, ожидалось 0 и 1",
				secondReport.Accepted, secondReport.Duplicates)
		}
	})
}
