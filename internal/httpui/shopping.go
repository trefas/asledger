package httpui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
	"github.com/trefas/asledger/pkg/core/uuid"
)

// shoppingStatusOption — статус в фильтре и форме.
type shoppingStatusOption struct {
	// Value — значение статуса в книге.
	Value string
	// Label — подпись на кириллице.
	Label string
}

// shoppingPriorityOption — приоритет позиции в форме.
type shoppingPriorityOption struct {
	// Value — значение приоритета в книге.
	Value string
	// Label — подпись на кириллице.
	Label string
}

// shoppingListCard — список покупок для страницы.
type shoppingListCard struct {
	// List — данные списка.
	List model.ShoppingList
	// Owner — имя владельца.
	Owner string
	// Status — подпись статуса.
	Status string
	// StatusValue — значение статуса для формы.
	StatusValue string
	// Planned — дата похода или «не задана».
	Planned string
}

// shoppingListView — данные страницы списков покупок.
type shoppingListView struct {
	Lists           []shoppingListCard
	Status          string
	IncludeArchived bool
	Statuses        []shoppingStatusOption
	Users           []model.User
}

// shoppingItemRow — позиция списка для страницы.
type shoppingItemRow struct {
	Item          model.ShoppingListItem
	Account       string
	AccountID     int64
	Assignee      string
	AssigneeID    int64
	Priority      string
	PriorityValue string
	Status        string
	StatusValue   string
	// Accounts и Users — справочники для формы редактирования строки.
	Accounts []model.Account
	Users    []model.User
}

// shoppingDetailView — данные страницы одного списка покупок.
type shoppingDetailView struct {
	List       shoppingListCard
	Items      []shoppingItemRow
	Users      []model.User
	Accounts   []model.Account
	Priorities []shoppingPriorityOption
	Statuses   []shoppingStatusOption
}

// shopDraft — введённые в магазине значения, которые нужно сохранить при
// ошибке (например, конфликте версий), чтобы пользователь не потерял данные.
type shopDraft struct {
	ItemID        int64
	ActualPrice   string
	InCart        bool
	PriceMismatch bool
	Comment       string
}

// shopItemRow — позиция на экране магазина.
type shopItemRow struct {
	Item          model.ShoppingListItem
	Account       string
	Status        string
	StatusValue   string
	Confirmed     bool
	ActualPrice   string
	InCart        bool
	PriceMismatch bool
	Comment       string
}

// shoppingShopView — данные экрана «в магазине».
type shoppingShopView struct {
	List             shoppingListCard
	Items            []shopItemRow
	UnconfirmedCount int
	Error            string
}

// checkoutItemRow — подтверждённая позиция на экране оформления покупки.
type checkoutItemRow struct {
	Item        model.ShoppingListItem
	AccountID   int64
	AccountPath string
	Amount      string
	AmountKop   money.Kop
}

// checkoutSourceRow — счёт-источник оплаты с суммой.
type checkoutSourceRow struct {
	AccountID   int64
	AccountPath string
	Amount      string
}

// checkoutLine — строка предпросмотра проводки.
type checkoutLine struct {
	AccountPath string
	Side        string
	Amount      money.Kop
}

// checkoutPreview — предпросмотр проводки покупки до записи.
type checkoutPreview struct {
	Date        string
	Description string
	Actor       string
	Lines       []checkoutLine
	Total       money.Kop
	DebitTotal  money.Kop
	CreditTotal money.Kop
}

// checkoutView — данные экрана оформления покупки.
type checkoutView struct {
	List        shoppingListCard
	Items       []checkoutItemRow
	Unconfirmed []shoppingItemRow
	Sources     []checkoutSourceRow
	Accounts    []model.Account
	Users       []model.User
	ActorID     int64
	Date        string
	Description string
	RequestID   string
	Total       money.Kop
	Error       string
	Preview     *checkoutPreview
}

// checkoutSubmitted — значения формы оформления, сохранённые при ошибке.
type checkoutSubmitted struct {
	ItemAccounts  map[int64]int64
	ItemAmounts   map[int64]string
	SourceAmounts map[int64]string
	Date          string
	Description   string
	ActorID       int64
	// RequestID — uuid будущей проводки: повторная отправка не создаёт дубль.
	RequestID string
}

// shoppingListStatuses — варианты статуса списка с подписями.
func shoppingListStatuses() []shoppingStatusOption {
	options := make([]shoppingStatusOption, 0, len(model.ShoppingListStatuses()))
	for _, status := range model.ShoppingListStatuses() {
		options = append(options, shoppingStatusOption{
			Value: string(status),
			Label: shoppingListStatusLabel(status),
		})
	}
	return options
}

// shoppingPriorities — варианты приоритета с подписями.
func shoppingPriorities() []shoppingPriorityOption {
	options := make([]shoppingPriorityOption, 0, len(model.ShoppingPriorities()))
	for _, priority := range model.ShoppingPriorities() {
		options = append(options, shoppingPriorityOption{
			Value: string(priority),
			Label: shoppingPriorityLabel(priority),
		})
	}
	return options
}

// shoppingListStatusLabel переводит статус списка в кириллицу.
func shoppingListStatusLabel(status model.ShoppingListStatus) string {
	switch status {
	case model.ShoppingListDraft:
		return "черновик"
	case model.ShoppingListActive:
		return "в работе"
	case model.ShoppingListShopping:
		return "в магазине"
	case model.ShoppingListCompleted:
		return "завершён"
	case model.ShoppingListArchived:
		return "архив"
	default:
		return string(status)
	}
}

