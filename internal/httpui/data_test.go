package httpui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// TestASL_11_DataImportAndExport проверяет страницу импорта и выгрузки книги.
func TestASL_11_DataImportAndExport(t *testing.T) {
	web := newTestWeb(t)
	web.seedUserChart(t)

	// Страница доступна и содержит формы импорта и выгрузки.
	rec := web.get(t, "/data")
	if rec.Code != http.StatusOK {
		t.Fatalf("страница данных: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Импорт книги hledger", "/data/import", "/data/export/hledger", "/data/export/csv"} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице данных нет %q", want)
		}
	}

	// Импорт книги: счета уже есть, поэтому проводки проходят в строгом режиме.
	journal := "2026-01-15 Пятёрочка\n    Расходы:Продукты        1 234,56\n    Активы:Наличные\n"
	rec = web.postForm(t, "/data/import", url.Values{
		"mode": {"гибкий"}, "content": {journal},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("импорт: код %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"Отчёт об импорте", "Принято проводок", "Ошибок разбора и отклонённых проводок нет"} {
		if !strings.Contains(body, want) {
			t.Errorf("в отчёте нет %q:\n%s", want, body)
		}
	}

	ctx := context.Background()
	transactions, err := web.db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("проводок в книге %d, ожидалась 1", len(transactions))
	}

	// Проблемная книга: отчёт перечисляет причину пропуска.
	bad := "2026-02-01 Плохо\n    Расходы:Продукты        100,00\n    Активы:Наличные          50,00\n"
	rec = web.postForm(t, "/data/import", url.Values{"mode": {"гибкий"}, "content": {bad}})
	if rec.Code != http.StatusOK {
		t.Fatalf("импорт проблемной книги: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ERR_UNBALANCED") {
		t.Errorf("в отчёте нет причины ERR_UNBALANCED:\n%s", rec.Body.String())
	}

	// Выгрузка hledger файлом.
	rec = web.get(t, "/data/export/hledger")
	if rec.Code != http.StatusOK {
		t.Fatalf("выгрузка hledger: код %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "asledger.journal") {
		t.Errorf("заголовок вложения %q", got)
	}
	if !strings.Contains(rec.Body.String(), "2026-01-15 Пятёрочка") {
		t.Errorf("выгрузка hledger без проводки:\n%s", rec.Body.String())
	}

	// Выгрузка CSV журнала и отчёта об остатках.
	rec = web.get(t, "/data/export/csv")
	if rec.Code != http.StatusOK {
		t.Fatalf("выгрузка CSV: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Расходы:Продукты;DEBIT;1234,56") {
		t.Errorf("журнал CSV неполон:\n%s", rec.Body.String())
	}
	rec = web.get(t, "/data/export/csv?report=balances")
	if rec.Code != http.StatusOK {
		t.Fatalf("выгрузка остатков: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "остаток") {
		t.Errorf("отчёт об остатках неполон:\n%s", rec.Body.String())
	}
	rec = web.get(t, "/data/export/csv?report=turnover&from=2026-01-01&to=2026-01-31")
	if rec.Code != http.StatusOK {
		t.Fatalf("выгрузка оборотов: код %d", rec.Code)
	}

	// Ошибки: неверный отчёт и неверная дата.
	if rec := web.get(t, "/data/export/csv?report=нечто"); rec.Code != http.StatusInternalServerError {
		t.Errorf("неизвестный отчёт: код %d", rec.Code)
	}
	if rec := web.get(t, "/data/export/csv?report=turnover&from=вчера"); rec.Code != http.StatusInternalServerError {
		t.Errorf("неверная дата: код %d", rec.Code)
	}
	if rec := web.get(t, "/data/export/xml"); rec.Code != http.StatusNotFound {
		t.Errorf("неизвестный формат: код %d", rec.Code)
	}
}

// TestASL_11_DataImportStrictUnknownAccount проверяет отчёт строгого режима.
func TestASL_11_DataImportStrictUnknownAccount(t *testing.T) {
	web := newTestWeb(t)
	web.seedUserChart(t)

	journal := "2026-03-01 Покупка\n    Недвижимость:Квартира    100,00\n    Активы:Наличные\n"
	rec := web.postForm(t, "/data/import", url.Values{"mode": {"строгий"}, "content": {journal}})
	if rec.Code != http.StatusOK {
		t.Fatalf("импорт: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "импорт отклонён целиком") {
		t.Errorf("нет сообщения об отклонении импорта:\n%s", body)
	}
	if !strings.Contains(body, "Недвижимость") {
		t.Errorf("в отчёте нет неизвестного счёта:\n%s", body)
	}
	if web.userAccounts(t) != 8 {
		t.Errorf("строгий режим изменил план счетов: счетов %d", web.userAccounts(t))
	}
}

// TestASL_11_DataImportWithoutContent проверяет пустую форму импорта.
func TestASL_11_DataImportWithoutContent(t *testing.T) {
	web := newTestWeb(t)
	web.seedUserChart(t)

	rec := web.postForm(t, "/data/import", url.Values{"mode": {"гибкий"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("пустой импорт: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "вставьте книгу hledger") {
		t.Errorf("нет сообщения о пустой книге:\n%s", rec.Body.String())
	}

	rec = web.postForm(t, "/data/import", url.Values{"mode": {"как-нибудь"}, "content": {"x"}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "неизвестный режим") {
		t.Errorf("неизвестный режим: код %d, тело %s", rec.Code, rec.Body.String())
	}
}
