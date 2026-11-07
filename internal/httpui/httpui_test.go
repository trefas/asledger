package httpui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/auth"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/store/storetest"
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

	db := storetest.OpenTemp(t)

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

// previewValues вытаскивает скрытые поля предпросмотра из HTML формы.
//
// Сервер требует, чтобы проведение пришло после расчёта разбивки: тесты
// сначала запрашивают предпросмотр и переносят его суммы в форму проведения.
func previewValues(t *testing.T, body string) url.Values {
	t.Helper()
	pattern := regexp.MustCompile(`name="preview_(principal|interest|total)" value="(-?\d+)"`)
	values := url.Values{}
	for _, match := range pattern.FindAllStringSubmatch(body, -1) {
		values.Set("preview_"+match[1], match[2])
	}
	if len(values) != 3 {
		t.Fatalf("в ответе нет скрытых полей предпросмотра: %s", body)
	}
	return values
}

// TestASL_11_ChartEditor проверяет дерево, переименование, архивирование и
// защиту служебных счетов.
func TestASL_11_ChartEditor(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	t.Run("создание счёта и пересчёт пути потомков", func(t *testing.T) {
		rec := w.postForm(t, "/chart/create", url.Values{"name": {"Отпуск"}, "type": {"актив"}, "parent_id": {strconvID(w.accountID(t, "Активы"))}})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("создание: код %d", rec.Code)
		}
		child := w.accountID(t, "Активы:Отпуск")

		rec = w.postForm(t, "/chart/rename", url.Values{"account_id": {strconvID(child)}, "name": {"Отпуск 2026"}})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("переименование: код %d", rec.Code)
		}
		if _, err := w.db.AccountByPath(context.Background(), "Активы:Отпуск 2026"); err != nil {
			t.Fatalf("путь после переименования: %v", err)
		}
	})

	t.Run("несовпадение типа с родителем", func(t *testing.T) {
		rec := w.postForm(t, "/chart/create", url.Values{"name": {"Странный"}, "type": {"доход"}, "parent_id": {strconvID(w.accountID(t, "Активы"))}})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("код %d", rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("ошибка типа не показана: %q", rec.Header().Get("Location"))
		}
		if _, err := w.db.AccountByPath(context.Background(), "Активы:Странный"); err == nil {
			t.Fatal("счёт с неверным типом создан")
		}
	})

	t.Run("служебный счёт защищён", func(t *testing.T) {
		capital := w.accountID(t, "Капитал:Начальные")
		rec := w.postForm(t, "/chart/rename", url.Values{"account_id": {strconvID(capital)}, "name": {"Другое"}})
		if !strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("служебный счёт переименован: %q", rec.Header().Get("Location"))
		}
		rec = w.postForm(t, "/chart/archive", url.Values{"account_id": {strconvID(capital)}})
		if !strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("служебный счёт заархивирован: %q", rec.Header().Get("Location"))
		}
	})

	t.Run("архивирование без удаления", func(t *testing.T) {
		card := w.accountID(t, "Активы:Карта")
		rec := w.postForm(t, "/chart/archive", url.Values{"account_id": {strconvID(card)}})
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("архивирование: %q", rec.Header().Get("Location"))
		}
		acc, err := w.db.Account(context.Background(), card)
		if err != nil {
			t.Fatalf("счёт исчез: %v", err)
		}
		if !acc.IsArchived {
			t.Fatal("счёт не заархивирован")
		}
		// Скрыт из обычного списка, но доступен вместе с архивом.
		visible, err := w.db.ListAccounts(context.Background(), store.AccountFilter{})
		if err != nil {
			t.Fatalf("ListAccounts: %v", err)
		}
		for _, item := range visible {
			if item.ID == card {
				t.Fatal("заархивированный счёт виден в обычном списке")
			}
		}
	})

	t.Run("режим контроля остатка", func(t *testing.T) {
		rec := w.postForm(t, "/chart/mode", url.Values{"mode": {store.BalanceModeLimited}})
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("смена режима: %q", rec.Header().Get("Location"))
		}
		mode, ok, err := w.db.Setting(context.Background(), store.SettingBalanceMode)
		if err != nil || !ok || mode != store.BalanceModeLimited {
			t.Fatalf("режим: %q ok=%v err=%v", mode, ok, err)
		}
	})

	t.Run("страница дерева", func(t *testing.T) {
		rec := w.get(t, "/chart")
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "План счетов") || !strings.Contains(body, "Расходы:Продукты") {
			t.Fatal("дерево счетов не отрисовано")
		}
		if !strings.Contains(body, "служебный") {
			t.Fatal("служебные счета не помечены")
		}
	})
}

