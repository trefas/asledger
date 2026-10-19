package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// head возвращает текущий head_seq книги.
func (a *testAPI) head(t *testing.T) int64 {
	t.Helper()
	head, err := a.db.HeadSeq(context.Background())
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	return head
}

// transactionCount возвращает число проводок в книге.
func (a *testAPI) transactionCount(t *testing.T) int {
	t.Helper()
	list, err := a.db.ListTransactions(context.Background(), store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	return len(list)
}

// pushBody собирает тело запроса push.
func (a *testAPI) pushBody(t *testing.T, commitID string, baseSeq int64, ops ...map[string]any) map[string]any {
	t.Helper()
	operations := make([]map[string]any, 0, len(ops))
	operations = append(operations, ops...)
	return map[string]any{"commit_id": commitID, "base_seq": baseSeq, "operations": operations}
}

// pushResult разбирает ответ push.
func pushResult(t *testing.T, rec *httptest.ResponseRecorder) syncsvc.PushResult {
	t.Helper()
	var result syncsvc.PushResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("разбор ответа push: %v", err)
	}
	return result
}

// TestASL_10_Snapshot проверяет снапшот книги.
func TestASL_10_Snapshot(t *testing.T) {
	a := newTestAPI(t)
	if rec := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, a.expenseBody(t, "snap-1", 10000)); rec.Code != http.StatusCreated {
		t.Fatalf("проводка: код %d", rec.Code)
	}

	rec := a.do(t, http.MethodGet, "/api/v1/sync/snapshot", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("снапшот: код %d", rec.Code)
	}
	var snapshot syncsvc.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("разбор снапшота: %v", err)
	}
	if snapshot.FormatVersion != syncsvc.SnapshotFormatVersion {
		t.Fatalf("версия формата = %d", snapshot.FormatVersion)
	}
	if snapshot.HeadSeq == 0 || len(snapshot.Transactions) != 1 || len(snapshot.Accounts) == 0 {
		t.Fatalf("снапшот неполон: head=%d, tx=%d, acc=%d",
			snapshot.HeadSeq, len(snapshot.Transactions), len(snapshot.Accounts))
	}
	if len(snapshot.Transactions[0].Lines) == 0 {
		t.Fatal("в снапшоте нет строк проводки")
	}
	if len(snapshot.Users) == 0 || snapshot.Users[0].Name != "Андрей" {
		t.Fatalf("пользователи в снапшоте: %+v", snapshot.Users)
	}
	if bytesContains(rec.Body.Bytes(), "\"token_hash\"") || bytesContains(rec.Body.Bytes(), a.token) {
		t.Fatal("в снапшоте обнаружен секрет")
	}
}

// TestASL_10_PushFastForward проверяет fast-forward и идемпотентность.
func TestASL_10_PushFastForward(t *testing.T) {
	a := newTestAPI(t)

	body := a.pushBody(t, "commit-1", a.head(t),
		a.expenseBody(t, "push-1", 10000),
		a.expenseBody(t, "push-2", 20000),
	)
	rec := a.do(t, http.MethodPost, "/api/v1/sync/push", a.token, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("push: код %d, тело %s", rec.Code, rec.Body.String())
	}
	result := pushResult(t, rec)
	if result.Status != syncsvc.StatusAccepted {
		t.Fatalf("статус = %q, ожидался accepted (warnings: %v)", result.Status, result.Warnings)
	}
	if len(result.Applied) != 2 {
		t.Fatalf("применено %d операций, ожидалось 2", len(result.Applied))
	}
	if result.ResultSeq != a.head(t) {
		t.Fatalf("result_seq = %d, head = %d", result.ResultSeq, a.head(t))
	}

	t.Run("повтор даёт тот же статус", func(t *testing.T) {
		repeat := a.do(t, http.MethodPost, "/api/v1/sync/push", a.token, body)
		again := pushResult(t, repeat)
		if again.Status != result.Status || again.ResultSeq != result.ResultSeq {
			t.Fatalf("повтор: статус %q, result_seq %d", again.Status, again.ResultSeq)
		}
		if a.transactionCount(t) != 2 {
			t.Fatalf("повтор создал дубли: проводок %d", a.transactionCount(t))
		}
	})
}

// TestASL_10_PushAtomicity проверяет, что невалидная операция откатывает пакет.
func TestASL_10_PushAtomicity(t *testing.T) {
	a := newTestAPI(t)
	before := a.transactionCount(t)
	headBefore := a.head(t)

	invalid := a.expenseBody(t, "atomic-2", 10000)
	invalid["entries"] = []map[string]any{
		{"account_id": a.account(t, "Расходы:Продукты").ID, "amount_kop": 10000, "role": "расход"},
		{"account_id": a.account(t, "Доходы:Зарплата").ID, "amount_kop": 10000, "role": "источник"},
	}

	body := a.pushBody(t, "commit-atomic", a.head(t),
		a.expenseBody(t, "atomic-1", 5000),
		invalid,
		a.expenseBody(t, "atomic-3", 7000),
	)
	rec := a.do(t, http.MethodPost, "/api/v1/sync/push", a.token, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("код %d, ожидался 422", rec.Code)
	}
	var rejected pushRejected
	if err := json.Unmarshal(rec.Body.Bytes(), &rejected); err != nil {
		t.Fatalf("разбор отказа: %v", err)
	}
	if rejected.Status != syncsvc.StatusRejected || rejected.Error.Code == "" {
		t.Fatalf("ответ отказа неполон: %+v", rejected)
	}
	if a.transactionCount(t) != before || a.head(t) != headBefore {
		t.Fatalf("пакет не откатился: проводок %d, head %d", a.transactionCount(t), a.head(t))
	}
}

