package model

import (
	"time"

	"github.com/trefas/asledger/pkg/core/money"
)

// ShoppingListStatus — статус списка покупок.
type ShoppingListStatus string

// Допустимые статусы списка покупок.
const (
	// ShoppingListDraft — черновик, список ещё формируется.
	ShoppingListDraft ShoppingListStatus = "draft"
	// ShoppingListActive — список в работе, можно идти в магазин.
	ShoppingListActive ShoppingListStatus = "active"
	// ShoppingListShopping — пользователь находится в магазине.
	ShoppingListShopping ShoppingListStatus = "shopping"
	// ShoppingListCompleted — покупка оформлена, список завершён.
	ShoppingListCompleted ShoppingListStatus = "completed"
	// ShoppingListArchived — список в архиве.
	ShoppingListArchived ShoppingListStatus = "archived"
)

// ShoppingListStatuses возвращает все допустимые статусы списка.
func ShoppingListStatuses() []ShoppingListStatus {
	return []ShoppingListStatus{
		ShoppingListDraft, ShoppingListActive, ShoppingListShopping,
		ShoppingListCompleted, ShoppingListArchived,
	}
}

// Valid сообщает, является ли статус списка допустимым.
func (s ShoppingListStatus) Valid() bool {
	switch s {
	case ShoppingListDraft, ShoppingListActive, ShoppingListShopping,
		ShoppingListCompleted, ShoppingListArchived:
		return true
	default:
		return false
	}
}

// ShoppingItemStatus — статус позиции списка покупок.
type ShoppingItemStatus string

// Допустимые статусы позиции.
const (
	// ShoppingItemPlanned — позиция запланирована, ещё не подтверждена.
	ShoppingItemPlanned ShoppingItemStatus = "planned"
	// ShoppingItemConfirmed — позиция подтверждена к покупке (цена или «в корзине»).
	ShoppingItemConfirmed ShoppingItemStatus = "confirmed"
	// ShoppingItemPurchased — позиция куплена и проведена в книге.
	ShoppingItemPurchased ShoppingItemStatus = "purchased"
	// ShoppingItemSkipped — позиция пропущена (нет в магазине).
	ShoppingItemSkipped ShoppingItemStatus = "skipped"
)

// ShoppingItemStatuses возвращает все допустимые статусы позиции.
func ShoppingItemStatuses() []ShoppingItemStatus {
	return []ShoppingItemStatus{
		ShoppingItemPlanned, ShoppingItemConfirmed,
		ShoppingItemPurchased, ShoppingItemSkipped,
	}
}

// Valid сообщает, является ли статус позиции допустимым.
func (s ShoppingItemStatus) Valid() bool {
	switch s {
	case ShoppingItemPlanned, ShoppingItemConfirmed,
		ShoppingItemPurchased, ShoppingItemSkipped:
		return true
	default:
		return false
	}
}

// Confirmed сообщает, подтверждена ли позиция к покупке.
//
// Позиция подтверждена, если указана фактическая цена или стоит отметка
// «в корзине» — оба варианта равнозначны для включения в проводку.
func (i ShoppingListItem) Confirmed() bool {
	return i.ActualPrice.Kopecks() > 0 || i.InCart
}

// ShoppingPriority — приоритет/группа позиции списка.
type ShoppingPriority string

// Допустимые приоритеты позиции.
const (
	// ShoppingPriorityLow — низкий приоритет.
	ShoppingPriorityLow ShoppingPriority = "low"
	// ShoppingPriorityNormal — обычный приоритет.
	ShoppingPriorityNormal ShoppingPriority = "normal"
	// ShoppingPriorityHigh — высокий приоритет.
	ShoppingPriorityHigh ShoppingPriority = "high"
)

// ShoppingPriorities возвращает все допустимые приоритеты.
func ShoppingPriorities() []ShoppingPriority {
	return []ShoppingPriority{
		ShoppingPriorityLow, ShoppingPriorityNormal, ShoppingPriorityHigh,
	}
}

// Valid сообщает, является ли приоритет допустимым.
func (p ShoppingPriority) Valid() bool {
	switch p {
	case ShoppingPriorityLow, ShoppingPriorityNormal, ShoppingPriorityHigh:
		return true
	default:
		return false
	}
}

// ShoppingList — список покупок.
type ShoppingList struct {
	ID          int64              `json:"id"`
	UUID        string             `json:"uuid"`
	Name        string             `json:"name"`
	PlannedDate time.Time          `json:"planned_date"`
	Status      ShoppingListStatus `json:"status"`
	OwnerID     int64              `json:"owner_id"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	ArchivedAt  *time.Time         `json:"archived_at,omitempty"`
	// Items — позиции списка; заполняются при чтении одного списка.
	Items []ShoppingListItem `json:"items,omitempty"`
	// ItemCount — число позиций (для сводки списка).
	ItemCount int `json:"item_count,omitempty"`
	// ConfirmedCount — число подтверждённых позиций.
	ConfirmedCount int `json:"confirmed_count,omitempty"`
	// ConfirmedTotal — сумма подтверждённых позиций.
	ConfirmedTotal money.Kop `json:"confirmed_total_kop,omitempty"`
}

// ShoppingListItem — позиция списка покупок.
type ShoppingListItem struct {
	ID            int64              `json:"id"`
	UUID          string             `json:"uuid"`
	ListID        int64              `json:"list_id"`
	Name          string             `json:"name"`
	AccountID     *int64             `json:"account_id,omitempty"`
	AssigneeID    *int64             `json:"assignee_id,omitempty"`
	Priority      ShoppingPriority   `json:"priority"`
	PlannedPrice  money.Kop          `json:"planned_price_kop"`
	ActualPrice   money.Kop          `json:"actual_price_kop"`
	InCart        bool               `json:"in_cart"`
	PriceMismatch bool               `json:"price_mismatch"`
	Comment       string             `json:"comment,omitempty"`
	Status        ShoppingItemStatus `json:"status"`
	Position      int                `json:"position"`
	Version       int64              `json:"version"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// ShoppingListTemplate — шаблон списка покупок.
//
// В шаблон попадают только базовые позиции: наименование, статья расходов и
// приоритет. Цены, ответственные и статусы в шаблон не копируются.
type ShoppingListTemplate struct {
	ID        int64                  `json:"id"`
	UUID      string                 `json:"uuid"`
	Name      string                 `json:"name"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Items     []ShoppingTemplateItem `json:"items,omitempty"`
}

// ShoppingTemplateItem — базовая позиция шаблона списка.
type ShoppingTemplateItem struct {
	ID         int64            `json:"id"`
	TemplateID int64            `json:"template_id"`
	Name       string           `json:"name"`
	AccountID  *int64           `json:"account_id,omitempty"`
	Priority   ShoppingPriority `json:"priority"`
	Position   int              `json:"position"`
}

// Сущности журнала изменений для списков покупок.
const (
	// EntityShoppingList — список покупок.
	EntityShoppingList = "shopping_lists"
	// EntityShoppingItem — позиция списка покупок.
	EntityShoppingItem = "shopping_items"
	// EntityShoppingTemplate — шаблон списка покупок.
	EntityShoppingTemplate = "shopping_list_templates"
)