// TestASL_11_UsersAndTokens проверяет управление пользователями и токенами.
func TestASL_11_UsersAndTokens(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	t.Run("создание и переименование", func(t *testing.T) {
		rec := w.postForm(t, "/users/create", url.Values{"name": {"Мама"}, "secret": {"секрет-мамы"}})
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("создание пользователя: %q", rec.Header().Get("Location"))
		}
		user, err := w.db.UserByName(context.Background(), "Мама")
		if err != nil {
			t.Fatalf("UserByName: %v", err)
		}
		rec = w.postForm(t, "/users/rename", url.Values{"user_id": {strconvID(user.ID)}, "name": {"Мария"}})
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("переименование: %q", rec.Header().Get("Location"))
		}
		if _, err := w.db.UserByName(context.Background(), "Мария"); err != nil {
			t.Fatalf("пользователь не переименован: %v", err)
		}
	})

	t.Run("токен показывается один раз и хранится хешем", func(t *testing.T) {
		user, err := w.db.UserByName(context.Background(), "Мария")
		if err != nil {
			t.Fatalf("UserByName: %v", err)
		}
		rec := w.postForm(t, "/users/token", url.Values{
			"user_id": {strconvID(user.ID)}, "client_id": {"телефон-мамы"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("выпуск токена: код %d", rec.Code)
		}
		token := extractToken(t, rec.Body.String())
		if token == "" {
			t.Fatal("токен не показан")
		}

		device, err := w.db.DeviceByTokenHash(context.Background(), auth.HashToken(token))
		if err != nil {
			t.Fatalf("устройство не привязано: %v", err)
		}
		if device.ClientID != "телефон-мамы" || device.UserID != user.ID {
			t.Fatalf("устройство: %+v", device)
		}

		// Открытый токен больше не показывается при обычном открытии страницы.
		again := w.get(t, "/users")
		if strings.Contains(again.Body.String(), token) {
			t.Fatal("токен показан повторно")
		}
	})

	t.Run("отозванный токен перестаёт работать", func(t *testing.T) {
		rec := w.postForm(t, "/users/revoke", url.Values{"client_id": {"телефон-мамы"}})
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("отзыв: %q", rec.Header().Get("Location"))
		}
		devices, err := w.db.ListDevices(context.Background())
		if err != nil {
			t.Fatalf("ListDevices: %v", err)
		}
		for _, device := range devices {
			if device.ClientID == "телефон-мамы" {
				t.Fatal("устройство осталось после отзыва")
			}
		}
	})
}

// TestASL_11_InitialBalances проверяет ввод стартовых остатков и запрет повтора.
func TestASL_11_InitialBalances(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	rec := w.get(t, "/initial")
	if rec.Code != http.StatusOK {
		t.Fatalf("форма остатков: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Капитал:Начальные") {
		t.Fatal("форма не показывает служебную строку капитала")
	}

	form := url.Values{
		"date":              {today()},
		"asset_account":     {strconvID(w.accountID(t, "Активы:Наличные")), strconvID(w.accountID(t, "Активы:Карта"))},
		"asset_amount":      {"100 000,00", "50 000,00"},
		"liability_account": {strconvID(w.accountID(t, "Обязательства:Кредит"))},
		"liability_amount":  {"30 000,00"},
	}
	rec = w.postForm(t, "/initial", form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "kind=error") {
		t.Fatalf("ввод остатков: %q", rec.Header().Get("Location"))
	}

	ctx := context.Background()
	transactions, err := w.db.ListTransactions(ctx, store.TransactionFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("проводок %d, ожидалась 1", len(transactions))
	}
	// ΣА − ΣП = ΣК: капитал — 120 000,00 ₽.
	total, err := w.db.SubtreeBalance(ctx, "Капитал")
	if err != nil {
		t.Fatalf("SubtreeBalance: %v", err)
	}
	if total != 12000000 {
		t.Fatalf("капитал %d, ожидалось 12000000", total)
	}

	t.Run("повторный ввод запрещён", func(t *testing.T) {
		rec := w.postForm(t, "/initial", form)
		if !strings.Contains(rec.Header().Get("Location"), "kind=error") {
			t.Fatalf("повтор принят: %q", rec.Header().Get("Location"))
		}
		transactions, err := w.db.ListTransactions(ctx, store.TransactionFilter{Limit: 10})
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if len(transactions) != 1 {
			t.Fatalf("повтор создал проводку: %d", len(transactions))
		}
		// Форма заменяется сообщением о завершённом вводе.
		page := w.get(t, "/initial")
		if !strings.Contains(page.Body.String(), "Остатки уже внесены") {
			t.Fatal("форма не заблокирована после ввода")
		}
	})
}

// extractToken достаёт выпущенный токен из страницы пользователей.
func extractToken(t *testing.T, body string) string {
	t.Helper()
	const marker = `<code class="token">`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.Index(body[start:], "</code>")
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}
