package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestASL_10_ChartsTemplates проверяет список шаблонных планов счетов.
func TestASL_10_ChartsTemplates(t *testing.T) {
	a := newTestAPI(t)
	rec := a.do(t, http.MethodGet, "/api/v1/charts/templates", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("templates: код %d", rec.Code)
	}
	var payload struct {
		Templates []templateDTO `json:"templates"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("разбор шаблонов: %v", err)
	}
	if len(payload.Templates) != 1 {
		t.Fatalf("шаблонов %d, ожидался 1", len(payload.Templates))
	}
	if payload.Templates[0].Name != "Тест" || payload.Templates[0].Accounts == 0 {
		t.Fatalf("шаблон неполон: %+v", payload.Templates[0])
	}
}

// TestASL_10_ImportExport проверяет каркас импорта и экспорта.
func TestASL_10_ImportExport(t *testing.T) {
	a := newTestAPI(t)

	t.Run("импорт плана счетов из JSON", func(t *testing.T) {
		chart := `{"name":"Доп","accounts":[{"name":"Вложения","type":"актив","children":[{"name":"Ценные бумаги","type":"актив"}]}]}`
		rec := a.do(t, http.MethodPost, "/api/v1/import", a.token, map[string]any{
			"format": formatChartJSON, "content": chart,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("import: код %d, тело %s", rec.Code, rec.Body.String())
		}
		var report struct {
			Created int `json:"created"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil || report.Created == 0 {
			t.Fatalf("отчёт импорта: %s", rec.Body.String())
		}
	})

	t.Run("hledger отклоняется в строгом режиме без проводок", func(t *testing.T) {
		rec := a.do(t, http.MethodPost, "/api/v1/import", a.token, map[string]any{
			"format": "hledger", "mode": "строгий", "content": "2026-01-01 Тест\n",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("неизвестный формат", func(t *testing.T) {
		rec := a.do(t, http.MethodPost, "/api/v1/import", a.token, map[string]any{
			"format": "загадка", "content": "x",
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("код %d, ожидался 400", rec.Code)
		}
	})

	t.Run("экспорт JSON, CSV и hledger", func(t *testing.T) {
		if rec := a.do(t, http.MethodGet, "/api/v1/export?format=json", a.token, nil); rec.Code != http.StatusOK {
			t.Fatalf("json: код %d", rec.Code)
		}
		for _, path := range []string{
			"/api/v1/export?format=csv",
			"/api/v1/export?format=hledger",
		} {
			rec := a.do(t, http.MethodGet, path, a.token, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: код %d, тело %s", path, rec.Code, rec.Body.String())
			}
			if rec.Body.Len() == 0 {
				t.Fatalf("%s: пустое тело выгрузки", path)
			}
		}
		rec := a.do(t, http.MethodGet, "/api/v1/export?format=загадка", a.token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("неизвестный формат: код %d, ожидался 400", rec.Code)
		}
	})
}