// shoppingItemStatusLabel переводит статус позиции в кириллицу.
func shoppingItemStatusLabel(status model.ShoppingItemStatus) string {
	switch status {
	case model.ShoppingItemPlanned:
		return "запланировано"
	case model.ShoppingItemConfirmed:
		return "подтверждено"
	case model.ShoppingItemPurchased:
		return "куплено"
	case model.ShoppingItemSkipped:
		return "пропущено"
	default:
		return string(status)
	}
}

// shoppingPriorityLabel переводит приоритет в кириллицу.
func shoppingPriorityLabel(priority model.ShoppingPriority) string {
	switch priority {
	case model.ShoppingPriorityLow:
		return "низкий"
	case model.ShoppingPriorityNormal:
		return "обычный"
	case model.ShoppingPriorityHigh:
		return "высокий"
	default:
		return string(priority)
	}
}

// handleShoppingLists показывает списки покупок и форму создания.
func (s *Server) handleShoppingLists(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	filter := store.ShoppingListFilter{IncludeArchived: queryBool(query, "include_archived")}
	view := shoppingListView{
		Status:          strings.TrimSpace(query.Get("status")),
		IncludeArchived: filter.IncludeArchived,
		Statuses:        shoppingListStatuses(),
	}
	if view.Status != "" {
		status := model.ShoppingListStatus(view.Status)
		if !status.Valid() {
			s.renderError(w, r, http.StatusBadRequest, "Неизвестный статус списка покупок.")
			return
		}
		filter.Status = &status
	}

	lists, err := s.db.ListShoppingLists(ctx, filter)
	if err != nil {
		s.fail(w, err)
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	view.Users = users
	names := userNames(users)
	for i := range lists {
		view.Lists = append(view.Lists, toShoppingListCard(&lists[i], names))
	}
	s.render(w, r, "shopping", pageData{Title: "Списки покупок", Active: "shopping", Data: view})
}

// handleShoppingListCreate создаёт список покупок из формы.
func (s *Server) handleShoppingListCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	planned, err := parseFormDate(r.FormValue("planned_date"))
	if err != nil {
		s.mutationError(w, r, "/shopping-lists", err.Error())
		return
	}
	ownerID, err := s.shoppingOwner(ctx, r.FormValue("owner_id"))
	if err != nil {
		s.mutationError(w, r, "/shopping-lists", err.Error())
		return
	}
	list, err := s.db.CreateShoppingList(ctx, store.ShoppingListInput{
		Name:        r.FormValue("name"),
		PlannedDate: planned,
		OwnerID:     ownerID,
	})
	if err != nil {
		s.mutationError(w, r, "/shopping-lists", shoppingErrorMessage(err))
		return
	}
	s.redirectOK(w, r, shoppingListPath(list.ID), "Список создан: "+list.Name)
}

// handleShoppingListDetail показывает список покупок с позициями.
func (s *Server) handleShoppingListDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	view, err := s.shoppingDetailView(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	s.render(w, r, "shopping_detail", pageData{
		Title: view.List.List.Name, Active: "shopping", Data: view,
	})
}

// handleShoppingItemCreate добавляет позицию в список.
//
// При заголовке Accept: application/json отвечает фрагментом строки и
// сводкой — форма добавляет товары без перезагрузки страницы.
func (s *Server) handleShoppingItemCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	jsonMode := wantsJSON(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.shoppingItemCreateError(w, r, jsonMode, http.StatusBadRequest, "не удалось определить список покупок")
		return
	}
	back := shoppingListPath(id)
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	input, err := s.shoppingItemInput(r)
	if err != nil {
		s.shoppingItemCreateError(w, r, jsonMode, http.StatusUnprocessableEntity, err.Error())
		return
	}
	item, err := s.db.AddShoppingItem(ctx, id, input)
	if err != nil {
		s.shoppingItemCreateError(w, r, jsonMode, shoppingStatusFor(err), shoppingErrorMessage(err))
		return
	}
	if !jsonMode {
		s.redirectOK(w, r, back, "Позиция добавлена: "+item.Name)
		return
	}

	row, err := s.shoppingRow(ctx, item)
	if err != nil {
		s.fail(w, err)
		return
	}
	rowHTML, err := s.renderShoppingRow(row)
	if err != nil {
		s.fail(w, err)
		return
	}
	list, err := s.db.ShoppingList(ctx, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeShoppingJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"html":                rowHTML,
		"item_count":          list.ItemCount,
		"confirmed_count":     list.ConfirmedCount,
		"confirmed_total_kop": list.ConfirmedTotal.String(),
	})
}

// shoppingItemCreateError отвечает на ошибку добавления позиции.
func (s *Server) shoppingItemCreateError(w http.ResponseWriter, r *http.Request, jsonMode bool, status int, message string) {
	if status == http.StatusInternalServerError {
		s.logger.Error("не удалось добавить позицию списка покупок", "err", message)
	}
	if jsonMode {
		writeShoppingJSON(w, status, map[string]any{"error": message})
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.mutationError(w, r, shoppingListPath(id), message)
}

// handleShoppingItemUpdate изменяет позицию с проверкой версии.
func (s *Server) handleShoppingItemUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := s.shoppingItemID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	back := s.shoppingBackPath(r)
	version, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("version")), 10, 64)
	if err != nil || version <= 0 {
		s.mutationError(w, r, back, "не задана версия позиции — обновите страницу и повторите")
		return
	}
	patch, err := s.shoppingItemPatch(r, version)
	if err != nil {
		s.mutationError(w, r, back, err.Error())
		return
	}
	item, err := s.db.UpdateShoppingItem(ctx, id, patch)
	if err != nil {
		s.mutationError(w, r, back, shoppingErrorMessage(err))
		return
	}
	s.redirectOK(w, r, back, "Позиция сохранена: "+item.Name)
}

