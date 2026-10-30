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

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
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
