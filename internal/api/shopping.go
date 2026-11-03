package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

// shoppingListDTO — список покупок в ответе API.
type shoppingListDTO struct {
	ID             int64                    `json:"id"`
	UUID           string                   `json:"uuid"`
	Name           string                   `json:"name"`
	PlannedDate    *time.Time               `json:"planned_date,omitempty"`
	Status         model.ShoppingListStatus `json:"status"`
	OwnerID        int64                    `json:"owner_id"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	ArchivedAt     *time.Time               `json:"archived_at,omitempty"`
	ItemCount      int                      `json:"item_count"`
	ConfirmedCount int                      `json:"confirmed_count"`
	ConfirmedTotal money.Kop                `json:"confirmed_total_kop"`
	Items          []shoppingItemDTO        `json:"items,omitempty"`
}

// shoppingItemDTO — позиция списка покупок в ответе API.
type shoppingItemDTO struct {
	ID            int64                    `json:"id"`
	UUID          string                   `json:"uuid"`
	ListID        int64                    `json:"list_id"`
	Name          string                   `json:"name"`
	AccountID     *int64                   `json:"account_id,omitempty"`
	AssigneeID    *int64                   `json:"assignee_id,omitempty"`
	Priority      model.ShoppingPriority   `json:"priority"`
	PlannedPrice  money.Kop                `json:"planned_price_kop"`
	ActualPrice   money.Kop                `json:"actual_price_kop"`
	InCart        bool                     `json:"in_cart"`
	PriceMismatch bool                     `json:"price_mismatch"`
	Comment       string                   `json:"comment,omitempty"`
	Status        model.ShoppingItemStatus `json:"status"`
	Position      int                      `json:"position"`
	Version       int64                    `json:"version"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
	// Confirmed — позиция подтверждена к покупке (цена или «в корзине»).
	Confirmed bool `json:"confirmed"`
}

// shoppingListRequest — тело создания списка.
type shoppingListRequest struct {
	Name        string                   `json:"name"`
	PlannedDate *time.Time               `json:"planned_date"`
	Status      model.ShoppingListStatus `json:"status"`
	OwnerID     int64                    `json:"owner_id"`
	// TemplateID — создать список из шаблона; позиции копируются из него.
	TemplateID int64 `json:"template_id"`
}

// shoppingListPatchRequest — тело частичного изменения списка.
type shoppingListPatchRequest struct {
	Name        *string                   `json:"name"`
	PlannedDate *time.Time                `json:"planned_date"`
	Status      *model.ShoppingListStatus `json:"status"`
	OwnerID     *int64                    `json:"owner_id"`
}

// shoppingItemRequest — тело создания позиции.
type shoppingItemRequest struct {
	UUID          string                   `json:"uuid"`
	Name          string                   `json:"name"`
	AccountID     *int64                   `json:"account_id"`
	AssigneeID    *int64                   `json:"assignee_id"`
	Priority      model.ShoppingPriority   `json:"priority"`
	PlannedPrice  money.Kop                `json:"planned_price_kop"`
	ActualPrice   money.Kop                `json:"actual_price_kop"`
	InCart        bool                     `json:"in_cart"`
	PriceMismatch bool                     `json:"price_mismatch"`
	Comment       string                   `json:"comment"`
	Status        model.ShoppingItemStatus `json:"status"`
	Position      int                      `json:"position"`
}

// shoppingItemPatchRequest — тело частичного изменения позиции.
type shoppingItemPatchRequest struct {
	Version       int64                     `json:"version"`
	Name          *string                   `json:"name"`
	AccountID     *int64                    `json:"account_id"`
	ClearAccount  bool                      `json:"clear_account"`
	AssigneeID    *int64                    `json:"assignee_id"`
	ClearAssignee bool                      `json:"clear_assignee"`
	Priority      *model.ShoppingPriority   `json:"priority"`
	PlannedPrice  *money.Kop                `json:"planned_price_kop"`
	ActualPrice   *money.Kop                `json:"actual_price_kop"`
	InCart        *bool                     `json:"in_cart"`
	PriceMismatch *bool                     `json:"price_mismatch"`
	Comment       *string                   `json:"comment"`
	Status        *model.ShoppingItemStatus `json:"status"`
	Position      *int                      `json:"position"`
}