// handleShoppingItemDelete удаляет позицию списка.
func (s *Server) handleShoppingItemDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := s.shoppingItemID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	back := s.shoppingBackPath(r)
	if err := s.db.DeleteShoppingItem(r.Context(), id); err != nil {
		s.mutationError(w, r, back, shoppingErrorMessage(err))
		return
	}
	s.redirectOK(w, r, back, "Позиция удалена")
}

// handleShoppingListArchive переводит список в архив.
func (s *Server) handleShoppingListArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	if err := s.db.ArchiveShoppingList(r.Context(), id); err != nil {
		s.mutationError(w, r, shoppingListPath(id), shoppingErrorMessage(err))
		return
	}
	s.redirectOK(w, r, "/shopping-lists", "Список переведён в архив")
}

// handleShoppingListStatus меняет статус списка.
func (s *Server) handleShoppingListStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	status := model.ShoppingListStatus(strings.TrimSpace(r.FormValue("status")))
	if !status.Valid() {
		s.mutationError(w, r, shoppingListPath(id), "неизвестный статус списка")
		return
	}
	if _, err := s.db.UpdateShoppingList(r.Context(), id, store.ShoppingListPatch{Status: &status}); err != nil {
		s.mutationError(w, r, shoppingListPath(id), shoppingErrorMessage(err))
		return
	}
	s.redirectOK(w, r, shoppingListPath(id), "Статус списка: "+shoppingListStatusLabel(status))
}

// handleShoppingSuggestJSON подсказывает статьи расходов по плану и истории.
func (s *Server) handleShoppingSuggestJSON(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := s.db.SuggestShoppingAccounts(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeWebJSON(w, map[string]any{"items": items})
}

// handleShoppingShop показывает экран «в магазине».
func (s *Server) handleShoppingShop(w http.ResponseWriter, r *http.Request) {
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	view, err := s.shoppingShopView(r.Context(), id, nil)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	s.render(w, r, "shopping_shop", pageData{
		Title: view.List.List.Name + " — в магазине", Active: "shopping", Data: view,
	})
}

// handleShoppingShopUpdate сохраняет фактическую цену и отметки позиции.
//
// При конфликте версий страница перерисовывается с введёнными значениями,
// чтобы покупатель не потерял цену из ценника.
func (s *Server) handleShoppingShopUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	listID, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	itemID, err := strconv.ParseInt(r.PathValue("item"), 10, 64)
	if err != nil || itemID <= 0 {
		s.renderError(w, r, http.StatusBadRequest, "Не удалось определить позицию списка.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	draft := &shopDraft{
		ItemID:        itemID,
		ActualPrice:   strings.TrimSpace(r.FormValue("actual_price")),
		InCart:        formChecked(r, "in_cart"),
		PriceMismatch: formChecked(r, "price_mismatch"),
		Comment:       strings.TrimSpace(r.FormValue("comment")),
	}

	item, err := s.db.ShoppingItem(ctx, itemID)
	if err != nil || item.ListID != listID {
		s.renderError(w, r, http.StatusNotFound, "Позиция списка не найдена.")
		return
	}
	version, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("version")), 10, 64)
	if err != nil || version <= 0 {
		s.shopFailure(w, r, listID, draft, "не задана версия позиции — обновите страницу и повторите")
		return
	}
	actual, err := parseFormMoney(draft.ActualPrice)
	if err != nil {
		s.shopFailure(w, r, listID, draft, err.Error())
		return
	}

	status := item.Status
	if status == model.ShoppingItemPlanned || status == model.ShoppingItemConfirmed {
		status = model.ShoppingItemPlanned
		if actual.Kopecks() > 0 || draft.InCart {
			status = model.ShoppingItemConfirmed
		}
	}
	comment := draft.Comment
	_, err = s.db.UpdateShoppingItem(ctx, itemID, store.ShoppingItemPatch{
		Version:       version,
		ActualPrice:   &actual,
		InCart:        &draft.InCart,
		PriceMismatch: &draft.PriceMismatch,
		Comment:       &comment,
		Status:        &status,
	})
	if err != nil {
		message := shoppingErrorMessage(err)
		var ve *store.ValidationError
		if errors.As(err, &ve) && ve.Code == store.CodeShoppingVersionConflict {
			message = "Позицию изменили на другом устройстве. Проверьте цену и сохраните ещё раз."
		}
		s.shopFailure(w, r, listID, draft, message)
		return
	}
	s.redirectShop(w, r, listID, itemID, "Сохранено: "+item.Name)
}

// shopFailure перерисовывает экран магазина с сообщением и введёнными данными.
func (s *Server) shopFailure(w http.ResponseWriter, r *http.Request, listID int64, draft *shopDraft, message string) {
	view, err := s.shoppingShopView(r.Context(), listID, draft)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	view.Error = message
	s.renderPageWithStatus(w, r, "shopping_shop", view.List.List.Name+" — в магазине", "shopping", view, http.StatusOK)
}

