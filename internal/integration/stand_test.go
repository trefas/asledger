// Package integration — интеграционный стенд вехи S7 (раздел 13 эпика).
//
// Стенд поднимает настоящий HTTP-сервер asledger на случайном порту поверх
// изолированной книги во временном каталоге и позволяет подключить несколько
// независимых клиентов с собственными client_id и локальными репликами.
// Все тесты пакета ходят только по localhost и детерминированы.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/trefas/asledger/internal/api"
	"github.com/trefas/asledger/pkg/core/store"
)

// testChart — минимальный план счетов стенда.
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

// Пользователь стенда.
const (
	userName   = "Андрей"
	userSecret = "секрет-андрея"
)

// stand — поднятый сервер поверх изолированной книги.
type stand struct {
	db     *store.DB
	server *httptest.Server
}

// newStand поднимает сервер, создаёт пользователя и план счетов.
//
// Ресурсы (книга и сервер) закрываются автоматически по окончании теста.
func newStand(t *testing.T) *stand {
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
	if _, err := db.CreateUser(ctx, userName, userSecret); err != nil {
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

	server := httptest.NewServer(api.New(db, api.Config{Version: "test", ChartsDir: chartsDir}).Handler())
	t.Cleanup(server.Close)
	return &stand{db: db, server: server}
}

// accountID возвращает идентификатор счёта по пути в серверной книге.
func (s *stand) accountID(t *testing.T, path string) int64 {
	t.Helper()
	account, err := s.db.AccountByPath(context.Background(), path)
	if err != nil {
		t.Fatalf("AccountByPath(%q): %v", path, err)
	}
	return account.ID
}

// pair привязывает устройство с заданным client_id и возвращает токен.
func (s *stand) pair(t *testing.T, clientID string) string {
	t.Helper()
	status, raw := s.do(t, http.MethodPost, "/api/v1/auth/pair", "", map[string]any{
		"name": userName, "secret": userSecret, "client_id": clientID,
	})
	if status != http.StatusOK {
		t.Fatalf("pair(%s): код %d, тело %s", clientID, status, raw)
	}
	var response struct {
		Token    string `json:"token"`
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("pair: разбор ответа: %v", err)
	}
	if response.Token == "" || response.ClientID != clientID {
		t.Fatalf("pair(%s): неожиданный ответ %s", clientID, raw)
	}
	return response.Token
}

// do выполняет HTTP-запрос к серверу стенда и возвращает код и тело.
//
// Ошибки транспорта считаются фатальными для теста.
func (s *stand) do(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	status, raw, err := s.try(method, path, token, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return status, raw
}

// try выполняет HTTP-запрос и возвращает код, тело и ошибку транспорта.
//
// В отличие от do не вызывает t.Fatalf и потому безопасен для горутин.
func (s *stand) try(method, path, token string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.server.URL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.server.Client().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, raw, nil
}

// mustDecode разбирает JSON-ответ или падает с понятным сообщением.
func mustDecode(t *testing.T, raw []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("разбор ответа %s: %v", raw, err)
	}
}

// describeResponse делает сообщение об ошибке информативным.
func describeResponse(status int, raw []byte) string {
	return fmt.Sprintf("код %d, тело %s", status, raw)
}
