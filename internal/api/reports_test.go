package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
)

// urlQuery кодирует значение параметра строки запроса (кириллица, «:»).
func urlQuery(value string) string { return url.QueryEscape(value) }

// incomeBody собирает онлайн-проводку дохода на карту.
func (a *testAPI) incomeBody(t *testing.T, uuid string, amount int64) map[string]any {
	t.Helper()
	return map[string]any{
		"op_type": model.OpIncome,
		"uuid":    uuid,
		"date":    now(),
		"entries": []map[string]any{
			{"account_id": a.account(t, "Активы:Карта").ID, "amount_kop": amount, "role": "поступление"},
			{"account_id": a.account(t, "Доходы:Зарплата").ID, "amount_kop": amount, "role": "доход"},
		},
	}
}

// postOK отправляет проводку и требует код 201.
func (a *testAPI) postOK(t *testing.T, path string, body any) {
	t.Helper()
	rec := a.do(t, http.MethodPost, path, a.token, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST %s: код %d, тело %s", path, rec.Code, rec.Body.String())
	}
}

// TestASL_10_ReportsBalancesAPI проверяет отчёт об остатках через API.
func TestASL_10_ReportsBalancesAPI(t *testing.T) {
	a := newTestAPI(t)
	a.postOK(t, "/api/v1/transactions", a.incomeBody(t, "inc-1", 100000))
	a.postOK(t, "/api/v1/transactions", a.expenseBody(t, "exp-1", 30000))

	rec := a.do(t, http.MethodGet, "/api/v1/reports/balances", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("balances: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var dto balancesReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if dto.AssetsKop != 70000 || dto.NetWorthKop != 70000 {
		t.Errorf("итоги: активы %d, капитал %d, ожидалось 70000", dto.AssetsKop, dto.NetWorthKop)
	}
	if len(dto.Negative) != 1 || dto.Negative[0] != "Активы:Наличные" {
		t.Errorf("подсветка минусов: %v", dto.Negative)
	}
	var found bool
	for _, row := range dto.Rows {
		if row.Path == "Активы:Карта" {
			found = true
			if row.TotalKop != 100000 || row.Total == "" || row.Type != model.TypeAsset {
				t.Errorf("строка карты: %+v", row)
			}
		}
	}
	if !found {
		t.Error("в отчёте нет строки «Активы:Карта»")
	}

	// Без токена отчёт недоступен.
	if rec := a.do(t, http.MethodGet, "/api/v1/reports/balances", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("balances без токена: код %d", rec.Code)
	}
}

// TestASL_10_ReportsTurnoversAPI проверяет обороты по узлу через API.
func TestASL_10_ReportsTurnoversAPI(t *testing.T) {
	a := newTestAPI(t)
	a.postOK(t, "/api/v1/transactions", a.expenseBody(t, "exp-1", 25000))

	rec := a.do(t, http.MethodGet, "/api/v1/reports/turnovers?node="+urlQuery("Расходы"), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("turnovers: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var dto turnoversReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	var debit int64
	for _, row := range dto.Rows {
		if row.Path != "Расходы" && len(row.Path) >= len("Расходы:") && row.Path[:len("Расходы:")] != "Расходы:" {
			t.Errorf("в отчёт попал чужой счёт %q", row.Path)
		}
		debit += int64(row.DebitKop)
	}
	if debit != 25000 {
		t.Errorf("оборот по дебету поддерева расходов: %d, ожидалось 25000", debit)
	}

	// Период: фильтр по будущей дате даёт пустые обороты.
	tomorrow := time.Now().UTC().AddDate(0, 0, 2).Format(time.DateOnly)
	rec = a.do(t, http.MethodGet, "/api/v1/reports/turnovers?from="+tomorrow, a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("turnovers(период): код %d", rec.Code)
	}
	var empty turnoversReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	for _, row := range empty.Rows {
		if row.DebitKop != 0 || row.CreditKop != 0 {
			t.Errorf("в будущем периоде есть движение: %+v", row)
		}
	}

	// Некорректный период — ошибка запроса.
	if rec := a.do(t, http.MethodGet, "/api/v1/reports/turnovers?from=31.02.2026", a.token, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("неверный период: код %d", rec.Code)
	}
}

// TestASL_10_ReportsExpensesAPI проверяет расходы по членам семьи через API.
func TestASL_10_ReportsExpensesAPI(t *testing.T) {
	a := newTestAPI(t)
	a.postOK(t, "/api/v1/transactions", a.expenseBody(t, "exp-1", 25000))
	a.postOK(t, "/api/v1/transactions", a.expenseBody(t, "exp-2", 10000))

	rec := a.do(t, http.MethodGet, "/api/v1/reports/expenses", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expenses: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var dto expensesReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if dto.TotalKop != 35000 {
		t.Errorf("итог расходов: %d, ожидалось 35000", dto.TotalKop)
	}
	if len(dto.Actors) != 1 || dto.Actors[0].Name != "Андрей" {
		t.Fatalf("авторы: %+v", dto.Actors)
	}
	if len(dto.Actors[0].Articles) != 1 || dto.Actors[0].Articles[0].Path != "Расходы:Продукты" {
		t.Errorf("статьи: %+v", dto.Actors[0].Articles)
	}

	// Период вне проводок — пусто.
	tomorrow := time.Now().UTC().AddDate(0, 0, 2).Format(time.DateOnly)
	rec = a.do(t, http.MethodGet, "/api/v1/reports/expenses?from="+tomorrow, a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expenses(период): код %d", rec.Code)
	}
	var empty expensesReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if len(empty.Actors) != 0 || empty.TotalKop != 0 {
		t.Errorf("будущий период не пуст: %+v", empty)
	}
}

// TestASL_10_ReportsJournalAPI проверяет журнал с фильтрами и страницами.
func TestASL_10_ReportsJournalAPI(t *testing.T) {
	a := newTestAPI(t)
	a.postOK(t, "/api/v1/transactions", a.expenseBody(t, "exp-1", 25000))
	a.postOK(t, "/api/v1/transactions", a.expenseBody(t, "exp-2", 10000))
	a.postOK(t, "/api/v1/transactions", a.incomeBody(t, "inc-1", 500000))

	rec := a.do(t, http.MethodGet, "/api/v1/reports/journal?per_page=2", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("journal: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var dto journalReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if dto.Total != 3 || dto.PerPage != 2 || len(dto.Rows) != 2 || dto.Page != 1 {
		t.Fatalf("страница 1: %+v", dto)
	}
	if dto.Rows[0].ActorName == "" || dto.Rows[0].Lines[0].Path == "" {
		t.Errorf("журнал без имён автора или счетов: %+v", dto.Rows[0])
	}
	if dto.Rows[0].Date == "" || dto.Rows[0].Amount == "" {
		t.Errorf("журнал без даты или суммы: %+v", dto.Rows[0])
	}

	rec = a.do(t, http.MethodGet, "/api/v1/reports/journal?per_page=2&page=2", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("journal(стр. 2): код %d", rec.Code)
	}
	var page2 journalReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &page2); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if page2.Page != 2 || len(page2.Rows) != 1 {
		t.Errorf("страница 2: %+v", page2)
	}

	// Фильтр по поддереву расходов.
	rec = a.do(t, http.MethodGet, "/api/v1/reports/journal?account="+urlQuery("Расходы"), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("journal(счёт): код %d", rec.Code)
	}
	var byAccount journalReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &byAccount); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if byAccount.Total != 2 {
		t.Errorf("проводок по расходам: %d, ожидалось 2", byAccount.Total)
	}

	// Фильтр по типу операции.
	rec = a.do(t, http.MethodGet, "/api/v1/reports/journal?op_type="+urlQuery(string(model.OpIncome)), a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("journal(тип): код %d", rec.Code)
	}
	var byType journalReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &byType); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if byType.Total != 1 || byType.Rows[0].OpType != model.OpIncome {
		t.Errorf("проводки дохода: %+v", byType)
	}

	// Некорректный тип операции — ошибка запроса.
	if rec := a.do(t, http.MethodGet, "/api/v1/reports/journal?op_type=нет", a.token, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("неверный тип операции: код %d", rec.Code)
	}
	// Некорректная нижняя граница суммы — ошибка запроса.
	if rec := a.do(t, http.MethodGet, "/api/v1/reports/journal?amount_from=abc", a.token, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("неверная сумма: код %d", rec.Code)
	}
}

// TestASL_10_ReconcileAPI проверяет сверку и корректировку через API.
func TestASL_10_ReconcileAPI(t *testing.T) {
	a := newTestAPI(t)
	a.postOK(t, "/api/v1/transactions", a.incomeBody(t, "inc-1", 100000))

	card := a.account(t, "Активы:Карта")
	rec := a.do(t, http.MethodPost, "/api/v1/reconcile", a.token, map[string]any{
		"account_id": card.ID, "fact_kop": 90000, "generate": true, "uuid": "adj-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("reconcile: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var result reconcileResultDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if result.DiffKop != -10000 {
		t.Errorf("расхождение: %d, ожидалось -10000", result.DiffKop)
	}
	if result.Path != "Активы:Карта" || result.Transaction == nil {
		t.Fatalf("результат: %+v", result)
	}
	if result.Transaction.OpType != model.OpAdjustment {
		t.Errorf("тип корректировки: %s", result.Transaction.OpType)
	}

	// После корректировки расхождения нет.
	rec = a.do(t, http.MethodGet, "/api/v1/reports/discrepancies", a.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("discrepancies: код %d", rec.Code)
	}
	var discrepancies struct {
		Rows []discrepancyDTO `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &discrepancies); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if len(discrepancies.Rows) != 1 || discrepancies.Rows[0].DiffKop != 0 {
		t.Errorf("расхождения после корректировки: %+v", discrepancies.Rows)
	}

	// Сверка дохода со снятием расхождения невозможна.
	income := a.account(t, "Доходы:Зарплата")
	rec = a.do(t, http.MethodPost, "/api/v1/reconcile", a.token, map[string]any{
		"account_id": income.ID, "fact_kop": 0, "generate": true,
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("сверка дохода: код %d, ожидался 422", rec.Code)
	}

	// Неизвестный счёт.
	rec = a.do(t, http.MethodPost, "/api/v1/reconcile", a.token, map[string]any{
		"account_id": 999999, "fact_kop": 0,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("сверка неизвестного счёта: код %d", rec.Code)
	}

	// Сверка требует токена.
	if rec := a.do(t, http.MethodPost, "/api/v1/reconcile", "", map[string]any{"account_id": card.ID}); rec.Code != http.StatusUnauthorized {
		t.Errorf("сверка без токена: код %d", rec.Code)
	}
}