// redirectShop переадресует на экран магазина к сохранённой позиции.
func (s *Server) redirectShop(w http.ResponseWriter, r *http.Request, listID, itemID int64, message string) {
	target := shoppingShopPath(listID) + "?flash=" + encodeQuery(message) + "&kind=ok"
	if itemID > 0 {
		target += "#item-" + strconv.FormatInt(itemID, 10)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// shoppingShopView собирает данные экрана магазина.
func (s *Server) shoppingShopView(ctx context.Context, id int64, draft *shopDraft) (shoppingShopView, error) {
	list, err := s.db.ShoppingList(ctx, id)
	if err != nil {
		return shoppingShopView{}, err
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return shoppingShopView{}, err
	}
	paths, err := s.accountPaths(ctx)
	if err != nil {
		return shoppingShopView{}, err
	}
	view := shoppingShopView{List: toShoppingListCard(list, userNames(users))}
	for i := range list.Items {
		item := &list.Items[i]
		row := shopItemRow{
			Item:          *item,
			Status:        shoppingItemStatusLabel(item.Status),
			StatusValue:   string(item.Status),
			Confirmed:     item.Confirmed(),
			ActualPrice:   kopInput(item.ActualPrice),
			InCart:        item.InCart,
			PriceMismatch: item.PriceMismatch,
			Comment:       item.Comment,
		}
		if item.AccountID != nil {
			row.Account = paths[*item.AccountID]
		}
		if draft != nil && draft.ItemID == item.ID {
			row.ActualPrice = draft.ActualPrice
			row.InCart = draft.InCart
			row.PriceMismatch = draft.PriceMismatch
			row.Comment = draft.Comment
		}
		if !row.Confirmed {
			view.UnconfirmedCount++
		}
		view.Items = append(view.Items, row)
	}
	return view, nil
}

// shoppingShopPath собирает адрес экрана магазина.
func shoppingShopPath(id int64) string {
	return shoppingListPath(id) + "/shop"
}

// formChecked сообщает, отмечен ли флажок формы.
func formChecked(r *http.Request, name string) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(name))) {
	case "1", "on", "true", "да":
		return true
	default:
		return false
	}
}

// kopInput выводит сумму для поля ввода: «1 250,00» без знака рубля.
func kopInput(k money.Kop) string {
	if k == 0 {
		return ""
	}
	return strings.TrimSuffix(k.String(), " ₽")
}

// handleShoppingCheckout показывает экран оформления покупки.
func (s *Server) handleShoppingCheckout(w http.ResponseWriter, r *http.Request) {
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	view, err := s.shoppingCheckoutView(r.Context(), id, nil)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	s.render(w, r, "shopping_checkout", pageData{
		Title: "Оформить покупку", Active: "shopping", Data: view,
	})
}

// handleShoppingCheckoutPreview собирает предпросмотр проводки без записи.
//
// Проведение покупки — отдельная задача; здесь показываются строки, которые
// получит движок, и проверяются инварианты матрицы ASL-2.2.
func (s *Server) handleShoppingCheckoutPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	list, err := s.db.ShoppingList(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}

	names := userNames(users)
	in, err := checkoutSubmittedFromForm(r, list, names)
	if err != nil {
		s.fail(w, err)
		return
	}

	entries, total, err := checkoutItemEntries(r, list, in)
	if err != nil {
		s.checkoutFailure(w, r, id, in, err.Error())
		return
	}
	sourceEntries, err := s.checkoutSourceEntries(ctx, r, total, in)
	if err != nil {
		s.checkoutFailure(w, r, id, in, err.Error())
		return
	}
	entries = append(entries, sourceEntries...)

	date, err := parseDate(in.Date)
	if err != nil {
		s.checkoutFailure(w, r, id, in, err.Error())
		return
	}
	prepared, err := s.engine.Prepare(ctx, engine.Operation{
		OpType:      model.OpExpense,
		UUID:        in.RequestID,
		Date:        date,
		Description: in.Description,
		Source:      "web",
		Entries:     entries,
	}, in.ActorID)
	if err != nil {
		s.checkoutFailure(w, r, id, in, statusMessage(err))
		return
	}

	view, err := s.shoppingCheckoutView(ctx, id, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	preview, err := s.checkoutPreview(ctx, prepared.Transaction, total, names)
	if err != nil {
		s.fail(w, err)
		return
	}
	view.Error = ""
	view.Preview = preview
	s.renderPageWithStatus(w, r, "shopping_checkout", "Оформить покупку", "shopping", view, http.StatusOK)
}

// checkoutSubmittedFromForm разбирает форму оформления и сохраняет значения
// для повторной отрисовки. request_id переносится из формы: повторная отправка
// того же оформления остаётся идемпотентной.
func checkoutSubmittedFromForm(r *http.Request, list *model.ShoppingList, names map[int64]string) (*checkoutSubmitted, error) {
	in := &checkoutSubmitted{
		ItemAccounts:  make(map[int64]int64),
		ItemAmounts:   make(map[int64]string),
		SourceAmounts: make(map[int64]string),
		Date:          strings.TrimSpace(r.FormValue("date")),
		Description:   strings.TrimSpace(r.FormValue("description")),
		ActorID:       parseID(r.FormValue("actor_id"), list.OwnerID),
		RequestID:     strings.TrimSpace(r.FormValue("request_id")),
	}
	if in.Date == "" {
		in.Date = todayString()
	}
	if _, ok := names[in.ActorID]; !ok {
		in.ActorID = list.OwnerID
	}
	if in.RequestID == "" {
		id, err := uuid.New()
		if err != nil {
			return nil, err
		}
		in.RequestID = id
	}
	return in, nil
}