// shoppingTemplateDTO — шаблон списка покупок в ответе API.
type shoppingTemplateDTO struct {
	ID        int64                     `json:"id"`
	UUID      string                    `json:"uuid"`
	Name      string                    `json:"name"`
	CreatedAt time.Time                 `json:"created_at"`
	UpdatedAt time.Time                 `json:"updated_at"`
	Items     []shoppingTemplateItemDTO `json:"items,omitempty"`
}

// shoppingTemplateItemDTO — базовая позиция шаблона в ответе API.
type shoppingTemplateItemDTO struct {
	ID         int64                  `json:"id"`
	TemplateID int64                  `json:"template_id"`
	Name       string                 `json:"name"`
	AccountID  *int64                 `json:"account_id,omitempty"`
	Priority   model.ShoppingPriority `json:"priority"`
	Position   int                    `json:"position"`
}

// shoppingTemplateItemRequest — базовая позиция шаблона в запросе.
type shoppingTemplateItemRequest struct {
	Name      string                 `json:"name"`
	AccountID *int64                 `json:"account_id"`
	Priority  model.ShoppingPriority `json:"priority"`
	Position  int                    `json:"position"`
}

// shoppingTemplateRequest — тело создания шаблона.
type shoppingTemplateRequest struct {
	Name  string                        `json:"name"`
	Items []shoppingTemplateItemRequest `json:"items"`
}

// shoppingTemplatePatchRequest — тело частичного изменения шаблона.
type shoppingTemplatePatchRequest struct {
	Name  *string                        `json:"name"`
	Items *[]shoppingTemplateItemRequest `json:"items"`
}

// handleShoppingLists отдаёт список списков или создаёт новый.
func (s *Server) handleShoppingLists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listShoppingLists(w, r)
	case http.MethodPost:
		s.createShoppingList(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "метод не поддерживается")
	}
}

// listShoppingLists отдаёт списки покупок с фильтрами.
func (s *Server) listShoppingLists(w http.ResponseWriter, r *http.Request) {
	filter := store.ShoppingListFilter{IncludeArchived: queryBool(r, "include_archived")}
	if text := r.URL.Query().Get("status"); text != "" {
		status := model.ShoppingListStatus(text)
		if !status.Valid() {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "недопустимый статус списка")
			return
		}
		filter.Status = &status
	}
	if text := r.URL.Query().Get("owner_id"); text != "" {
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "owner_id должен быть целым числом")
			return
		}
		filter.OwnerID = &id
	}
	limit, err := intQuery(r, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "limit должен быть целым числом")
		return
	}
	offset, err := intQuery(r, "offset")
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "offset должен быть целым числом")
		return
	}
	filter.Limit = int(limit)
	filter.Offset = int(offset)

	lists, err := s.db.ListShoppingLists(r.Context(), filter)
	if err != nil {
		s.shoppingError(w, err)
		return
	}
	items := make([]shoppingListDTO, 0, len(lists))
	for i := range lists {
		items = append(items, toShoppingListDTO(&lists[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// createShoppingList создаёт список покупок.
func (s *Server) createShoppingList(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())
	var req shoppingListRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	ownerID := req.OwnerID
	if ownerID == 0 {
		ownerID = device.UserID
	}
	var planned time.Time
	if req.PlannedDate != nil {
		planned = *req.PlannedDate
	}
	if req.TemplateID > 0 {
		list, err := s.db.CreateShoppingListFromTemplate(r.Context(), req.TemplateID, req.Name, ownerID, planned)
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toShoppingListDTO(list))
		return
	}
	list, err := s.db.CreateShoppingList(r.Context(), store.ShoppingListInput{
		Name: req.Name, PlannedDate: planned, Status: req.Status, OwnerID: ownerID,
	})
	if err != nil {
		s.shoppingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toShoppingListDTO(list))
}

// handleShoppingList отдаёт, изменяет или архивирует список.
func (s *Server) handleShoppingList(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "неверный идентификатор списка")
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.db.ShoppingList(r.Context(), id)
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toShoppingListDTO(list))
	case http.MethodPatch:
		var req shoppingListPatchRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
			return
		}
		list, err := s.db.UpdateShoppingList(r.Context(), id, store.ShoppingListPatch{
			Name: req.Name, PlannedDate: req.PlannedDate,
			Status: req.Status, OwnerID: req.OwnerID,
		})
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toShoppingListDTO(list))
	case http.MethodDelete:
		if err := s.db.ArchiveShoppingList(r.Context(), id); err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "archived", "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "метод не поддерживается")
	}
}

