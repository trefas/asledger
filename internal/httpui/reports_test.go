package httpui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// spend подаёт трату через форму ввода и требует успешного сохранения.
func (w *testWeb) spend(t *testing.T, amount string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{
		"op_type":     {"трата"},
		"date":        {today()},
		"description": {"продукты"},
		"actor_id":    {""},
		"a_account":   {strconvID(w.accountID(t, "Расходы:Продукты"))},
		"a_amount":    {amount},
		"b_account":   {strconvID(w.accountID(t, "Активы:Наличные"))},
		"b_amount":    {amount},
	}
	return w.postForm(t, "/entry", form)
}

// earn подаёт доход на карту через форму ввода.
func (w *testWeb) earn(t *testing.T, amount string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{
		"op_type":     {"доход"},
		"date":        {today()},
		"description": {"зарплата"},
		"actor_id":    {""},
		"a_account":   {strconvID(w.accountID(t, "Активы:Карта"))},
		"a_amount":    {amount},
		"b_account":   {strconvID(w.accountID(t, "Доходы:Зарплата"))},
		"b_amount":    {amount},
	}
	return w.postForm(t, "/entry", form)
}

// TestASL_11_ReportsPage проверяет экраны отчётов и подсветку минусов.
func TestASL_11_ReportsPage(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	if rec := w.earn(t, "10 000,00"); rec.Code != http.StatusSeeOther {
		t.Fatalf("доход: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if rec := w.spend(t, "11 000,00"); rec.Code != http.StatusSeeOther {
		t.Fatalf("трата: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rec := w.get(t, "/reports")
	if rec.Code != http.StatusOK {
		t.Fatalf("отчёты: код %d, тело %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Остатки по активам", "Обороты за период", "Расходы по членам семьи",
		"Капитал (активы − обязательства)", "₽", "Расходы:Продукты", "Андрей",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице отчётов нет %q", want)
		}
	}
	// Подсветка минусов: счёт «Активы:Наличные» ушёл в минус.
	if !strings.Contains(body, "Отрицательный остаток") || !strings.Contains(body, "Активы:Наличные") {
		t.Error("нет предупреждения об отрицательном остатке")
	}
	if !strings.Contains(body, `class="negative"`) {
		t.Error("строки с минусом не подсвечены")
	}

	t.Run("минус виден в журнале", func(t *testing.T) {
		rec := w.get(t, "/")
		if rec.Code != http.StatusOK {
			t.Fatalf("журнал: код %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Отрицательный остаток") {
			t.Error("в журнале нет предупреждения о минусе")
		}
	})

	t.Run("обороты по узлу", func(t *testing.T) {
		rec := w.get(t, "/reports?node="+url.QueryEscape("Расходы"))
		if rec.Code != http.StatusOK {
			t.Fatalf("отчёты по узлу: код %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Расходы:Продукты") {
			t.Error("нет строки расхода в отчёте по узлу")
		}
	})

	t.Run("неверный период", func(t *testing.T) {
		rec := w.get(t, "/reports?from=31.02.2026")
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("неверный период: код %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); !strings.Contains(loc, "kind=error") {
			t.Errorf("нет сообщения об ошибке: %q", loc)
		}
	})
}

// TestASL_11_ReconcilePage проверяет сверку счёта из web-интерфейса.
func TestASL_11_ReconcilePage(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	if rec := w.earn(t, "10 000,00"); rec.Code != http.StatusSeeOther {
		t.Fatalf("доход: код %d, тело %s", rec.Code, rec.Body.String())
	}

	rec := w.get(t, "/reconcile")
	if rec.Code != http.StatusOK {
		t.Fatalf("сверка: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Новая сверка") || !strings.Contains(body, "Активы:Карта") {
		t.Fatal("нет формы сверки с доступными счетами")
	}

	// Сверка с расхождением и созданием корректировки.
	form := url.Values{
		"account_id": {strconvID(w.accountID(t, "Активы:Карта"))},
		"amount":     {"9 500,00"},
		"generate":   {"1"},
	}
	rec = w.postForm(t, "/reconcile", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сверка: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "kind=ok") {
		t.Fatalf("сообщение о сверке: %q", location)
	}
	if message := flashOf(t, rec); !strings.Contains(message, "Корректировка создана") {
		t.Fatalf("сообщение о сверке: %q", message)
	}

	// После корректировки расхождений нет.
	rec = w.get(t, "/reconcile")
	if rec.Code != http.StatusOK {
		t.Fatalf("сверка после корректировки: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Расхождений нет") {
		t.Error("после корректировки расхождение не обнулилось")
	}

	// Сверка без создания корректировки показывает расхождение.
	form.Set("generate", "")
	form.Set("amount", "9 000,00")
	rec = w.postForm(t, "/reconcile", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сверка без корректировки: код %d", rec.Code)
	}
	if message := flashOf(t, rec); !strings.Contains(message, "Расхождение") {
		t.Errorf("нет сообщения о расхождении: %q", message)
	}

	// Доход нельзя скорректировать сверкой.
	form.Set("account_id", strconvID(w.accountID(t, "Доходы:Зарплата")))
	form.Set("generate", "1")
	rec = w.postForm(t, "/reconcile", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сверка дохода: код %d", rec.Code)
	}
	if message := flashOf(t, rec); !strings.Contains(message, "нельзя скорректировать") {
		t.Errorf("нет отказа по типу счёта: %q", message)
	}
}

// TestASL_11_JournalFiltersAndReverse проверяет фильтры, дубли и сторно в журнале.
func TestASL_11_JournalFiltersAndReverse(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	w.spend(t, "100,00")
	// Вторая такая же трата того же дня — возможный дубль.
	rec := w.spend(t, "100,00")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("вторая трата: код %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "kind=warn") {
		t.Errorf("нет плашки возможного дубля: %q", location)
	}

	transactions, err := w.db.ListTransactions(context.Background(), store.TransactionFilter{})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 2 {
		t.Fatalf("проводок %d, ожидалось 2", len(transactions))
	}

	rec = w.get(t, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("журнал: код %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Возможный дубль") {
		t.Error("в журнале нет плашки возможного дубля")
	}
	if !strings.Contains(body, "Отменить (сторно)") {
		t.Error("в журнале нет действия сторно")
	}

	// Фильтр по типу операции.
	if rec := w.get(t, "/?op_type="+url.QueryEscape("трата")); rec.Code != http.StatusOK {
		t.Fatalf("журнал с фильтром: код %d", rec.Code)
	}
	// Некорректный тип операции — переадресация с ошибкой.
	rec = w.get(t, "/?op_type=нет")
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "kind=error") {
		t.Errorf("неверный фильтр: код %d, %q", rec.Code, rec.Header().Get("Location"))
	}

	// Сторно первой проводки.
	form := url.Values{"uuid": {transactions[1].UUID}}
	rec = w.postForm(t, "/journal/reverse", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сторно: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if message := flashOf(t, rec); !strings.Contains(message, "отменена") {
		t.Errorf("сообщение о сторно: %q", message)
	}
	rec = w.get(t, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("журнал после сторно: код %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "Сторно") || !strings.Contains(body, "Отменена") {
		t.Error("сторно не отображается в журнале")
	}

	// Сторно неизвестной проводки — сообщение об ошибке.
	rec = w.postForm(t, "/journal/reverse", url.Values{"uuid": {"нет-такой"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "kind=error") {
		t.Errorf("сторно неизвестной проводки: код %d", rec.Code)
	}
}

// flashOf извлекает сообщение flash из заголовка Location.
func flashOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	location := rec.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("разбор Location %q: %v", location, err)
	}
	return parsed.Query().Get("flash")
}