// checkoutItemIDs возвращает позиции, которые попадут в проводку.
func checkoutItemIDs(list *model.ShoppingList) []int64 {
	var ids []int64
	for i := range list.Items {
		if !checkoutable(&list.Items[i]) {
			continue
		}
		ids = append(ids, list.Items[i].ID)
	}
	return ids
}

// checkoutable сообщает, участвует ли позиция в оформлении покупки.
func checkoutable(item *model.ShoppingListItem) bool {
	switch item.Status {
	case model.ShoppingItemPurchased, model.ShoppingItemSkipped:
		return false
	}
	return item.Confirmed()
}

// handleShoppingCheckoutPost проводит покупку: записывает проводку и отмечает
// позиции купленными одной транзакцией БД.
func (s *Server) handleShoppingCheckoutPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := s.shoppingID(w, r, "/shopping-lists")
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	list, err := s.db.ShoppingList(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	names := userNames(users)
	in, err := checkoutSubmittedFromForm(r, list, names)
	if err != nil {
		s.fail(w, err)
		return
	}

	// Повторная отправка того же оформления: проводка уже есть, дубль не нужен.
	if _, err := s.db.TransactionByUUID(ctx, in.RequestID); err == nil {
		s.redirectOK(w, r, shoppingListPath(id), "Эта покупка уже проведена: повтор не создал дубль")
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}

	entries, total, err := checkoutItemEntries(r, list, in)
	if err != nil {
		s.checkoutFailure(w, r, id, in, err.Error())
		return
	}
	sourceEntries, err := s.checkoutSourceEntries(ctx, r, total, in)
	if err != nil {
		s.checkoutFailure(w, r, id, in, err.Error())
		return
	}
	entries = append(entries, sourceEntries...)
	date, err := parseDate(in.Date)
	if err != nil {
		s.checkoutFailure(w, r, id, in, err.Error())
		return
	}
	prepared, err := s.engine.Prepare(ctx, engine.Operation{
		OpType:      model.OpExpense,
		UUID:        in.RequestID,
		Date:        date,
		Description: in.Description,
		Source:      "web",
		Entries:     entries,
	}, in.ActorID)
	if err != nil {
		s.checkoutFailure(w, r, id, in, statusMessage(err))
		return
	}

	_, created, err := s.db.PostShoppingPurchase(ctx, store.ShoppingPurchaseInput{
		ListID:      id,
		ItemIDs:     checkoutItemIDs(list),
		Transaction: prepared.Transaction,
	})
	if err != nil {
		s.checkoutFailure(w, r, id, in, shoppingErrorMessage(err))
		return
	}
	if created {
		s.redirectOK(w, r, shoppingListPath(id), "Покупка проведена: позиции отмечены купленными")
		return
	}
	s.redirectOK(w, r, shoppingListPath(id), "Эта покупка уже проведена: повтор не создал дубль")
}

// checkoutFailure перерисовывает оформление с сообщением и введёнными данными.
func (s *Server) checkoutFailure(w http.ResponseWriter, r *http.Request, id int64, in *checkoutSubmitted, message string) {
	view, err := s.shoppingCheckoutView(r.Context(), id, in)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "Список покупок не найден.")
			return
		}
		s.fail(w, err)
		return
	}
	view.Error = message
	s.renderPageWithStatus(w, r, "shopping_checkout", "Оформить покупку", "shopping", view, http.StatusOK)
}

// shoppingCheckoutView собирает данные экрана оформления покупки.
func (s *Server) shoppingCheckoutView(ctx context.Context, id int64, in *checkoutSubmitted) (checkoutView, error) {
	list, err := s.db.ShoppingList(ctx, id)
	if err != nil {
		return checkoutView{}, err
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return checkoutView{}, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return checkoutView{}, err
	}
	paths, err := s.accountPaths(ctx)
	if err != nil {
		return checkoutView{}, err
	}
	names := userNames(users)

	view := checkoutView{
		List:        toShoppingListCard(list, names),
		Users:       users,
		Date:        todayString(),
		Description: "Покупки: " + list.Name,
		ActorID:     list.OwnerID,
	}
	if in != nil {
		view.Date = in.Date
		view.Description = in.Description
		view.ActorID = in.ActorID
		view.RequestID = in.RequestID
	}
	if view.RequestID == "" {
		requestID, err := uuid.New()
		if err != nil {
			return checkoutView{}, err
		}
		view.RequestID = requestID
	}

	var assetAccounts []model.Account
	for i := range accounts {
		switch accounts[i].Type {
		case model.TypeExpense:
			view.Accounts = append(view.Accounts, accounts[i])
		case model.TypeAsset:
			// Служебная «Дебиторка» — не источник оплаты покупки.
			if accounts[i].IsSystem {
				continue
			}
			assetAccounts = append(assetAccounts, accounts[i])
		}
	}

	for i := range list.Items {
		item := &list.Items[i]
		if item.Status == model.ShoppingItemPurchased || item.Status == model.ShoppingItemSkipped {
			continue
		}
		if !item.Confirmed() {
			view.Unconfirmed = append(view.Unconfirmed, toShoppingItemRow(item, paths, names))
			continue
		}
		row := checkoutItemRow{Item: *item}
		if item.AccountID != nil {
			row.AccountID = *item.AccountID
		}
		row.AccountPath = paths[row.AccountID]
		row.Amount = kopInput(item.ActualPrice)
		if row.Amount == "" {
			row.Amount = kopInput(item.PlannedPrice)
		}
		if in != nil {
			if accountID, ok := in.ItemAccounts[item.ID]; ok {
				row.AccountID = accountID
				row.AccountPath = paths[accountID]
			}
			if amount, ok := in.ItemAmounts[item.ID]; ok {
				row.Amount = amount
			}
		}
		if amount, err := parseFormMoney(row.Amount); err == nil {
			row.AmountKop = amount
			view.Total += amount
		}
		view.Items = append(view.Items, row)
	}

	for i := range assetAccounts {
		row := checkoutSourceRow{
			AccountID:   assetAccounts[i].ID,
			AccountPath: paths[assetAccounts[i].ID],
		}
		if in != nil {
			row.Amount = in.SourceAmounts[assetAccounts[i].ID]
		}
		view.Sources = append(view.Sources, row)
	}
	return view, nil
}

