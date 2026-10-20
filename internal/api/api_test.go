package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// testChart — минимальный план счетов для тестов API.
const testChart = `{"name":"Тест","accounts":[
  {"name":"Активы","type":"актив","children":[
    {"name":"Наличные","type":"актив"},
    {"name":"Карта","type":"актив"}]},
  {"name":"Обязательства","type":"пассив","children":[
    {"name":"Кредит","type":"пассив"}]},
  {"name":"Расходы","type":"расход","children":[
    {"name":"Продукты","type":"расход"}]},
  {"name":"Доходы","type":"доход","children":[
    {"name":"Зарплата","type":"доход"}]}
]}`

// testAPI — сервер API поверх временной книги и привязанное устройство.
type testAPI struct {
	db       *store.DB
	handler  http.Handler
	token    string
	clientID string
	actor    int64
	// remote — адрес клиента для проверки локальной сети; пусто — умолчание.
	remote string
}

// newTestAPI поднимает сервер, создаёт пользователя, план счетов и устройство.
func newTestAPI(t *testing.T) *testAPI {
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
	if _, err := db.CreateUser(ctx, "Андрей", "секрет-андрея"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	chart, err := store.ParseChart([]byte(testChart))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}
	if _, err := db.ImportChart(ctx, chart); err != nil {
		t.Fatalf("ImportChart: %v", err)
	}

	chartsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(chartsDir, "test.json"), []byte(testChart), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	server := New(db, Config{Version: "test", ChartsDir: chartsDir})
	api := &testAPI{db: db, handler: server.Handler()}

	rec := api.do(t, http.MethodPost, "/api/v1/auth/pair", "", map[string]any{
		"name": "Андрей", "secret": "секрет-андрея", "client_id": "устройство-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("pair: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var pr pairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &pr); err != nil {
		t.Fatalf("pair: разбор ответа: %v", err)
	}
	api.token = pr.Token
	api.clientID = pr.ClientID
	api.actor = pr.UserID
	return api
}

// do выполняет запрос к серверу и возвращает запись ответа.
func (a *testAPI) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if a.remote != "" {
		req.RemoteAddr = a.remote
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

// account возвращает счёт по пути.
func (a *testAPI) account(t *testing.T, path string) *model.Account {
	t.Helper()
	acc, err := a.db.AccountByPath(context.Background(), path)
	if err != nil {
		t.Fatalf("AccountByPath(%q): %v", path, err)
	}
	return acc
}

// now возвращает текущую дату в формате RFC3339.
func now() string { return time.Now().UTC().Format(time.RFC3339) }

// expenseBody собирает онлайн-проводку траты.
func (a *testAPI) expenseBody(t *testing.T, uuid string, amount int64) map[string]any {
	t.Helper()
	return map[string]any{
		"op_type": model.OpExpense,
		"uuid":    uuid,
		"date":    now(),
		"entries": []map[string]any{
			{"account_id": a.account(t, "Расходы:Продукты").ID, "amount_kop": amount, "role": "расход"},
			{"account_id": a.account(t, "Активы:Наличные").ID, "amount_kop": amount, "role": "источник"},
		},
	}
}

// TestASL_10_HealthAndRouting проверяет базовую маршрутизацию и JSON-ошибки.
func TestASL_10_HealthAndRouting(t *testing.T) {
	a := newTestAPI(t)

	rec := a.do(t, http.MethodGet, "/api/v1/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("health: код %d", rec.Code)
	}
	var health struct {
		Status        string `json:"status"`
		Version       string `json:"version"`
		FormatVersion int    `json:"format_version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("health: не JSON: %v", err)
	}
	if health.Status != "ok" || health.FormatVersion == 0 {
		t.Fatalf("health: %+v", health)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/нет-такого", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("неизвестный маршрут: код %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("404: Content-Type = %q", ct)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code == "" {
		t.Fatalf("404: не JSON-конверт: %s", rec.Body.String())
	}

	rec = a.do(t, http.MethodGet, "/api/v1/transactions", "", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("неверный метод: код %d", rec.Code)
	}
}

// TestASL_10_AuthPair проверяет привязку устройства и защиту маршрутов.
func TestASL_10_AuthPair(t *testing.T) {
	a := newTestAPI(t)

	t.Run("неверный секрет", func(t *testing.T) {
		rec := a.do(t, http.MethodPost, "/api/v1/auth/pair", "", map[string]any{
			"name": "Андрей", "secret": "неверный", "client_id": "чужой",
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("код %d, ожидался 401", rec.Code)
		}
	})

	t.Run("без токена", func(t *testing.T) {
		if rec := a.do(t, http.MethodGet, "/api/v1/sync/snapshot", "", nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("код %d, ожидался 401", rec.Code)
		}
	})

	t.Run("неверный токен", func(t *testing.T) {
		if rec := a.do(t, http.MethodGet, "/api/v1/sync/snapshot", "мусор", nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("код %d, ожидался 401", rec.Code)
		}
	})

	t.Run("токен не хранится в открытом виде", func(t *testing.T) {
		devices, err := a.db.ListDevices(context.Background())
		if err != nil {
			t.Fatalf("ListDevices: %v", err)
		}
		for _, device := range devices {
			if device.TokenHash == a.token {
				t.Fatal("открытый токен сохранён в книге")
			}
		}
	})

	t.Run("рабочий токен и отзыв", func(t *testing.T) {
		if rec := a.do(t, http.MethodGet, "/api/v1/sync/snapshot", a.token, nil); rec.Code != http.StatusOK {
			t.Fatalf("снапшот по токену: код %d", rec.Code)
		}
		if rec := a.do(t, http.MethodDelete, "/api/v1/auth/token", a.token, nil); rec.Code != http.StatusOK {
			t.Fatalf("отзыв: код %d", rec.Code)
		}
		if rec := a.do(t, http.MethodGet, "/api/v1/sync/snapshot", a.token, nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("после отзыва: код %d, ожидался 401", rec.Code)
		}
	})
}

// TestASL_10_Transactions проверяет онлайн-проводку через движок.
func TestASL_10_Transactions(t *testing.T) {
	a := newTestAPI(t)

	t.Run("проводка принимается", func(t *testing.T) {
		rec := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, a.expenseBody(t, "api-tx-1", 10000))
		if rec.Code != http.StatusCreated {
			t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
		}
		var dto transactionDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
			t.Fatalf("разбор ответа: %v", err)
		}
		if dto.OpType != model.OpExpense || len(dto.Lines) != 2 {
			t.Fatalf("ответ неполон: %+v", dto)
		}
		for _, line := range dto.Lines {
			if !line.Side.Valid() {
				t.Fatalf("сторона не назначена: %+v", line)
			}
		}
	})

	t.Run("идемпотентность uuid", func(t *testing.T) {
		first := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, a.expenseBody(t, "api-tx-2", 5000))
		second := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, a.expenseBody(t, "api-tx-2", 5000))
		var one, two transactionDTO
		_ = json.Unmarshal(first.Body.Bytes(), &one)
		_ = json.Unmarshal(second.Body.Bytes(), &two)
		if one.ID != two.ID {
			t.Fatalf("повтор создал проводку %d вместо %d", two.ID, one.ID)
		}
	})

	t.Run("невалидная операция даёт код и не меняет книгу", func(t *testing.T) {
		before, _ := a.db.ListTransactions(context.Background(), store.TransactionFilter{})
		body := a.expenseBody(t, "api-tx-3", 10000)
		body["entries"] = []map[string]any{
			{"account_id": a.account(t, "Расходы:Продукты").ID, "amount_kop": 10000, "role": "расход"},
			{"account_id": a.account(t, "Доходы:Зарплата").ID, "amount_kop": 10000, "role": "источник"},
		}
		rec := a.do(t, http.MethodPost, "/api/v1/transactions", a.token, body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидался 422", rec.Code)
		}
		var env errorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code == "" {
			t.Fatalf("нет кода валидации: %s", rec.Body.String())
		}
		after, _ := a.db.ListTransactions(context.Background(), store.TransactionFilter{})
		if len(after) != len(before) {
			t.Fatalf("книга изменилась при отказе: было %d, стало %d", len(before), len(after))
		}
	})
}
