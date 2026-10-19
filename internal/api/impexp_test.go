package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// importJournal — книга hledger для проверки импорта через API.
const importJournal = `2026-01-15 Пятёрочка
    Расходы:Продукты        1 234,56
    Активы:Наличные

2026-01-16 Проценты по вкладу
    Активы:Вклад              500,00
    Доходы:Проценты
`

// TestASL_8_ImportHledgerAPI проверяет импорт книги hledger через REST API.
func TestASL_8_ImportHledgerAPI(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "mode": "гибкий", "file_name": "book.journal",
		"content": importJournal,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("импорт: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var report importReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("разбор отчёта: %v", err)
	}
	if report.Format != formatHledger || report.Mode != string(hledgerFlexible) {
		t.Errorf("отчёт: формат %q, режим %q", report.Format, report.Mode)
	}
	if report.Accepted != 2 || report.Skipped != 0 || report.Rejected {
		t.Fatalf("отчёт: принято %d, пропущено %d, отклонён %v, проблемы %v",
			report.Accepted, report.Skipped, report.Rejected, report.Problems)
	}
	want := []string{"Активы:Вклад", "Доходы:Проценты"}
	if strings.Join(report.CreatedAccounts, ",") != strings.Join(want, ",") {
		t.Errorf("созданные счета %v, ожидались %v", report.CreatedAccounts, want)
	}
	if api.account(t, "Активы:Вклад").Type != "актив" {
		t.Error("счёт вклада создан без унаследованного типа")
	}

	// Повторный импорт идемпотентен и не создаёт проводок.
	rec = api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "content": importJournal,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("повторный импорт: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("разбор отчёта: %v", err)
	}
	if report.Accepted != 0 || report.Duplicates != 2 {
		t.Errorf("повторный импорт: принято %d, дублей %d", report.Accepted, report.Duplicates)
	}
}

// hledgerFlexible — строковое значение гибкого режима импорта.
const hledgerFlexible = "гибкий"

// TestASL_8_ImportHledgerStrictAPI проверяет строгий режим и отчёт с проблемами.
func TestASL_8_ImportHledgerStrictAPI(t *testing.T) {
	api := newTestAPI(t)

	journal := "2026-02-01 Покупка\n    Недвижимость:Квартира   100,00\n    Активы:Наличные\n"
	rec := api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "mode": "строгий", "content": journal,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("импорт: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var report importReportDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("разбор отчёта: %v", err)
	}
	if !report.Rejected || report.Accepted != 0 {
		t.Fatalf("строгий режим: отклонён %v, принято %d", report.Rejected, report.Accepted)
	}
	found := false
	for _, problem := range report.Problems {
		if strings.Contains(problem.Message, "Недвижимость") {
			found = true
		}
	}
	if !found {
		t.Errorf("в отчёте нет неизвестного счёта: %v", report.Problems)
	}

	// Неизвестный режим — ошибка запроса.
	rec = api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "mode": "как-нибудь", "content": journal,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("неизвестный режим: код %d", rec.Code)
	}

	// Неизвестный формат — ошибка запроса.
	rec = api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "xml", "content": "",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("неизвестный формат: код %d", rec.Code)
	}
}

