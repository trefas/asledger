package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// replica — независимый клиент синхронизации: собственный токен устройства и
// локальная реплика книги (счета и проводки), собранная снапшотом и дельтами.
type replica struct {
	t        *testing.T
	stand    *stand
	clientID string
	token    string

	headSeq   int64
	chainHash string

	accounts     map[int64]model.Account
	byPath       map[string]int64
	transactions map[string]model.Transaction
}

// newReplica подключает устройство к серверу стенда.
func newReplica(t *testing.T, s *stand, clientID string) *replica {
	t.Helper()
	return &replica{
		t:            t,
		stand:        s,
		clientID:     clientID,
		token:        s.pair(t, clientID),
		accounts:     map[int64]model.Account{},
		byPath:       map[string]int64{},
		transactions: map[string]model.Transaction{},
	}
}

// clone получает полный снапшот книги и заменяет локальную реплику.
func (r *replica) clone() {
	r.t.Helper()
	status, raw := r.stand.do(r.t, http.MethodGet, "/api/v1/sync/snapshot", r.token, nil)
	if status != http.StatusOK {
		r.t.Fatalf("snapshot: %s", describeResponse(status, raw))
	}
	var snapshot syncsvc.Snapshot
	mustDecode(r.t, raw, &snapshot)

	r.accounts = make(map[int64]model.Account, len(snapshot.Accounts))
	r.byPath = make(map[string]int64, len(snapshot.Accounts))
	for _, account := range snapshot.Accounts {
		r.accounts[account.ID] = account
		r.byPath[account.FullPath] = account.ID
	}
	r.transactions = make(map[string]model.Transaction, len(snapshot.Transactions))
	for _, tr := range snapshot.Transactions {
		r.transactions[tr.UUID] = tr
	}
	r.headSeq = snapshot.HeadSeq
	r.chainHash = snapshot.ChainHash
}

// pull забирает дельту от текущего состояния реплики.
//
// При обнаружении расхождения (ResyncRequired) реплика не изменяется: клиент
// обязан выполнить полный ресинк со снапшота (см. sync).
func (r *replica) pull() syncsvc.Pull {
	r.t.Helper()
	path := fmt.Sprintf("/api/v1/sync/pull?since=%d", r.headSeq)
	if r.chainHash != "" {
		path += "&hash=" + r.chainHash
	}
	status, raw := r.stand.do(r.t, http.MethodGet, path, r.token, nil)
	if status != http.StatusOK {
		r.t.Fatalf("pull: %s", describeResponse(status, raw))
	}
	var pull syncsvc.Pull
	mustDecode(r.t, raw, &pull)
	if !pull.ResyncRequired {
		r.applyPull(pull)
	}
	return pull
}

// applyPull вливает дельту в локальную реплику.
func (r *replica) applyPull(pull syncsvc.Pull) {
	for _, tr := range pull.Transactions {
		r.transactions[tr.UUID] = tr
	}
	for _, account := range pull.Accounts {
		r.accounts[account.ID] = account
		r.byPath[account.FullPath] = account.ID
	}
	r.headSeq = pull.HeadSeq
	r.chainHash = pull.ChainHash
}

// sync подтягивает дельту, а при расхождении выполняет полный ресинк.
func (r *replica) sync() syncsvc.Pull {
	r.t.Helper()
	pull := r.pull()
	if pull.ResyncRequired {
		r.clone()
	}
	return pull
}

// history возвращает всю историю коммитов сервера, не изменяя реплику.
func (r *replica) history() []model.Commit {
	r.t.Helper()
	status, raw := r.stand.do(r.t, http.MethodGet, "/api/v1/sync/pull?since=0", r.token, nil)
	if status != http.StatusOK {
		r.t.Fatalf("pull(история): %s", describeResponse(status, raw))
	}
	var pull syncsvc.Pull
	mustDecode(r.t, raw, &pull)
	return pull.Commits
}

// verifyChain пересчитывает хэш-цепочку по истории и сравнивает с состоянием
// реплики — так клиент доказывает идентичность своей копии книги серверу.
func (r *replica) verifyChain() error {
	hash := chainOf(r.history())
	if hash != r.chainHash {
		return fmt.Errorf("клиент %s: локальный хеш %q, серверный %q", r.clientID, hash, r.chainHash)
	}
	return nil
}

// chainOf сворачивает звенья хэш-цепочки.
func chainOf(commits []model.Commit) string {
	hash := ""
	for _, commit := range commits {
		hash = syncsvc.ChainHash(hash, commit.OpsJSON)
	}
	return hash
}

// pushOutcome — результат отправки пакета.
type pushOutcome struct {
	status int
	result syncsvc.PushResult
	raw    []byte
}