// handleShoppingListItems добавляет позицию в список.
func (s *Server) handleShoppingListItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "метод не поддерживается")
		return
	}
	listID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || listID <= 0 {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "неверный идентификатор списка")
		return
	}
	var req shoppingItemRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	item, err := s.db.AddShoppingItem(r.Context(), listID, store.ShoppingItemInput{
		UUID: req.UUID, Name: req.Name, AccountID: req.AccountID,
		AssigneeID: req.AssigneeID, Priority: req.Priority,
		PlannedPrice: req.PlannedPrice, ActualPrice: req.ActualPrice,
		InCart: req.InCart, PriceMismatch: req.PriceMismatch,
		Comment: req.Comment, Status: req.Status, Position: req.Position,
	})
	if err != nil {
		s.shoppingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toShoppingItemDTO(item))
}

// handleShoppingItem изменяет или удаляет позицию.
func (s *Server) handleShoppingItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "неверный идентификатор позиции")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req shoppingItemPatchRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
			return
		}
		if req.Version <= 0 {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не задана версия позиции")
			return
		}
		item, err := s.db.UpdateShoppingItem(r.Context(), id, store.ShoppingItemPatch{
			Version: req.Version, Name: req.Name,
			AccountID: req.AccountID, ClearAccount: req.ClearAccount,
			AssigneeID: req.AssigneeID, ClearAssignee: req.ClearAssignee,
			Priority: req.Priority, PlannedPrice: req.PlannedPrice,
			ActualPrice: req.ActualPrice, InCart: req.InCart,
			PriceMismatch: req.PriceMismatch, Comment: req.Comment,
			Status: req.Status, Position: req.Position,
		})
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toShoppingItemDTO(item))
	case http.MethodDelete:
		if err := s.db.DeleteShoppingItem(r.Context(), id); err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "метод не поддерживается")
	}
}

// handleShoppingTemplates отдаёт список шаблонов или создаёт новый.
func (s *Server) handleShoppingTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := s.db.ListShoppingTemplatesFull(r.Context())
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		items := make([]shoppingTemplateDTO, 0, len(templates))
		for i := range templates {
			items = append(items, toShoppingTemplateDTO(&templates[i]))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req shoppingTemplateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
			return
		}
		tpl, err := s.db.CreateShoppingTemplate(r.Context(), req.Name, toShoppingTemplateItemInputs(req.Items))
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toShoppingTemplateDTO(tpl))
	default:
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "метод не поддерживается")
	}
}

// handleShoppingTemplate отдаёт, изменяет или удаляет шаблон.
func (s *Server) handleShoppingTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "неверный идентификатор шаблона")
		return
	}
	switch r.Method {
	case http.MethodGet:
		tpl, err := s.db.ShoppingTemplate(r.Context(), id)
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toShoppingTemplateDTO(tpl))
	case http.MethodPatch:
		var req shoppingTemplatePatchRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
			return
		}
		patch := store.ShoppingTemplatePatch{Name: req.Name}
		if req.Items != nil {
			inputs := toShoppingTemplateItemInputs(*req.Items)
			patch.Items = &inputs
		}
		tpl, err := s.db.UpdateShoppingTemplate(r.Context(), id, patch)
		if err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toShoppingTemplateDTO(tpl))
	case http.MethodDelete:
		if err := s.db.DeleteShoppingTemplate(r.Context(), id); err != nil {
			s.shoppingError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted", "id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "ERR_METHOD_NOT_ALLOWED", "метод не поддерживается")
	}
}

