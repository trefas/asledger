package httpui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// TestASL_15_WebShoppingLists проверяет страницы списков покупок и CRUD позиций.
func TestASL_15_WebShoppingLists(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	userID := w.userID(t)
	accountID := w.accountID(t, "Расходы:Продукты")

	rec := w.get(t, "/shopping-lists")
	if rec.Code != http.StatusOK {
		t.Fatalf("страница списков: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Списки покупок", "Новый список", "Покупки"} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице списков нет %q", want)
		}
	}

	rec = w.postForm(t, "/shopping-lists", url.Values{
		"name":         {"Продукты на неделю"},
		"planned_date": {"2026-11-01"},
		"owner_id":     {strconv.FormatInt(userID, 10)},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("создание списка: код %d (тело %s)", rec.Code, rec.Body.String())
	}
	listPath := redirectPath(t, rec)
	if !strings.HasPrefix(listPath, "/shopping-lists/") {
		t.Fatalf("переадресация после создания: %q", listPath)
	}

	rec = w.get(t, listPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("страница списка: код %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"Продукты на неделю", "Добавить товар", "01.11.2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице списка нет %q", want)
		}
	}

	// Добавление позиции обычной формой: статья задаётся путём счёта.
	rec = w.postForm(t, listPath+"/items", url.Values{
		"name":          {"Хлеб"},
		"account_path":  {"Расходы:Продукты"},
		"assignee_id":   {strconv.FormatInt(userID, 10)},
		"priority":      {"high"},
		"planned_price": {"50,00"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("добавление позиции: код %d (тело %s)", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	list, err := w.db.ShoppingList(ctx, listIDFromPath(t, listPath))
	if err != nil {
		t.Fatalf("ShoppingList: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("позиций = %d, ожидалась 1", len(list.Items))
	}
	item := list.Items[0]
	if item.AccountID == nil || *item.AccountID != accountID {
		t.Fatalf("статья расходов = %v, ожидался %d", item.AccountID, accountID)
	}
	if item.PlannedPrice != money.FromKopecks(5000) || item.Priority != model.ShoppingPriorityHigh {
		t.Fatalf("позиция: %+v", item)
	}
	if item.AssigneeID == nil || *item.AssigneeID != userID {
		t.Fatalf("ответственный = %v", item.AssigneeID)
	}

	// Строка позиции отрисована вместе с формой правки и статьёй расходов.
	rec = w.get(t, listPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("список после добавления: код %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"Хлеб", "Расходы:Продукты", "/shopping-items/" + strconv.FormatInt(item.ID, 10), "name=\"version\""} {
		if !strings.Contains(body, want) {
			t.Errorf("в списке нет %q", want)
		}
	}

	// Редактирование позиции с версией.
	rec = w.postForm(t, "/shopping-items/"+strconv.FormatInt(item.ID, 10), url.Values{
		"version":       {strconv.FormatInt(item.Version, 10)},
		"list_id":       {strconv.FormatInt(list.ID, 10)},
		"name":          {"Хлеб бородинский"},
		"account_id":    {strconv.FormatInt(accountID, 10)},
		"assignee_id":   {""},
		"priority":      {"normal"},
		"planned_price": {"62,50"},
		"comment":       {"взять свежий"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("редактирование позиции: код %d (тело %s)", rec.Code, rec.Body.String())
	}
	updated, err := w.db.ShoppingItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ShoppingItem: %v", err)
	}
	if updated.Name != "Хлеб бородинский" || updated.PlannedPrice != money.FromKopecks(6250) {
		t.Fatalf("позиция после правки: %+v", updated)
	}
	if updated.Version != item.Version+1 || updated.AssigneeID != nil {
		t.Fatalf("версия/ответственный после правки: %+v", updated)
	}

	// Устаревшая версия возвращает на страницу с сообщением об ошибке.
	rec = w.postForm(t, "/shopping-items/"+strconv.FormatInt(item.ID, 10), url.Values{
		"version": {strconv.FormatInt(item.Version, 10)},
		"list_id": {strconv.FormatInt(list.ID, 10)},
		"name":    {"Устаревшая правка"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("конфликт версий: код %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "kind=error") {
		t.Fatalf("конфликт версий: переадресация %q, ожидалось сообщение об ошибке", location)
	}

	// Удаление позиции.
	rec = w.postForm(t, "/shopping-items/"+strconv.FormatInt(item.ID, 10)+"/delete", url.Values{
		"list_id": {strconv.FormatInt(list.ID, 10)},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("удаление позиции: код %d", rec.Code)
	}
	list, err = w.db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList после удаления: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("после удаления позиций = %d", len(list.Items))
	}

	// Смена статуса и архивация.
	rec = w.postForm(t, listPath+"/status", url.Values{"status": {"shopping"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("смена статуса: код %d", rec.Code)
	}
	rec = w.postForm(t, listPath+"/archive", nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("архивация: код %d", rec.Code)
	}
	list, err = w.db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList после архивации: %v", err)
	}
	if list.Status != model.ShoppingListArchived {
		t.Fatalf("статус после архивации = %q", list.Status)
	}

	rec = w.get(t, "/shopping-lists?include_archived=1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Продукты на неделю") {
		t.Fatalf("архивный список не виден: код %d", rec.Code)
	}
}

// TestASL_15_WebShoppingAddJSON проверяет добавление позиций без перезагрузки.
func TestASL_15_WebShoppingAddJSON(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	userID := w.userID(t)
	ctx := context.Background()

	list, err := w.db.CreateShoppingList(ctx, store.ShoppingListInput{
		Name:    "Сто позиций",
		OwnerID: userID,
	})
	if err != nil {
		t.Fatalf("CreateShoppingList: %v", err)
	}
	listPath := "/shopping-lists/" + strconv.FormatInt(list.ID, 10)

	for i := 1; i <= 100; i++ {
		rec := w.postJSON(t, listPath+"/items", url.Values{
			"name":         {"Товар " + strconv.Itoa(i)},
			"account_path": {"Расходы:Продукты"},
			"priority":     {"normal"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("добавление %d: код %d (тело %s)", i, rec.Code, rec.Body.String())
		}
		var payload struct {
			OK   bool   `json:"ok"`
			HTML string `json:"html"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("разбор ответа %d: %v", i, err)
		}
		if !payload.OK || !strings.Contains(payload.HTML, "Товар "+strconv.Itoa(i)) {
			t.Fatalf("ответ %d не содержит строку позиции: %s", i, rec.Body.String())
		}
	}

	list, err = w.db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList: %v", err)
	}
	if list.ItemCount != 100 {
		t.Fatalf("позиций = %d, ожидалось 100", list.ItemCount)
	}

	// Ошибка валидации приходит JSON с понятным сообщением.
	rec := w.postJSON(t, listPath+"/items", url.Values{
		"name":         {"Неверная статья"},
		"account_path": {"Активы:Наличные"},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("неверная статья: код %d, ожидался 422 (тело %s)", rec.Code, rec.Body.String())
	}
	var failure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &failure); err != nil {
		t.Fatalf("разбор ошибки: %v", err)
	}
	if !strings.Contains(failure.Error, "расход") {
		t.Fatalf("сообщение об ошибке = %q", failure.Error)
	}

	// Подсказка статей расходов отдаёт план счетов.
	rec = w.get(t, "/shopping/suggest.json?q="+url.QueryEscape("Продукт"))
	if rec.Code != http.StatusOK {
		t.Fatalf("подсказка: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Расходы:Продукты") {
		t.Fatalf("подсказка не содержит статью: %s", rec.Body.String())
	}
}

// postJSON отправляет форму с заголовком Accept: application/json.
func (w *testWeb) postJSON(t *testing.T, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	w.handler.ServeHTTP(rec, req)
	return rec
}

// redirectPath возвращает путь переадресации без строки запроса.
func redirectPath(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatalf("нет заголовка Location")
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("разбор Location %q: %v", location, err)
	}
	return parsed.Path
}

// listIDFromPath достаёт идентификатор списка из адреса страницы.
func listIDFromPath(t *testing.T, path string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(path, "/shopping-lists/"), 10, 64)
	if err != nil {
		t.Fatalf("идентификатор списка из %q: %v", path, err)
	}
	return id
}

// TestASL_15_WebShoppingShop проверяет режим магазина: подтверждение ценой и
// отметкой «в корзине», расхождение, итог и сохранение данных при конфликте.
func TestASL_15_WebShoppingShop(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	userID := w.userID(t)
	accountID := w.accountID(t, "Расходы:Продукты")
	ctx := context.Background()

	list, err := w.db.CreateShoppingList(ctx, store.ShoppingListInput{
		Name: "Поход в магазин", OwnerID: userID,
	})
	if err != nil {
		t.Fatalf("CreateShoppingList: %v", err)
	}
	add := func(name string) *model.ShoppingListItem {
		t.Helper()
		item, err := w.db.AddShoppingItem(ctx, list.ID, store.ShoppingItemInput{
			Name: name, AccountID: &accountID, Priority: model.ShoppingPriorityNormal,
		})
		if err != nil {
			t.Fatalf("AddShoppingItem(%q): %v", name, err)
		}
		return item
	}
	bread := add("Хлеб")
	milk := add("Молоко")
	cheese := add("Сыр")

	shopPath := "/shopping-lists/" + strconv.FormatInt(list.ID, 10) + "/shop"
	rec := w.get(t, shopPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("экран магазина: код %d (тело %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"в магазине", "Хлеб", "Молоко", "Сыр", "Итого", "неподтверждённых: 3"} {
		if !strings.Contains(body, want) {
			t.Errorf("на экране магазина нет %q", want)
		}
	}

	// Хлеб подтверждаем ценой, отмечаем «в корзине» и расхождение.
	rec = w.postForm(t, shopPath+"/"+strconv.FormatInt(bread.ID, 10), url.Values{
		"version":        {strconv.FormatInt(bread.Version, 10)},
		"actual_price":   {"59,90"},
		"in_cart":        {"1"},
		"price_mismatch": {"1"},
		"comment":        {"ценник 59,90"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сохранение хлеба: код %d (тело %s)", rec.Code, rec.Body.String())
	}
	saved, err := w.db.ShoppingItem(ctx, bread.ID)
	if err != nil {
		t.Fatalf("ShoppingItem(хлеб): %v", err)
	}
	if saved.Status != model.ShoppingItemConfirmed || !saved.Confirmed() {
		t.Fatalf("хлеб не подтверждён: %+v", saved)
	}
	if saved.ActualPrice != money.FromKopecks(5990) {
		t.Fatalf("цена хлеба = %s, ожидалось 59,90 ₽", saved.ActualPrice)
	}
	if !saved.InCart || !saved.PriceMismatch || saved.Comment != "ценник 59,90" {
		t.Fatalf("отметки хлеба: %+v", saved)
	}
	breadVersion := saved.Version

	// Молоко подтверждаем только отметкой «в корзине» — цена ещё неизвестна.
	rec = w.postForm(t, shopPath+"/"+strconv.FormatInt(milk.ID, 10), url.Values{
		"version": {strconv.FormatInt(milk.Version, 10)},
		"in_cart": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("сохранение молока: код %d (тело %s)", rec.Code, rec.Body.String())
	}
	saved, err = w.db.ShoppingItem(ctx, milk.ID)
	if err != nil {
		t.Fatalf("ShoppingItem(молоко): %v", err)
	}
	if saved.Status != model.ShoppingItemConfirmed || !saved.Confirmed() {
		t.Fatalf("молоко не подтверждено отметкой: %+v", saved)
	}
	if saved.ActualPrice != 0 {
		t.Fatalf("цена молока = %s, ожидался ноль", saved.ActualPrice)
	}

	// Сыр остаётся неподтверждённым и не попадает в итог.
	unchanged, err := w.db.ShoppingItem(ctx, cheese.ID)
	if err != nil {
		t.Fatalf("ShoppingItem(сыр): %v", err)
	}
	if unchanged.Confirmed() || unchanged.Status != model.ShoppingItemPlanned {
		t.Fatalf("сыр подтверждён без цены и отметки: %+v", unchanged)
	}

	list, err = w.db.ShoppingList(ctx, list.ID)
	if err != nil {
		t.Fatalf("ShoppingList: %v", err)
	}
	if list.ConfirmedCount != 2 || list.ItemCount != 3 {
		t.Fatalf("подтверждено %d из %d, ожидалось 2 из 3", list.ConfirmedCount, list.ItemCount)
	}
	if list.ConfirmedTotal != money.FromKopecks(5990) {
		t.Fatalf("итог = %s, ожидалось 59,90 ₽ (неподтверждённые и нулевые не в счёте)", list.ConfirmedTotal)
	}

	rec = w.get(t, shopPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("экран магазина после сохранения: код %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"59,90 ₽", "неподтверждённых: 1", "расхождение цены"} {
		if !strings.Contains(body, want) {
			t.Errorf("на экране магазина после сохранения нет %q", want)
		}
	}

	// Конфликт версий: позицию изменили на другом устройстве, а телефон
	// присылает устаревшую версию с новой ценой. Введённая цена не теряется.
	newName := "Хлеб бородинский"
	if _, err := w.db.UpdateShoppingItem(ctx, bread.ID, store.ShoppingItemPatch{
		Version: breadVersion, Name: &newName,
	}); err != nil {
		t.Fatalf("правка хлеба на «другом устройстве»: %v", err)
	}

	rec = w.postForm(t, shopPath+"/"+strconv.FormatInt(bread.ID, 10), url.Values{
		"version":      {strconv.FormatInt(bread.Version, 10)},
		"actual_price": {"65,00"},
		"in_cart":      {"1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("конфликт версий: код %d, ожидалась перерисовка (тело %s)", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "65,00") {
		t.Fatalf("при конфликте потеряна введённая цена: %s", body)
	}
	if !strings.Contains(body, "изменили на другом устройстве") {
		t.Fatalf("при конфликте нет понятного сообщения: %s", body)
	}
	after, err := w.db.ShoppingItem(ctx, bread.ID)
	if err != nil {
		t.Fatalf("ShoppingItem(хлеб) после конфликта: %v", err)
	}
	if after.ActualPrice != money.FromKopecks(5990) || after.Name != "Хлеб бородинский" {
		t.Fatalf("конфликт изменил данные: %+v", after)
	}
}