// TestASL_10_PushRebase проверяет rebase при отставшем base_seq.
func TestASL_10_PushRebase(t *testing.T) {
	a := newTestAPI(t)
	// Создаём коммит, чтобы голова ушла вперёд.
	if rec := a.do(t, http.MethodPost, "/api/v1/sync/push", a.token,
		a.pushBody(t, "commit-first", a.head(t), a.expenseBody(t, "rebase-1", 1000))); rec.Code != http.StatusOK {
		t.Fatalf("первый push: код %d", rec.Code)
	}

	rec := a.do(t, http.MethodPost, "/api/v1/sync/push", a.token,
		a.pushBody(t, "commit-rebase", 0, a.expenseBody(t, "rebase-2", 2000)))
	if rec.Code != http.StatusOK {
		t.Fatalf("rebase: код %d, тело %s", rec.Code, rec.Body.String())
	}
	result := pushResult(t, rec)
	if result.Status != syncsvc.StatusAcceptedWithWarnings {
		t.Fatalf("статус = %q, ожидался accepted-with-warnings", result.Status)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("rebase не выставил предупреждений")
	}
}

// TestASL_10_Pull проверяет дельту журнала изменений.
func TestASL_10_Pull(t *testing.T) {
	a := newTestAPI(t)
	if rec := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, a.expenseBody(t, "pull-1", 10000)); rec.Code != http.StatusCreated {
		t.Fatalf("проводка: код %d", rec.Code)
	}

	rec := a.do(t, http.MethodGet, "/api/v1/sync/pull?since=0", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pull: код %d", rec.Code)
	}
	var pull syncsvc.Pull
	if err := json.Unmarshal(rec.Body.Bytes(), &pull); err != nil {
		t.Fatalf("разбор дельты: %v", err)
	}
	if pull.HeadSeq == 0 || len(pull.Changes) == 0 || len(pull.Transactions) == 0 {
		t.Fatalf("дельта неполна: head=%d, changes=%d, tx=%d",
			pull.HeadSeq, len(pull.Changes), len(pull.Transactions))
	}
	if !pull.AccountsReadOnly || len(pull.Accounts) == 0 {
		t.Fatal("план счетов должен приходить целиком и read-only")
	}
	if pull.Commits[0].Hash == "" {
		t.Fatal("в дельте нет звена хэш-цепочки")
	}
}

// TestASL_10_Suggest проверяет подсказки счетов.
func TestASL_10_Suggest(t *testing.T) {
	a := newTestAPI(t)

	rec := a.do(t, http.MethodGet, "/api/v1/accounts/suggest?q=зарп&op_type=доход", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("suggest: код %d", rec.Code)
	}
	var payload struct {
		Items []accountDTO `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("разбор подсказок: %v", err)
	}
	if len(payload.Items) == 0 || payload.Items[0].FullPath != "Доходы:Зарплата" || !payload.Items[0].Usable {
		t.Fatalf("подсказка не найдена: %+v", payload.Items)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/accounts/suggest?q=Актив&op_type=трата", a.token, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("разбор подсказок: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatal("по запросу «Актив» ничего не найдено")
	}
	for _, item := range payload.Items {
		if !item.IsLeaf && item.Usable {
			t.Fatalf("папка помечена пригодной: %+v", item)
		}
	}
	if !payload.Items[0].IsLeaf {
		t.Fatalf("листовые счета должны идти первыми: %+v", payload.Items[0])
	}
}

// TestASL_10_TransactionSource проверяет источник проводки из API.
func TestASL_10_TransactionSource(t *testing.T) {
	a := newTestAPI(t)
	if rec := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, a.expenseBody(t, "src-1", 1000)); rec.Code != http.StatusCreated {
		t.Fatalf("код %d", rec.Code)
	}
	tr, err := a.db.TransactionByUUID(context.Background(), "src-1")
	if err != nil {
		t.Fatalf("TransactionByUUID: %v", err)
	}
	if tr.Source != "api:"+a.clientID {
		t.Fatalf("источник = %q, ожидался api:%s", tr.Source, a.clientID)
	}
	if tr.CommitID == 0 {
		t.Fatal("онлайн-проводка не привязана к коммиту")
	}
}

// bytesContains сообщает, содержит ли байтовый срез подстроку.
func bytesContains(data []byte, sub string) bool {
	return bytes.Contains(data, []byte(sub))
}