// checkoutItemEntries собирает строки расходов по подтверждённым позициям.
func checkoutItemEntries(r *http.Request, list *model.ShoppingList, in *checkoutSubmitted) ([]engine.Entry, money.Kop, error) {
	var entries []engine.Entry
	var total money.Kop
	for i := range list.Items {
		item := &list.Items[i]
		if !checkoutable(item) {
			continue
		}
		key := strconv.FormatInt(item.ID, 10)
		accountRaw := strings.TrimSpace(r.FormValue("item_account_" + key))
		amountRaw := strings.TrimSpace(r.FormValue("item_amount_" + key))
		accountID, _ := strconv.ParseInt(accountRaw, 10, 64)
		in.ItemAccounts[item.ID] = accountID
		in.ItemAmounts[item.ID] = amountRaw
		if accountID <= 0 {
			return nil, 0, errors.New("для позиции «" + item.Name + "» не выбрана статья расходов")
		}
		amount, err := parseFormMoney(amountRaw)
		if err != nil {
			return nil, 0, errors.New("позиция «" + item.Name + "»: " + err.Error())
		}
		if amount.Kopecks() <= 0 {
			return nil, 0, errors.New("для позиции «" + item.Name + "» укажите сумму больше нуля")
		}
		entries = append(entries, engine.Entry{AccountID: accountID, Amount: amount, Role: engine.RoleExpense})
		total += amount
	}
	if len(entries) == 0 {
		return nil, 0, errors.New("в списке нет подтверждённых позиций для оформления")
	}
	return aggregateEntries(entries), total, nil
}

// aggregateEntries складывает строки одного счёта: таблица проводок хранит
// не больше одной строки на счёт и сторону, поэтому несколько позиций с общей
// статьёй расходов дают одну строку с суммарной суммой.
func aggregateEntries(entries []engine.Entry) []engine.Entry {
	merged := make([]engine.Entry, 0, len(entries))
	index := make(map[int64]int, len(entries))
	for _, entry := range entries {
		if pos, ok := index[entry.AccountID]; ok {
			merged[pos].Amount += entry.Amount
			continue
		}
		index[entry.AccountID] = len(merged)
		merged = append(merged, entry)
	}
	return merged
}

// checkoutSourceEntries собирает строки источников и распределяет итог.
//
// Пустые суммы получают остаток: если источников несколько, остаток делится
// между ними поровну, последнему достаются копейки от деления.
func (s *Server) checkoutSourceEntries(ctx context.Context, r *http.Request, total money.Kop, in *checkoutSubmitted) ([]engine.Entry, error) {
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return nil, err
	}
	type source struct {
		id     int64
		amount *money.Kop
	}
	var sources []source
	var filled money.Kop
	for i := range accounts {
		if accounts[i].Type != model.TypeAsset || accounts[i].IsSystem {
			continue
		}
		raw := strings.TrimSpace(r.FormValue("source_amount_" + strconv.FormatInt(accounts[i].ID, 10)))
		in.SourceAmounts[accounts[i].ID] = raw
		if raw == "" {
			sources = append(sources, source{id: accounts[i].ID})
			continue
		}
		amount, err := parseFormMoney(raw)
		if err != nil {
			return nil, errors.New("источник «" + accounts[i].FullPath + "»: " + err.Error())
		}
		if amount.Kopecks() < 0 {
			return nil, errors.New("сумма источника не может быть отрицательной")
		}
		value := amount
		sources = append(sources, source{id: accounts[i].ID, amount: &value})
		filled += amount
	}
	if len(sources) == 0 {
		return nil, errors.New("в книге нет листовых счетов-активов для оплаты")
	}
	if filled > total {
		return nil, errors.New("сумма источников больше итога покупки")
	}

	remaining := total - filled
	var blanks []int
	for i := range sources {
		if sources[i].amount == nil {
			blanks = append(blanks, i)
		}
	}
	if len(blanks) == 0 && remaining != 0 {
		return nil, errors.New("сумма источников не равна итогу покупки")
	}
	switch {
	case len(blanks) == len(sources):
		// Ничего не указано — платим с первого счёта целиком.
		amount := total
		sources[0].amount = &amount
	case len(blanks) > 0:
		// Остаток делим между пустыми источниками, последнему — копейки деления.
		assigned := money.Kop(0)
		for n, idx := range blanks {
			var amount money.Kop
			if n == len(blanks)-1 {
				amount = remaining - assigned
			} else {
				amount = remaining / money.Kop(len(blanks))
			}
			assigned += amount
			value := amount
			sources[idx].amount = &value
		}
	}

	var entries []engine.Entry
	var sum money.Kop
	for _, src := range sources {
		if src.amount == nil || src.amount.Kopecks() <= 0 {
			continue
		}
		entries = append(entries, engine.Entry{AccountID: src.id, Amount: *src.amount, Role: engine.RoleSource})
		sum += *src.amount
	}
	if len(entries) == 0 {
		return nil, errors.New("не указан ни один источник оплаты")
	}
	if sum != total {
		return nil, errors.New("сумма источников не равна итогу покупки")
	}
	return aggregateEntries(entries), nil
}

