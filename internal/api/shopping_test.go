package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// TestASL_15_ShoppingAPI проверяет CRUD списков и позиций через REST.
func TestASL_15_ShoppingAPI(t *testing.T) {
	a := newTestAPI(t)

	if rec := a.do(t, http.MethodGet, "/api/v1/shopping-lists", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("без токена: код %d, ожидался 401", rec.Code)
	}

	rec := a.do(t, http.MethodPost, "/api/v1/shopping-lists", a.token, map[string]any{
		"name":         "На выходные",
		"planned_date": "2026-11-01T00:00:00Z",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("создание списка: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var list shoppingListDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("разбор списка: %v", err)
	}
	if list.ID == 0 || list.Status != model.ShoppingListActive || list.OwnerID != a.actor {
		t.Fatalf("список: %+v", list)
	}

	account := a.account(t, "Расходы:Продукты")
	rec = a.do(t, http.MethodPost, "/api/v1/shopping-lists/"+itoa(list.ID)+"/items", a.token, map[string]any{
		"name":              "Хлеб",
		"account_id":        account.ID,
		"priority":          model.ShoppingPriorityHigh,
		"planned_price_kop": 5000,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("создание позиции: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var item shoppingItemDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("разбор позиции: %v", err)
	}
	if item.Version != 1 || item.Confirmed {
		t.Fatalf("позиция: %+v", item)
	}

	// Подтверждение цены.
	rec = a.do(t, http.MethodPatch, "/api/v1/shopping-items/"+itoa(item.ID), a.token, map[string]any{
		"version":          item.Version,
		"actual_price_kop": 6200,
		"status":           model.ShoppingItemConfirmed,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("подтверждение: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var confirmed shoppingItemDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &confirmed); err != nil {
		t.Fatalf("разбор подтверждённой позиции: %v", err)
	}
	if confirmed.Version != item.Version+1 || !confirmed.Confirmed {
		t.Fatalf("подтверждённая позиция: %+v", confirmed)
	}

	// Устаревшая версия — 409.
	rec = a.do(t, http.MethodPatch, "/api/v1/shopping-items/"+itoa(item.ID), a.token, map[string]any{
		"version": item.Version,
		"comment": "устаревшая версия",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("конфликт версий: код %d, ожидался 409 (тело %s)", rec.Code, rec.Body.String())
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code != "ERR_SHOPPING_VERSION_CONFLICT" {
		t.Fatalf("конфликт версий: %+v (%v)", env, err)
	}

	// Список с позициями и итогом.
	rec = a.do(t, http.MethodGet, "/api/v1/shopping-lists/"+itoa(list.ID), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("чтение списка: код %d", rec.Code)
	}
	var got shoppingListDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("разбор списка: %v", err)
	}
	if got.ItemCount != 1 || got.ConfirmedCount != 1 || got.ConfirmedTotal != 6200 {
		t.Fatalf("агрегаты списка: %+v", got)
	}
	if len(got.Items) != 1 || !got.Items[0].Confirmed {
		t.Fatalf("позиции списка: %+v", got.Items)
	}

	// Купленную позицию нельзя удалить.
	rec = a.do(t, http.MethodPatch, "/api/v1/shopping-items/"+itoa(item.ID), a.token, map[string]any{
		"version": confirmed.Version,
		"status":  model.ShoppingItemPurchased,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("пометка purchased: код %d", rec.Code)
	}
	rec = a.do(t, http.MethodDelete, "/api/v1/shopping-items/"+itoa(item.ID), a.token, nil)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("удаление купленной позиции: код %d, ожидался 422", rec.Code)
	}

	// Архивация списка.
	rec = a.do(t, http.MethodDelete, "/api/v1/shopping-lists/"+itoa(list.ID), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("архивация: код %d, тело %s", rec.Code, rec.Body.String())
	}
	rec = a.do(t, http.MethodGet, "/api/v1/shopping-lists/"+itoa(list.ID), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("чтение архивного списка: код %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("разбор архивного списка: %v", err)
	}
	if got.Status != model.ShoppingListArchived {
		t.Fatalf("статус архивного списка = %q", got.Status)
	}
}

// TestASL_15_ShoppingAPIValidation проверяет коды ошибок REST.
func TestASL_15_ShoppingAPIValidation(t *testing.T) {
	a := newTestAPI(t)
	listRec := a.do(t, http.MethodPost, "/api/v1/shopping-lists", a.token, map[string]any{"name": "Проверка"})
	if listRec.Code != http.StatusCreated {
		t.Fatalf("создание списка: код %d", listRec.Code)
	}
	var list shoppingListDTO
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("разбор списка: %v", err)
	}

	t.Run("отрицательная цена", func(t *testing.T) {
		rec := a.do(t, http.MethodPost, "/api/v1/shopping-lists/"+itoa(list.ID)+"/items", a.token, map[string]any{
			"name": "Хлеб", "actual_price_kop": -1,
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидался 422 (тело %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("неверная статья расходов", func(t *testing.T) {
		asset := a.account(t, "Активы:Наличные")
		rec := a.do(t, http.MethodPost, "/api/v1/shopping-lists/"+itoa(list.ID)+"/items", a.token, map[string]any{
			"name": "Хлеб", "account_id": asset.ID,
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидался 422 (тело %s)", rec.Code, rec.Body.String())
		}
		var env errorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code != "ERR_SHOPPING_ACCOUNT_INVALID" {
			t.Fatalf("код ошибки: %+v (%v)", env, err)
		}
	})

	t.Run("patch без версии", func(t *testing.T) {
		rec := a.do(t, http.MethodPost, "/api/v1/shopping-lists/"+itoa(list.ID)+"/items", a.token, map[string]any{
			"name": "Молоко",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("создание позиции: код %d", rec.Code)
		}
		var item shoppingItemDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
			t.Fatalf("разбор позиции: %v", err)
		}
		rec = a.do(t, http.MethodPatch, "/api/v1/shopping-items/"+itoa(item.ID), a.token, map[string]any{
			"comment": "без версии",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("код %d, ожидался 400", rec.Code)
		}
	})
}

// TestASL_15_ShoppingSnapshotAndPull проверяет передачу списков в синхронизации.
func TestASL_15_ShoppingSnapshotAndPull(t *testing.T) {
	a := newTestAPI(t)

	listRec := a.do(t, http.MethodPost, "/api/v1/shopping-lists", a.token, map[string]any{"name": "Синхронизация"})
	if listRec.Code != http.StatusCreated {
		t.Fatalf("создание списка: код %d", listRec.Code)
	}
	var list shoppingListDTO
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("разбор списка: %v", err)
	}
	account := a.account(t, "Расходы:Продукты")
	itemRec := a.do(t, http.MethodPost, "/api/v1/shopping-lists/"+itoa(list.ID)+"/items", a.token, map[string]any{
		"name": "Хлеб", "account_id": account.ID, "actual_price_kop": 6200,
	})
	if itemRec.Code != http.StatusCreated {
		t.Fatalf("создание позиции: код %d, тело %s", itemRec.Code, itemRec.Body.String())
	}

	snapshotRec := a.do(t, http.MethodGet, "/api/v1/sync/snapshot", a.token, nil)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("снапшот: код %d", snapshotRec.Code)
	}
	var snapshot syncsvc.Snapshot
	if err := json.Unmarshal(snapshotRec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("разбор снапшота: %v", err)
	}
	if snapshot.FormatVersion != syncsvc.SnapshotFormatVersion {
		t.Fatalf("format_version = %d", snapshot.FormatVersion)
	}
	if len(snapshot.ShoppingLists) != 1 || len(snapshot.ShoppingItems) != 1 {
		t.Fatalf("списки в снапшоте: %d/%d", len(snapshot.ShoppingLists), len(snapshot.ShoppingItems))
	}
	if snapshot.ShoppingItems[0].ActualPrice.Kopecks() != 6200 {
		t.Fatalf("цена в снапшоте: %s", snapshot.ShoppingItems[0].ActualPrice)
	}

	pullRec := a.do(t, http.MethodGet, "/api/v1/sync/pull?since=0", a.token, nil)
	if pullRec.Code != http.StatusOK {
		t.Fatalf("pull: код %d", pullRec.Code)
	}
	var pull syncsvc.Pull
	if err := json.Unmarshal(pullRec.Body.Bytes(), &pull); err != nil {
		t.Fatalf("разбор pull: %v", err)
	}
	if len(pull.ShoppingLists) != 1 || len(pull.ShoppingItems) != 1 {
		t.Fatalf("списки в pull: %d/%d", len(pull.ShoppingLists), len(pull.ShoppingItems))
	}
}

// itoa переводит идентификатор в строку.
func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