// TestASL_8_ExportHledgerAPI проверяет выгрузку книги файлом hledger.
func TestASL_8_ExportHledgerAPI(t *testing.T) {
	api := newTestAPI(t)
	if rec := api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "content": importJournal,
	}); rec.Code != http.StatusOK {
		t.Fatalf("подготовка: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rec := api.do(t, http.MethodGet, "/api/v1/export?format=hledger", api.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("выгрузка: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("тип содержимого %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "asledger.journal") {
		t.Errorf("заголовок вложения %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"; asledger", "account Активы:Наличные", "account Расходы:Продукты", "; тип: расход",
		"2026-01-15 Пятёрочка", "-1234.56",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("в выгрузке нет %q:\n%s", want, body)
		}
	}
}

// TestASL_8_ExportCSVAPI проверяет выгрузку CSV журнала и отчётов.
func TestASL_8_ExportCSVAPI(t *testing.T) {
	api := newTestAPI(t)
	if rec := api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "content": importJournal,
	}); rec.Code != http.StatusOK {
		t.Fatalf("подготовка: код %d, тело %s", rec.Code, rec.Body.String())
	}

	t.Run("журнал", func(t *testing.T) {
		rec := api.do(t, http.MethodGet, "/api/v1/export?format=csv", api.token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "asledger-journal.csv") {
			t.Errorf("заголовок вложения %q", got)
		}
		body := rec.Body.String()
		if !strings.HasPrefix(body, "\ufeff") {
			t.Error("CSV отдан без метки UTF-8")
		}
		if !strings.Contains(body, "проводка;uuid;дата;тип операции") {
			t.Errorf("нет заголовка журнала:\n%s", body)
		}
		if !strings.Contains(body, "Расходы:Продукты;DEBIT;1234,56") {
			t.Errorf("нет строки проводки:\n%s", body)
		}
	})

	t.Run("остатки", func(t *testing.T) {
		rec := api.do(t, http.MethodGet, "/api/v1/export?format=csv&report=balances", api.token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Активы:Наличные;актив;-1234,56") {
			t.Errorf("нет строки остатков:\n%s", body)
		}
	})

	t.Run("обороты", func(t *testing.T) {
		rawURL := "/api/v1/export?format=csv&report=turnover&from=2026-01-01&to=2026-01-31"
		rec := api.do(t, http.MethodGet, rawURL, api.token, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "счёт;тип;остаток на начало;дебет;кредит;остаток на конец") {
			t.Errorf("нет заголовка оборотов:\n%s", body)
		}
		if !strings.Contains(body, "Активы:Вклад;актив;0,00;500,00;0,00;500,00") {
			t.Errorf("нет строки оборотов:\n%s", body)
		}
	})

	t.Run("ошибки периода и отчёта", func(t *testing.T) {
		rec := api.do(t, http.MethodGet, "/api/v1/export?format=csv&report=turnover&from=вчера", api.token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("неверный период: код %d", rec.Code)
		}
		rec = api.do(t, http.MethodGet, "/api/v1/export?format=csv&report=нечто", api.token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("неизвестный отчёт: код %d", rec.Code)
		}
		rec = api.do(t, http.MethodGet, "/api/v1/export?format=xml", api.token, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("неизвестный формат: код %d", rec.Code)
		}
	})
}

// TestASL_8_ExportJSONSnapshotAPI проверяет полноту JSON-снапшота.
func TestASL_8_ExportJSONSnapshotAPI(t *testing.T) {
	api := newTestAPI(t)
	if rec := api.do(t, http.MethodPost, "/api/v1/import", api.token, map[string]any{
		"format": "hledger", "content": importJournal,
	}); rec.Code != http.StatusOK {
		t.Fatalf("подготовка: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rec := api.do(t, http.MethodGet, "/api/v1/export?format=json", api.token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("выгрузка: код %d, тело %s", rec.Code, rec.Body.String())
	}
	var snapshot struct {
		FormatVersion int               `json:"format_version"`
		HeadSeq       int64             `json:"head_seq"`
		ChainHash     string            `json:"chain_hash"`
		Accounts      []json.RawMessage `json:"accounts"`
		Users         []json.RawMessage `json:"users"`
		Transactions  []json.RawMessage `json:"transactions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("разбор снапшота: %v", err)
	}
	if snapshot.FormatVersion == 0 || snapshot.HeadSeq == 0 || snapshot.ChainHash == "" {
		t.Errorf("снапшот без версии, головы или звена цепочки: %+v", snapshot)
	}
	if len(snapshot.Transactions) != 2 {
		t.Errorf("проводок в снапшоте %d, ожидалось 2", len(snapshot.Transactions))
	}
	if len(snapshot.Accounts) == 0 || len(snapshot.Users) == 0 {
		t.Error("снапшот без плана счетов или пользователей")
	}
	// Снапшот не содержит секретов: ни токенов устройств, ни хешей паролей.
	body := rec.Body.String()
	for _, secret := range []string{api.token, "token_hash", "secret_hash", "секрет-андрея"} {
		if strings.Contains(body, secret) {
			t.Errorf("в снапшоте найден секрет %q", secret)
		}
	}
}

// TestASL_8_ImportExportRequireToken проверяет обязательность токена.
func TestASL_8_ImportExportRequireToken(t *testing.T) {
	api := newTestAPI(t)
	paths := []string{
		"/api/v1/import",
		"/api/v1/export?format=" + url.QueryEscape("hledger"),
		"/api/v1/export?format=csv",
	}
	for _, path := range paths {
		method := http.MethodGet
		if strings.Contains(path, "import") {
			method = http.MethodPost
		}
		rec := api.do(t, method, path, "", map[string]any{"format": "hledger"})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s без токена: код %d", method, path, rec.Code)
		}
	}
}
