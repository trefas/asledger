package model

import (
	"testing"

	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_15_ShoppingStatuses проверяет допустимые статусы списков и позиций.
func TestASL_15_ShoppingStatuses(t *testing.T) {
	listStatuses := ShoppingListStatuses()
	if len(listStatuses) != 5 {
		t.Fatalf("статусов списка = %d", len(listStatuses))
	}
	for _, status := range listStatuses {
		if !status.Valid() {
			t.Fatalf("статус списка %q должен быть допустим", status)
		}
	}
	if ShoppingListStatus("unknown").Valid() {
		t.Fatal("неизвестный статус списка признан допустимым")
	}

	itemStatuses := ShoppingItemStatuses()
	if len(itemStatuses) != 4 {
		t.Fatalf("статусов позиции = %d", len(itemStatuses))
	}
	for _, status := range itemStatuses {
		if !status.Valid() {
			t.Fatalf("статус позиции %q должен быть допустим", status)
		}
	}
	if ShoppingItemStatus("unknown").Valid() {
		t.Fatal("неизвестный статус позиции признан допустимым")
	}
}

// TestASL_15_ShoppingPriorities проверяет приоритеты позиций.
func TestASL_15_ShoppingPriorities(t *testing.T) {
	priorities := ShoppingPriorities()
	if len(priorities) != 3 {
		t.Fatalf("приоритетов = %d", len(priorities))
	}
	for _, priority := range priorities {
		if !priority.Valid() {
			t.Fatalf("приоритет %q должен быть допустим", priority)
		}
	}
	if ShoppingPriority("urgent").Valid() {
		t.Fatal("неизвестный приоритет признан допустимым")
	}
}

// TestASL_15_ShoppingConfirmed проверяет подтверждение позиции ценой или
// отметкой «в корзине».
func TestASL_15_ShoppingConfirmed(t *testing.T) {
	cases := []struct {
		name string
		item ShoppingListItem
		want bool
	}{
		{name: "запланирована", item: ShoppingListItem{}},
		{name: "цена", item: ShoppingListItem{ActualPrice: money.FromKopecks(1)}, want: true},
		{name: "в корзине", item: ShoppingListItem{InCart: true}, want: true},
		{name: "цена и корзина", item: ShoppingListItem{ActualPrice: money.FromKopecks(100), InCart: true}, want: true},
	}
	for _, tc := range cases {
		if got := tc.item.Confirmed(); got != tc.want {
			t.Fatalf("%s: Confirmed() = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}
