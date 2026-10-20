package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// rolesRequest запрашивает роли для типа операции.
func rolesRequest(t *testing.T, a *testAPI, op string) *engineRolesResponse {
	t.Helper()
	rec := a.do(t, http.MethodGet, "/api/v1/engine/roles?op_type="+url.QueryEscape(op), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("roles(%q): код %d, тело %s", op, rec.Code, rec.Body.String())
	}
	var resp engineRolesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("roles(%q): разбор ответа: %v", op, err)
	}
	return &resp
}

// TestASL_10_EngineRoles проверяет, что клиент получает роли и стороны для
// каждого типа операции.
func TestASL_10_EngineRoles(t *testing.T) {
	a := newTestAPI(t)

	for _, op := range model.OpTypes() {
		resp := rolesRequest(t, a, string(op))
		if resp.OpType != op {
			t.Errorf("%s: op_type %q", op, resp.OpType)
		}
		if resp.Title == "" {
			t.Errorf("%s: пустое название операции", op)
		}
		if len(resp.Roles) == 0 {
			t.Errorf("%s: нет ролей", op)
			continue
		}
		for _, role := range resp.Roles {
			if role.Role == "" {
				t.Errorf("%s: пустая роль", op)
			}
			if len(role.AccountTypes) == 0 {
				t.Errorf("%s/%s: нет допустимых типов счетов", op, role.Role)
			}
			if role.DirectionRequired {
				if role.Side != "" {
					t.Errorf("%s/%s: сторона %q при обязательном направлении", op, role.Role, role.Side)
				}
				continue
			}
			if !role.Side.Valid() {
				t.Errorf("%s/%s: недопустимая сторона %q", op, role.Role, role.Side)
			}
		}
	}
}

// TestASL_10_EngineRolesExpense проверяет конкретные роли траты: расход —
// дебет, источник — кредит.
func TestASL_10_EngineRolesExpense(t *testing.T) {
	a := newTestAPI(t)
	resp := rolesRequest(t, a, string(model.OpExpense))

	got := map[string]model.Side{}
	for _, role := range resp.Roles {
		got[role.Role] = role.Side
	}
	if got["расход"] != model.SideDebit {
		t.Errorf("роль «расход» на стороне %q, ожидался DEBIT", got["расход"])
	}
	if got["источник"] != model.SideCredit {
		t.Errorf("роль «источник» на стороне %q, ожидался CREDIT", got["источник"])
	}
	if resp.MinDebit != 1 || resp.MinCredit != 1 {
		t.Errorf("минимумы %d/%d, ожидались 1/1", resp.MinDebit, resp.MinCredit)
	}
	if !resp.AccountsReadOnly {
		t.Error("accounts_read_only должен быть true")
	}
	for _, role := range resp.Roles {
		if role.Reserved {
			t.Errorf("роль %q траты помечена reserved", role.Role)
		}
	}
}

// TestASL_10_EngineRolesReserved проверяет признак post-MVP у операций,
// которые Android-клиент первой версии скрывает.
func TestASL_10_EngineRolesReserved(t *testing.T) {
	a := newTestAPI(t)
	resp := rolesRequest(t, a, string(model.OpDebtPayment))
	if len(resp.Roles) == 0 {
		t.Fatal("нет ролей погашения долга")
	}
	for _, role := range resp.Roles {
		if !role.Reserved {
			t.Errorf("роль %q погашения долга не помечена reserved", role.Role)
		}
	}
}

// TestASL_10_EngineRolesUnknown проверяет ошибки для неизвестного типа
// операции и отсутствия параметра.
func TestASL_10_EngineRolesUnknown(t *testing.T) {
	a := newTestAPI(t)

	rec := a.do(t, http.MethodGet, "/api/v1/engine/roles?op_type=неизвестно", a.token, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("неизвестная операция: код %d, ожидался 422", rec.Code)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if env.Error.Code != "ERR_OP_TYPE_UNKNOWN" {
		t.Fatalf("код ошибки %q", env.Error.Code)
	}

	rec = a.do(t, http.MethodGet, "/api/v1/engine/roles", a.token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("без op_type: код %d, ожидался 400", rec.Code)
	}
}

// TestASL_10_EngineRolesAuth проверяет обязательность токена.
func TestASL_10_EngineRolesAuth(t *testing.T) {
	a := newTestAPI(t)
	rec := a.do(t, http.MethodGet, "/api/v1/engine/roles?op_type=трата", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена: код %d, ожидался 401", rec.Code)
	}
}