// toShoppingTemplateItemInputs переводит позиции запроса в входные данные store.
func toShoppingTemplateItemInputs(items []shoppingTemplateItemRequest) []store.ShoppingTemplateItemInput {
	inputs := make([]store.ShoppingTemplateItemInput, 0, len(items))
	for _, item := range items {
		inputs = append(inputs, store.ShoppingTemplateItemInput{
			Name: item.Name, AccountID: item.AccountID,
			Priority: item.Priority, Position: item.Position,
		})
	}
	return inputs
}

// toShoppingTemplateDTO переводит шаблон в формат ответа.
func toShoppingTemplateDTO(tpl *model.ShoppingListTemplate) shoppingTemplateDTO {
	dto := shoppingTemplateDTO{
		ID: tpl.ID, UUID: tpl.UUID, Name: tpl.Name,
		CreatedAt: tpl.CreatedAt.UTC(), UpdatedAt: tpl.UpdatedAt.UTC(),
	}
	if tpl.Items != nil {
		dto.Items = make([]shoppingTemplateItemDTO, 0, len(tpl.Items))
		for i := range tpl.Items {
			item := &tpl.Items[i]
			dto.Items = append(dto.Items, shoppingTemplateItemDTO{
				ID: item.ID, TemplateID: item.TemplateID, Name: item.Name,
				AccountID: item.AccountID, Priority: item.Priority, Position: item.Position,
			})
		}
	}
	return dto
}

// shoppingError переводит ошибку store в ответ API.
func (s *Server) shoppingError(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		status := http.StatusUnprocessableEntity
		if ve.Code == store.CodeShoppingVersionConflict {
			status = http.StatusConflict
		}
		writeError(w, status, ve.Code, ve.Message)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "ERR_NOT_FOUND", "запись не найдена")
		return
	}
	s.logger.Error("ошибка списка покупок", "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}

// toShoppingListDTO переводит список в формат ответа.
func toShoppingListDTO(list *model.ShoppingList) shoppingListDTO {
	dto := shoppingListDTO{
		ID: list.ID, UUID: list.UUID, Name: list.Name,
		Status: list.Status, OwnerID: list.OwnerID,
		CreatedAt: list.CreatedAt.UTC(), UpdatedAt: list.UpdatedAt.UTC(),
		ArchivedAt: list.ArchivedAt, ItemCount: list.ItemCount,
		ConfirmedCount: list.ConfirmedCount, ConfirmedTotal: list.ConfirmedTotal,
	}
	if !list.PlannedDate.IsZero() {
		t := list.PlannedDate.UTC()
		dto.PlannedDate = &t
	}
	if list.Items != nil {
		dto.Items = make([]shoppingItemDTO, 0, len(list.Items))
		for i := range list.Items {
			dto.Items = append(dto.Items, toShoppingItemDTO(&list.Items[i]))
		}
	}
	return dto
}

// toShoppingItemDTO переводит позицию в формат ответа.
func toShoppingItemDTO(item *model.ShoppingListItem) shoppingItemDTO {
	return shoppingItemDTO{
		ID: item.ID, UUID: item.UUID, ListID: item.ListID, Name: item.Name,
		AccountID: item.AccountID, AssigneeID: item.AssigneeID,
		Priority: item.Priority, PlannedPrice: item.PlannedPrice,
		ActualPrice: item.ActualPrice, InCart: item.InCart,
		PriceMismatch: item.PriceMismatch, Comment: item.Comment,
		Status: item.Status, Position: item.Position, Version: item.Version,
		CreatedAt: item.CreatedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(),
		Confirmed: item.Confirmed(),
	}
}