// checkoutPreview переводит подготовленную проводку в строки предпросмотра.
func (s *Server) checkoutPreview(ctx context.Context, tr *model.Transaction, total money.Kop, names map[int64]string) (*checkoutPreview, error) {
	paths, err := s.accountPaths(ctx)
	if err != nil {
		return nil, err
	}
	preview := &checkoutPreview{
		Date:        tr.Date.Format("02.01.2006"),
		Description: tr.Description,
		Actor:       names[tr.ActorID],
		Total:       total,
	}
	for _, line := range tr.Lines {
		preview.Lines = append(preview.Lines, checkoutLine{
			AccountPath: paths[line.AccountID],
			Side:        sideLabel(line.Side),
			Amount:      line.Amount,
		})
		if line.Side == model.SideDebit {
			preview.DebitTotal += line.Amount
		} else {
			preview.CreditTotal += line.Amount
		}
	}
	return preview, nil
}

// shoppingCheckoutPath собирает адрес экрана оформления покупки.
func shoppingCheckoutPath(id int64) string {
	return shoppingListPath(id) + "/checkout"
}

// shoppingDetailView собирает данные страницы списка.
func (s *Server) shoppingDetailView(ctx context.Context, id int64) (shoppingDetailView, error) {
	list, err := s.db.ShoppingList(ctx, id)
	if err != nil {
		return shoppingDetailView{}, err
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return shoppingDetailView{}, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return shoppingDetailView{}, err
	}
	paths, err := s.accountPaths(ctx)
	if err != nil {
		return shoppingDetailView{}, err
	}
	names := userNames(users)

	view := shoppingDetailView{
		List:       toShoppingListCard(list, names),
		Users:      users,
		Priorities: shoppingPriorities(),
		Statuses:   shoppingListStatuses(),
	}
	for i := range accounts {
		if accounts[i].Type == model.TypeExpense {
			view.Accounts = append(view.Accounts, accounts[i])
		}
	}
	for i := range list.Items {
		row := toShoppingItemRow(&list.Items[i], paths, names)
		row.Accounts = view.Accounts
		row.Users = users
		view.Items = append(view.Items, row)
	}
	return view, nil
}

// shoppingRow собирает строку позиции для ответа JSON.
func (s *Server) shoppingRow(ctx context.Context, item *model.ShoppingListItem) (shoppingItemRow, error) {
	paths, err := s.accountPaths(ctx)
	if err != nil {
		return shoppingItemRow{}, err
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return shoppingItemRow{}, err
	}
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{OnlyLeaves: true})
	if err != nil {
		return shoppingItemRow{}, err
	}
	row := toShoppingItemRow(item, paths, userNames(users))
	row.Users = users
	for i := range accounts {
		if accounts[i].Type == model.TypeExpense {
			row.Accounts = append(row.Accounts, accounts[i])
		}
	}
	return row, nil
}

// renderShoppingRow отрисовывает фрагмент строки позиции.
func (s *Server) renderShoppingRow(row shoppingItemRow) (template.HTML, error) {
	page, ok := s.pages["shopping_detail"]
	if !ok {
		return "", errors.New("httpui: нет шаблона страницы списка покупок")
	}
	var buf bytes.Buffer
	if err := page.ExecuteTemplate(&buf, "shopping-item-row", row); err != nil {
		return "", fmt.Errorf("httpui: не удалось отрисовать строку позиции: %w", err)
	}
	return template.HTML(buf.String()), nil
}

// toShoppingListCard переводит список в данные страницы.
func toShoppingListCard(list *model.ShoppingList, names map[int64]string) shoppingListCard {
	card := shoppingListCard{
		List:        *list,
		Owner:       names[list.OwnerID],
		Status:      shoppingListStatusLabel(list.Status),
		StatusValue: string(list.Status),
		Planned:     "не задана",
	}
	if card.Owner == "" {
		card.Owner = fmt.Sprintf("пользователь #%d", list.OwnerID)
	}
	if !list.PlannedDate.IsZero() {
		card.Planned = list.PlannedDate.Format("02.01.2006")
	}
	return card
}

// toShoppingItemRow переводит позицию в данные страницы.
func toShoppingItemRow(item *model.ShoppingListItem, paths, names map[int64]string) shoppingItemRow {
	row := shoppingItemRow{
		Item:          *item,
		Priority:      shoppingPriorityLabel(item.Priority),
		PriorityValue: string(item.Priority),
		Status:        shoppingItemStatusLabel(item.Status),
		StatusValue:   string(item.Status),
	}
	if item.AccountID != nil {
		row.AccountID = *item.AccountID
		row.Account = paths[*item.AccountID]
	}
	if item.AssigneeID != nil {
		row.AssigneeID = *item.AssigneeID
		row.Assignee = names[*item.AssigneeID]
	}
	return row
}

