package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestASL_9_TransactionDuplicatesAndReverseAPI проверяет плашку дубля и сторно.
func TestASL_9_TransactionDuplicatesAndReverseAPI(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(t, http.MethodPost, "/api/v1/transactions", api.token, api.expenseBody(t, "dup-1", 15000))
	if rec.Code != http.StatusCreated {
		t.Fatalf("первая проводка: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var first transactionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if len(first.Duplicates) != 0 {
		t.Errorf("первая проводка помечена дублем: %+v", first.Duplicates)
	}

	// Вторая такая же проводка принимается, но помечается возможным дублем.
	rec = api.do(t, http.MethodPost, "/api/v1/transactions", api.token, api.expenseBody(t, "dup-2", 15000))
	if rec.Code != http.StatusCreated {
		t.Fatalf("вторая проводка: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var second transactionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if len(second.Duplicates) != 1 || second.Duplicates[0].UUID != "dup-2" {
		t.Fatalf("плашка дубля: %+v", second.Duplicates)
	}
	if len(second.Duplicates[0].Existing) != 1 || second.Duplicates[0].Existing[0] != "dup-1" {
		t.Errorf("существующие проводки в плашке: %+v", second.Duplicates[0])
	}

	// Отмена помеченной проводки — сторно с противоположными сторонами.
	rec = api.do(t, http.MethodPost, "/api/v1/transactions/dup-1/reverse", api.token, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("сторно: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var reversal transactionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &reversal); err != nil {
		t.Fatalf("разбор сторно: %v", err)
	}
	if reversal.ReversesID == nil || *reversal.ReversesID != first.ID {
		t.Errorf("reverses_id сторно: %+v", reversal.ReversesID)
	}
	if len(reversal.Lines) != len(first.Lines) || reversal.Lines[0].Side == first.Lines[0].Side {
		t.Errorf("сторно не инвертирует стороны: %+v", reversal.Lines)
	}

	// Повторная отмена идемпотентна: то же сторно, без второй инверсии.
	rec = api.do(t, http.MethodPost, "/api/v1/transactions/dup-1/reverse", api.token, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("повторное сторно: код %d", rec.Code)
	}
	var again transactionDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatalf("разбор повторного сторно: %v", err)
	}
	if again.UUID != reversal.UUID {
		t.Errorf("повторное сторно создало новую проводку: %s != %s", again.UUID, reversal.UUID)
	}

	// Сторно требует токена.
	if rec := api.do(t, http.MethodPost, "/api/v1/transactions/dup-2/reverse", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("сторно без токена: код %d", rec.Code)
	}
}
