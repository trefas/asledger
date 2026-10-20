package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// enrollBody собирает тело запроса привязки устройства.
func enrollBody(clientID string, userID int64) map[string]any {
	body := map[string]any{"client_id": clientID, "device_name": "Телефон"}
	if userID != 0 {
		body["user_id"] = userID
	}
	return body
}

// TestASL_10_EnrollSingleUser проверяет привязку без пароля при одном
// пользователе и работоспособность выданного токена.
func TestASL_10_EnrollSingleUser(t *testing.T) {
	a := newTestAPI(t)
	a.remote = "192.168.1.20:5555"

	rec := a.do(t, http.MethodPost, "/api/v1/auth/enroll", "", enrollBody("телефон-1", 0))
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var resp enrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("enroll: разбор ответа: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("enroll: пустой токен")
	}
	if resp.UserID != a.actor {
		t.Fatalf("enroll: user_id %d, ожидался %d", resp.UserID, a.actor)
	}
	if resp.UserName != "Андрей" {
		t.Fatalf("enroll: имя %q, ожидалось «Андрей»", resp.UserName)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/sync/snapshot", resp.Token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot с токеном enroll: код %d, тело %s", rec.Code, rec.Body.String())
	}
}

// TestASL_10_EnrollMultipleUsers проверяет выбор пользователя, когда в книге
// несколько членов семьи.
func TestASL_10_EnrollMultipleUsers(t *testing.T) {
	a := newTestAPI(t)
	a.remote = "192.168.1.21:5555"
	ctx := context.Background()

	second, err := a.db.CreateUser(ctx, "Мария", "секрет-марии")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rec := a.do(t, http.MethodPost, "/api/v1/auth/enroll", "", enrollBody("телефон-2", 0))
	if rec.Code != http.StatusConflict {
		t.Fatalf("выбор пользователя: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var choice enrollChoiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &choice); err != nil {
		t.Fatalf("выбор пользователя: разбор ответа: %v", err)
	}
	if choice.Error.Code != "ERR_ENROLL_USER_REQUIRED" {
		t.Fatalf("код ошибки %q", choice.Error.Code)
	}
	if len(choice.Users) != 2 {
		t.Fatalf("пользователей %d, ожидалось 2", len(choice.Users))
	}
	if strings.Contains(rec.Body.String(), "token") {
		t.Fatal("в ответе выбора не должно быть токена")
	}

	rec = a.do(t, http.MethodPost, "/api/v1/auth/enroll", "", enrollBody("телефон-2", second.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("enroll с user_id: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var resp enrollResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("enroll с user_id: разбор ответа: %v", err)
	}
	if resp.UserID != second.ID || resp.UserName != "Мария" {
		t.Fatalf("привязан пользователь %d/%q, ожидался %d/«Мария»",
			resp.UserID, resp.UserName, second.ID)
	}
}

// TestASL_10_EnrollUnknownUser проверяет отказ при неизвестном user_id.
func TestASL_10_EnrollUnknownUser(t *testing.T) {
	a := newTestAPI(t)
	a.remote = "192.168.1.22:5555"

	rec := a.do(t, http.MethodPost, "/api/v1/auth/enroll", "", enrollBody("телефон-3", 9999))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("код %d, ожидался 422: %s", rec.Code, rec.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if env.Error.Code != "ERR_ENROLL_USER_UNKNOWN" {
		t.Fatalf("код ошибки %q", env.Error.Code)
	}
}

// TestASL_10_EnrollDisabled проверяет запрет привязки настройкой книги.
func TestASL_10_EnrollDisabled(t *testing.T) {
	a := newTestAPI(t)
	a.remote = "192.168.1.23:5555"
	if err := a.db.SetSetting(context.Background(), store.SettingDevicesEnrollment, store.EnrollmentDeny); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}

	rec := a.do(t, http.MethodPost, "/api/v1/auth/enroll", "", enrollBody("телефон-4", 0))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("код %d, ожидался 403: %s", rec.Code, rec.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if env.Error.Code != "ERR_ENROLL_DISABLED" {
		t.Fatalf("код ошибки %q", env.Error.Code)
	}
}

// TestASL_10_EnrollPublicNetwork проверяет, что привязка с публичного адреса
// отклоняется.
func TestASL_10_EnrollPublicNetwork(t *testing.T) {
	a := newTestAPI(t)
	a.remote = "203.0.113.7:4444"

	rec := a.do(t, http.MethodPost, "/api/v1/auth/enroll", "", enrollBody("телефон-5", 0))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("код %d, ожидался 403: %s", rec.Code, rec.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if env.Error.Code != "ERR_ENROLL_FORBIDDEN" {
		t.Fatalf("код ошибки %q", env.Error.Code)
	}
}

// TestASL_10_EnrollNoUsers проверяет подсказку создать пользователя в пустой
// книге.
func TestASL_10_EnrollNoUsers(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "book.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	server := New(db, Config{Version: "test"})

	body := strings.NewReader(`{"client_id":"телефон-6"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enroll", body)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.24:5555"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("код %d, ожидался 409: %s", rec.Code, rec.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if env.Error.Code != "ERR_ENROLL_NO_USERS" {
		t.Fatalf("код ошибки %q", env.Error.Code)
	}
}