// shoppingItemInput собирает данные новой позиции из формы.
func (s *Server) shoppingItemInput(r *http.Request) (store.ShoppingItemInput, error) {
	accountID, err := s.shoppingAccountID(r)
	if err != nil {
		return store.ShoppingItemInput{}, err
	}
	assigneeID, err := parseOptionalID(r.FormValue("assignee_id"))
	if err != nil {
		return store.ShoppingItemInput{}, errors.New("неверный ответственный")
	}
	planned, err := parseFormMoney(r.FormValue("planned_price"))
	if err != nil {
		return store.ShoppingItemInput{}, err
	}
	priority := model.ShoppingPriority(strings.TrimSpace(r.FormValue("priority")))
	if priority == "" {
		priority = model.ShoppingPriorityNormal
	}
	return store.ShoppingItemInput{
		Name:         r.FormValue("name"),
		AccountID:    accountID,
		AssigneeID:   assigneeID,
		Priority:     priority,
		PlannedPrice: planned,
		Comment:      r.FormValue("comment"),
	}, nil
}

// shoppingItemPatch собирает частичное изменение позиции из формы.
func (s *Server) shoppingItemPatch(r *http.Request, version int64) (store.ShoppingItemPatch, error) {
	patch := store.ShoppingItemPatch{Version: version}
	name := strings.TrimSpace(r.FormValue("name"))
	patch.Name = &name

	accountID, err := parseOptionalID(r.FormValue("account_id"))
	if err != nil {
		return patch, errors.New("неверная статья расходов")
	}
	if accountID == nil {
		patch.ClearAccount = true
	} else {
		patch.AccountID = accountID
	}

	assigneeID, err := parseOptionalID(r.FormValue("assignee_id"))
	if err != nil {
		return patch, errors.New("неверный ответственный")
	}
	if assigneeID == nil {
		patch.ClearAssignee = true
	} else {
		patch.AssigneeID = assigneeID
	}

	if text := strings.TrimSpace(r.FormValue("priority")); text != "" {
		priority := model.ShoppingPriority(text)
		patch.Priority = &priority
	}
	planned, err := parseFormMoney(r.FormValue("planned_price"))
	if err != nil {
		return patch, err
	}
	patch.PlannedPrice = &planned
	comment := strings.TrimSpace(r.FormValue("comment"))
	patch.Comment = &comment
	return patch, nil
}

// shoppingAccountID определяет статью расходов по id или пути счёта.
func (s *Server) shoppingAccountID(r *http.Request) (*int64, error) {
	if id, err := parseOptionalID(r.FormValue("account_id")); err != nil {
		return nil, errors.New("неверная статья расходов")
	} else if id != nil {
		return id, nil
	}
	path := strings.TrimSpace(r.FormValue("account_path"))
	if path == "" {
		return nil, nil
	}
	account, err := s.db.AccountByPath(r.Context(), path)
	if err != nil {
		return nil, fmt.Errorf("статья расходов не найдена: %s", path)
	}
	id := account.ID
	return &id, nil
}

// shoppingOwner определяет владельца списка из формы.
func (s *Server) shoppingOwner(ctx context.Context, value string) (int64, error) {
	if strings.TrimSpace(value) != "" {
		id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || id <= 0 {
			return 0, errors.New("неверный владелец списка")
		}
		return id, nil
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, errors.New("в книге нет пользователей: создайте пользователя на странице «Пользователи»")
	}
	return users[0].ID, nil
}

// shoppingID читает идентификатор списка из пути.
func (s *Server) shoppingID(w http.ResponseWriter, r *http.Request, back string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.mutationError(w, r, back, "не удалось определить список покупок")
		return 0, false
	}
	return id, true
}

// shoppingItemID читает идентификатор позиции из пути.
func (s *Server) shoppingItemID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		s.mutationError(w, r, "/shopping-lists", "не удалось определить позицию списка")
		return 0, false
	}
	return id, true
}

// shoppingBackPath возвращает адрес списка из формы или общий список.
func (s *Server) shoppingBackPath(r *http.Request) string {
	if id, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("list_id")), 10, 64); err == nil && id > 0 {
		return shoppingListPath(id)
	}
	return "/shopping-lists"
}

// shoppingListPath собирает адрес страницы списка.
func shoppingListPath(id int64) string {
	return "/shopping-lists/" + strconv.FormatInt(id, 10)
}

// shoppingErrorMessage переводит ошибку store в понятное сообщение.
func shoppingErrorMessage(err error) string {
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		return ve.Message
	}
	if errors.Is(err, store.ErrNotFound) {
		return "запись не найдена — обновите страницу"
	}
	return "не удалось сохранить изменения"
}

// shoppingStatusFor выбирает HTTP-код ответа на ошибку store.
func shoppingStatusFor(err error) int {
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		return http.StatusUnprocessableEntity
	}
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// wantsJSON сообщает, ждёт ли клиент JSON вместо переадресации.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// writeShoppingJSON отправляет JSON с заданным кодом ответа.
func writeShoppingJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// userNames строит карту id → имя пользователя.
func userNames(users []model.User) map[int64]string {
	names := make(map[int64]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Name
	}
	return names
}

// parseFormDate разбирает дату формы; пустое значение — нулевое время.
func parseFormDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	date, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, errors.New("неверная дата: ожидается ГГГГ-ММ-ДД")
	}
	return date, nil
}

// parseFormMoney разбирает сумму формы; пустое значение — ноль.
func parseFormMoney(value string) (money.Kop, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	amount, err := money.Parse(value)
	if err != nil {
		return 0, errors.New("неверная сумма: ожидается число, например «1 234,56»")
	}
	return amount, nil
}

// parseOptionalID разбирает необязательный идентификатор.
func parseOptionalID(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New("неверный идентификатор")
	}
	return &id, nil
}
