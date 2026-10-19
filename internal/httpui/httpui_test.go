package httpui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// testChart — минимальный план счетов для тестов web-интерфейса.
const testChart = `{"name":"Тест","accounts":[
  {"name":"Активы","type":"актив","children":[
    {"name":"Наличные","type":"актив","min_balance":0},
    {"name":"Карта","type":"актив"}]},
  {"name":"Обязательства","type":"пассив","children":[
    {"name":"Кредит","type":"пассив"}]},
  {"name":"Расходы","type":"расход","children":[
    {"name":"Продукты","type":"расход"}]},
  {"name":"Доходы","type":"доход","children":[
    {"name":"Зарплата","type":"доход"}]}
]}`

// testWeb — стенд web-интерфейса поверх временной книги.
type testWeb struct {
	db      *store.DB
	handler http.Handler
	root    string
}

// newTestWeb поднимает web-интерфейс с пустой книгой.
func newTestWeb(t *testing.T) *testWeb {
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

	chartsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(chartsDir, "test.json"), []byte(testChart), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	server, err := New(Config{
		Version:   "test",
		AssetsDir: assetsDir(t),
		ChartsDir: chartsDir,
		DB:        db,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &testWeb{db: db, handler: server.Handler(), root: chartsDir}
}

// assetsDir возвращает путь к единому каталогу бренд-ассетов.
func assetsDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "assets", "logo")
	if _, err := os.Stat(filepath.Join(dir, "favicon.ico")); err != nil {
		t.Fatalf("нет каталога ассетов: %v", err)
	}
	return dir
}

// seedUserChart создаёт пользователя и план счетов.
func (w *testWeb) seedUserChart(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := w.db.CreateUser(ctx, "Андрей", "секрет-андрея"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	chart, err := store.ParseChart([]byte(testChart))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}
	if _, err := w.db.ImportChart(ctx, chart); err != nil {
		t.Fatalf("ImportChart: %v", err)
	}
}

// get выполняет GET-запрос.
func (w *testWeb) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	w.handler.ServeHTTP(rec, req)
	return rec
}

// postForm выполняет POST формы.
func (w *testWeb) postForm(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	w.handler.ServeHTTP(rec, req)
	return rec
}

// accountID возвращает идентификатор счёта по пути.
func (w *testWeb) accountID(t *testing.T, path string) int64 {
	t.Helper()
	acc, err := w.db.AccountByPath(context.Background(), path)
	if err != nil {
		t.Fatalf("AccountByPath(%q): %v", path, err)
	}
	return acc.ID
}

// userAccounts считает счета плана, введённые пользователем.
func (w *testWeb) userAccounts(t *testing.T) int {
	t.Helper()
	accounts, err := w.db.ListAccounts(context.Background(), store.AccountFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	return countUserAccounts(accounts)
}

// userID возвращает идентификатор первого пользователя.
func (w *testWeb) userID(t *testing.T) int64 {
	t.Helper()
	users, err := w.db.ListUsers(context.Background())
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers: %v (%d)", err, len(users))
	}
	return users[0].ID
}

// TestASL_11_WebJournalAndBrand проверяет каркас web-интерфейса.
func TestASL_11_WebJournalAndBrand(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	rec := w.get(t, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("журнал: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"asledger", "Журнал", "₽", "/assets/asledger-logo.svg", "/favicon.ico", "/static/style.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице журнала нет %q", want)
		}
	}
	if !strings.Contains(body, `lang="ru"`) {
		t.Error("страница не помечена как русскоязычная")
	}

	t.Run("favicon и манифест", func(t *testing.T) {
		if rec := w.get(t, "/favicon.ico"); rec.Code != http.StatusOK {
			t.Fatalf("favicon.ico: код %d", rec.Code)
		}
		if rec := w.get(t, "/manifest.webmanifest"); rec.Code != http.StatusOK {
			t.Fatalf("manifest: код %d", rec.Code)
		}
	})
	t.Run("статика и ассеты", func(t *testing.T) {
		if rec := w.get(t, "/static/style.css"); rec.Code != http.StatusOK {
			t.Fatalf("style.css: код %d", rec.Code)
		}
		if rec := w.get(t, "/static/app.js"); rec.Code != http.StatusOK {
			t.Fatalf("app.js: код %d", rec.Code)
		}
		if rec := w.get(t, "/assets/icon-192.png"); rec.Code != http.StatusOK {
			t.Fatalf("icon-192.png: код %d", rec.Code)
		}
	})
}