// push отправляет коммит с текущим base_seq реплики.
func (r *replica) push(commitID string, ops ...syncsvc.Operation) pushOutcome {
	r.t.Helper()
	outcome, err := r.tryPush(r.headSeq, commitID, ops...)
	if err != nil {
		r.t.Fatalf("push: %v", err)
	}
	return outcome
}

// pushAt отправляет коммит с явно заданным base_seq (сценарий отставшего клиента).
func (r *replica) pushAt(baseSeq int64, commitID string, ops ...syncsvc.Operation) pushOutcome {
	r.t.Helper()
	outcome, err := r.tryPush(baseSeq, commitID, ops...)
	if err != nil {
		r.t.Fatalf("push: %v", err)
	}
	return outcome
}

// tryPush отправляет пакет и не вызывает t.Fatalf — безопасно для горутин.
func (r *replica) tryPush(baseSeq int64, commitID string, ops ...syncsvc.Operation) (pushOutcome, error) {
	status, raw, err := r.stand.try(http.MethodPost, "/api/v1/sync/push", r.token, map[string]any{
		"base_seq":   baseSeq,
		"commit_id":  commitID,
		"operations": ops,
	})
	if err != nil {
		return pushOutcome{}, err
	}
	var result syncsvc.PushResult
	if err := json.Unmarshal(raw, &result); err != nil && status == http.StatusOK {
		return pushOutcome{}, fmt.Errorf("разбор ответа push: %w", err)
	}
	return pushOutcome{status: status, result: result, raw: raw}, nil
}

// account возвращает счёт реплики по пути.
func (r *replica) account(path string) int64 {
	id, ok := r.byPath[path]
	if !ok {
		r.t.Fatalf("в реплике %s нет счёта %q", r.clientID, path)
	}
	return id
}

// expense собирает операцию траты из общего счёта-источника.
func (r *replica) expense(uuid string, amount money.Kop) syncsvc.Operation {
	return syncsvc.Operation{
		OpType:      model.OpExpense,
		UUID:        uuid,
		Date:        time.Now().UTC(),
		Description: "продукты",
		Entries: []syncsvc.Entry{
			{AccountID: r.account("Расходы:Продукты"), Amount: amount, Role: string(engine.RoleExpense)},
			{AccountID: r.account("Активы:Наличные"), Amount: amount, Role: string(engine.RoleSource)},
		},
	}
}

// income собирает операцию дохода на карту.
func (r *replica) income(uuid string, amount money.Kop) syncsvc.Operation {
	return syncsvc.Operation{
		OpType:      model.OpIncome,
		UUID:        uuid,
		Date:        time.Now().UTC(),
		Description: "зарплата",
		Entries: []syncsvc.Entry{
			{AccountID: r.account("Активы:Карта"), Amount: amount, Role: string(engine.RoleReceipt)},
			{AccountID: r.account("Доходы:Зарплата"), Amount: amount, Role: string(engine.RoleIncome)},
		},
	}
}

// uuids возвращает отсортированные uuid проводок реплики.
func (r *replica) uuids() []string {
	out := make([]string, 0, len(r.transactions))
	for uuid := range r.transactions {
		out = append(out, uuid)
	}
	sort.Strings(out)
	return out
}

// state описывает содержимое реплики строкой для сравнения копий.
func (r *replica) state() string {
	uuids := r.uuids()
	parts := make([]string, 0, len(uuids))
	for _, uuid := range uuids {
		tr := r.transactions[uuid]
		lines := make([]string, 0, len(tr.Lines))
		for _, line := range tr.Lines {
			lines = append(lines, fmt.Sprintf("%d:%s:%d", line.AccountID, line.Side, line.Amount.Kopecks()))
		}
		sort.Strings(lines)
		parts = append(parts, fmt.Sprintf("%s|%s|%d|%s",
			tr.UUID, tr.OpType, tr.Date.UTC().Unix(), strings.Join(lines, ",")))
	}
	return strings.Join(parts, "\n")
}

// balances считает остатки реплики по природе типов счетов — так же, как сервер.
func (r *replica) balances() map[int64]money.Kop {
	debit := map[int64]money.Kop{}
	credit := map[int64]money.Kop{}
	for _, tr := range r.transactions {
		for _, line := range tr.Lines {
			if line.Side == model.SideDebit {
				debit[line.AccountID] += line.Amount
			} else {
				credit[line.AccountID] += line.Amount
			}
		}
	}
	out := make(map[int64]money.Kop, len(r.accounts))
	for id, account := range r.accounts {
		if account.Type == model.TypeAsset || account.Type == model.TypeExpense {
			out[id] = debit[id] - credit[id]
		} else {
			out[id] = credit[id] - debit[id]
		}
	}
	return out
}

// balance возвращает остаток счёта реплики по пути.
func (r *replica) balance(path string) money.Kop {
	return r.balances()[r.account(path)]
}