// TestASL_11_BrandAssetsSet проверяет комплект favicon и PNG из единого каталога.
func TestASL_11_BrandAssetsSet(t *testing.T) {
	dir := assetsDir(t)
	for _, name := range []string{
		"favicon.ico", "favicon-16.png", "favicon-32.png", "favicon-48.png",
		"apple-touch-icon.png", "icon-192.png", "icon-512.png",
		"asledger-logo.svg", "manifest.webmanifest",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("нет файла ассетов %s: %v", name, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, "manifest.webmanifest"))
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	var manifest struct {
		Name  string `json:"name"`
		Lang  string `json:"lang"`
		Icons []struct {
			Src   string `json:"src"`
			Sizes string `json:"sizes"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if manifest.Name != "asledger" || manifest.Lang != "ru" {
		t.Fatalf("manifest: name=%q lang=%q", manifest.Name, manifest.Lang)
	}
	if len(manifest.Icons) == 0 {
		t.Fatal("manifest без иконок")
	}
	for _, icon := range manifest.Icons {
		name := filepath.Base(icon.Src)
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("иконка манифеста %s отсутствует в каталоге ассетов", icon.Src)
		}
	}
}

// TestASL_11_SetupFromTemplate проверяет первичную настройку из шаблона.
func TestASL_11_SetupFromTemplate(t *testing.T) {
	w := newTestWeb(t)

	rec := w.get(t, "/setup")
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Начать с шаблона") {
		t.Fatal("на экране настройки нет варианта «Начать с шаблона»")
	}

	form := url.Values{
		"template":    {"test.json"},
		"user_name":   {"Андрей"},
		"user_secret": {"секрет-андрея"},
	}
	rec = w.postForm(t, "/setup/template", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("импорт шаблона: код %d, тело %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/") {
		t.Fatalf("переадресация не в журнал: %q", loc)
	}

	users, err := w.db.ListUsers(context.Background())
	if err != nil || len(users) != 1 {
		t.Fatalf("ListUsers: %v, пользователей %d", err, len(users))
	}
	if got := w.userAccounts(t); got == 0 {
		t.Fatal("план счетов не создан")
	}

	t.Run("экран ввода доступен", func(t *testing.T) {
		rec := w.get(t, "/entry")
		if rec.Code != http.StatusOK {
			t.Fatalf("entry: код %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Сохранить проводку") {
			t.Fatal("нет формы ввода проводки")
		}
	})
}

// TestASL_11_SetupImportJSON проверяет импорт JSON и отсутствие частичного дерева.
func TestASL_11_SetupImportJSON(t *testing.T) {
	w := newTestWeb(t)

	t.Run("неверный JSON не оставляет дерева", func(t *testing.T) {
		form := url.Values{
			"content":     {`{"name":"Тест","accounts":[{"name":"Активы","type":"актив"},{"name":"Активы","type":"актив"}]}`},
			"user_name":   {"Андрей"},
			"user_secret": {"секрет-андрея"},
		}
		rec := w.postForm(t, "/setup/import", form)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("код %d, ожидался 422", rec.Code)
		}
		if got := w.userAccounts(t); got != 0 {
			t.Fatalf("частичное дерево: счетов пользователя %d", got)
		}
	})

	t.Run("верный JSON импортируется", func(t *testing.T) {
		form := url.Values{
			"content":     {testChart},
			"user_name":   {"Андрей"},
			"user_secret": {"секрет-андрея"},
		}
		rec := w.postForm(t, "/setup/import", form)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
		}
		if got := w.userAccounts(t); got == 0 {
			t.Fatal("план счетов не создан")
		}
	})
}

// TestASL_11_EntryCreatesTransaction проверяет ввод проводки и связь с актором.
func TestASL_11_EntryCreatesTransaction(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	actor := w.userID(t)

	form := url.Values{
		"op_type":     {"трата"},
		"date":        {today()},
		"description": {"продукты"},
		"actor_id":    {""},
		"a_account":   {strconvID(w.accountID(t, "Расходы:Продукты"))},
		"a_amount":    {"1 234,56"},
		"b_account":   {strconvID(w.accountID(t, "Активы:Наличные"))},
		"b_amount":    {"1234.56"},
	}
	rec := w.postForm(t, "/entry", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("проводка: код %d, тело %s", rec.Code, rec.Body.String())
	}

	transactions, err := w.db.ListTransactions(context.Background(), store.TransactionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("проводок %d, ожидалась 1", len(transactions))
	}
	tr := transactions[0]
	if tr.ActorID != actor {
		t.Fatalf("актор проводки %d, ожидался %d", tr.ActorID, actor)
	}
	if tr.Source != "web" {
		t.Fatalf("источник %q, ожидался web", tr.Source)
	}
	if len(tr.Lines) != 2 {
		t.Fatalf("строк %d, ожидалось 2", len(tr.Lines))
	}
	if tr.Lines[0].Amount != 123456 {
		t.Fatalf("сумма %d, ожидалось 123456", tr.Lines[0].Amount)
	}

	t.Run("проводка видна в журнале", func(t *testing.T) {
		rec := w.get(t, "/")
		if rec.Code != http.StatusOK {
			t.Fatalf("журнал: код %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "Расходы:Продукты") || !strings.Contains(body, "1 234,56 ₽") {
			t.Fatal("проводка не отображается в журнале")
		}
	})
}

// TestASL_11_EntryMergesSources проверяет несколько источников и повтор счёта.
func TestASL_11_EntryMergesSources(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	products := strconvID(w.accountID(t, "Расходы:Продукты"))
	cash := strconvID(w.accountID(t, "Активы:Наличные"))
	card := strconvID(w.accountID(t, "Активы:Карта"))

	form := url.Values{
		"op_type":   {"трата"},
		"date":      {today()},
		"a_account": {products},
		"a_amount":  {"1 000,00"},
		"b_account": {cash, card, cash},
		"b_amount":  {"300,00", "200,00", "500,00"},
	}
	rec := w.postForm(t, "/entry", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("код %d, тело %s", rec.Code, rec.Body.String())
	}

	transactions, err := w.db.ListTransactions(context.Background(), store.TransactionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("проводок %d, ожидалась 1", len(transactions))
	}
	tr := transactions[0]
	if len(tr.Lines) != 3 {
		t.Fatalf("строк %d, ожидалось 3 (одна статья и два источника)", len(tr.Lines))
	}
	// Строки одного счёта одной стороны слиты: 300 + 500 = 800, плюс карта 200.
	var creditTotal int64
	creditAmounts := map[int64]bool{}
	for _, line := range tr.Lines {
		if line.Side == model.SideCredit {
			creditTotal += int64(line.Amount)
			creditAmounts[int64(line.Amount)] = true
		}
	}
	if creditTotal != 100000 {
		t.Fatalf("сумма источников %d, ожидалось 100000", creditTotal)
	}
	if !creditAmounts[80000] || !creditAmounts[20000] {
		t.Fatalf("строки источников не объединены: %v", creditAmounts)
	}
}

// TestASL_11_EntryRejectsWrongType проверяет отказ при неверном типе счёта.
func TestASL_11_EntryRejectsWrongType(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	form := url.Values{
		"op_type":   {"трата"},
		"date":      {today()},
		"a_account": {strconvID(w.accountID(t, "Расходы:Продукты"))},
		"a_amount":  {"100,00"},
		"b_account": {strconvID(w.accountID(t, "Расходы:Продукты"))},
		"b_amount":  {"100,00"},
	}
	rec := w.postForm(t, "/entry", form)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("код %d, ожидался 422", rec.Code)
	}
	transactions, err := w.db.ListTransactions(context.Background(), store.TransactionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 0 {
		t.Fatalf("невалидная проводка сохранена: %d", len(transactions))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "трата/CREDIT/расход") {
		t.Fatalf("в ответе нет ячейки матрицы: %s", body)
	}
}

// TestASL_11_SuggestAndMatrix проверяет помощник ввода и матрицу.
func TestASL_11_SuggestAndMatrix(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	t.Run("подсказка по кириллице", func(t *testing.T) {
		rec := w.get(t, "/suggest.json?q=прод&op_type=трата&role=расход")
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d", rec.Code)
		}
		items := decodeItems(t, rec.Body.Bytes())
		if len(items) == 0 {
			t.Fatal("нет подсказок")
		}
		first := items[0]
		if first["path"] != "Расходы:Продукты" {
			t.Fatalf("подсказка %v", first["path"])
		}
		if first["usable"] != true || first["is_leaf"] != true {
			t.Fatalf("листовой счёт помечен непригодным: %v", first)
		}
	})

	t.Run("папка помечается отдельно", func(t *testing.T) {
		rec := w.get(t, "/suggest.json?q=расходы")
		items := decodeItems(t, rec.Body.Bytes())
		if len(items) == 0 {
			t.Fatal("нет подсказок")
		}
		found := false
		for _, item := range items {
			if item["path"] == "Расходы" {
				found = true
				if item["is_leaf"] != false {
					t.Errorf("папка помечена как лист: %v", item)
				}
				if item["usable"] != false {
					t.Errorf("папка помечена как пригодная: %v", item)
				}
			}
		}
		if !found {
			t.Fatal("в подсказках нет папки «Расходы»")
		}
	})

	t.Run("матрица разрешает верный тип", func(t *testing.T) {
		rec := w.get(t, "/matrix.json?op_type=трата&role=расход&account_id="+strconvID(w.accountID(t, "Расходы:Продукты")))
		result := decodeObject(t, rec.Body.Bytes())
		if result["allowed"] != true {
			t.Fatalf("счёт отклонён: %v", result)
		}
		if result["cell"] != "трата/DEBIT/расход" {
			t.Fatalf("ячейка %v", result["cell"])
		}
	})

	t.Run("матрица отклоняет неверный тип", func(t *testing.T) {
		rec := w.get(t, "/matrix.json?op_type=трата&role=источник&account_id="+strconvID(w.accountID(t, "Расходы:Продукты")))
		result := decodeObject(t, rec.Body.Bytes())
		if result["allowed"] != false {
			t.Fatalf("неверный тип разрешён: %v", result)
		}
		if !strings.Contains(result["message"].(string), "трата/CREDIT/расход") {
			t.Fatalf("нет ячейки матрицы: %v", result["message"])
		}
	})

	t.Run("зарезервированная ячейка", func(t *testing.T) {
		rec := w.get(t, "/matrix.json?op_type=трата&role=источник&account_id="+strconvID(w.accountID(t, "Обязательства:Кредит")))
		result := decodeObject(t, rec.Body.Bytes())
		if result["allowed"] != false || result["reserved"] != true {
			t.Fatalf("резерв не отмечен: %v", result)
		}
	})
}

// decodeItems разбирает список подсказок.
func decodeItems(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("разбор подсказок: %v (%s)", err, body)
	}
	return payload.Items
}

// decodeObject разбирает одиночный JSON-объект.
func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("разбор ответа: %v (%s)", err, body)
	}
	return payload
}

// today возвращает сегодняшнюю дату в формате формы (ГГГГ-ММ-ДД).
func today() string { return time.Now().Format("2006-01-02") }

// strconvID переводит идентификатор в строку.
func strconvID(id int64) string {
	return strconv.FormatInt(id, 10)
}
