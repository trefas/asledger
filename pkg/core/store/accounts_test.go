package store

import (
	"context"
	"errors"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// TestASL_5_CreateAccountTree проверяет построение дерева и полных путей.
func TestASL_5_CreateAccountTree(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	if root.FullPath != "Имущество" || !root.IsLeaf {
		t.Fatalf("корень: path=%q leaf=%v", root.FullPath, root.IsLeaf)
	}

	child := mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)
	if child.FullPath != "Имущество:Техника" || child.ParentID == nil || *child.ParentID != root.ID {
		t.Fatalf("потомок: path=%q parent=%v", child.FullPath, child.ParentID)
	}

	grand := mustCreateAccount(t, db, "Ноутбук", model.TypeAsset, &child.ID)
	if grand.FullPath != "Имущество:Техника:Ноутбук" {
		t.Fatalf("внук: path=%q", grand.FullPath)
	}

	// После появления потомков корень и середина перестают быть листьями.
	reloaded, err := db.Account(ctx, root.ID)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if reloaded.IsLeaf {
		t.Fatal("корень с потомками помечен листом")
	}
	mid, err := db.Account(ctx, child.ID)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if mid.IsLeaf {
		t.Fatal("счёт с потомком помечен листом")
	}
	leaf, err := db.Account(ctx, grand.ID)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if !leaf.IsLeaf {
		t.Fatal("счёт без потомков не помечен листом")
	}
}

// TestASL_5_AccountPathUnique проверяет запрет повторного полного пути.
func TestASL_5_AccountPathUnique(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)

	_, err := db.CreateAccount(ctx, AccountInput{Name: "Техника", Type: model.TypeAsset, ParentID: &root.ID})
	if !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("повторный путь: err=%v, ожидалась ErrDuplicatePath", err)
	}
}

// TestASL_5_AccountNameRules проверяет недопустимые имена счёта.
func TestASL_5_AccountNameRules(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	cases := map[string]string{
		"пустое имя":  "   ",
		"разделитель": "Активы:Наличные",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := db.CreateAccount(ctx, AccountInput{Name: input, Type: model.TypeAsset}); err == nil {
				t.Fatalf("имя %q принято, хотя должно быть отклонено", input)
			}
		})
	}

	t.Run("недопустимый тип", func(t *testing.T) {
		if _, err := db.CreateAccount(ctx, AccountInput{Name: "Странный", Type: model.AccountType("вымысел")}); err == nil {
			t.Fatal("недопустимый тип счёта принят")
		}
	})
}

// TestASL_5_RenameAccountRecurses проверяет пересчёт путей потомков при переименовании.
func TestASL_5_RenameAccountRecurses(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	child := mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)
	grand := mustCreateAccount(t, db, "Ноутбук", model.TypeAsset, &child.ID)

	if err := db.RenameAccount(ctx, root.ID, "Собственность"); err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}

	for id, want := range map[int64]string{
		root.ID:  "Собственность",
		child.ID: "Собственность:Техника",
		grand.ID: "Собственность:Техника:Ноутбук",
	} {
		acc, err := db.Account(ctx, id)
		if err != nil {
			t.Fatalf("Account(%d): %v", id, err)
		}
		if acc.FullPath != want {
			t.Fatalf("счёт %d: path=%q, ожидалось %q", id, acc.FullPath, want)
		}
	}
}

// TestASL_5_MoveAccountRecurses проверяет пересчёт путей при перемещении ветки.
func TestASL_5_MoveAccountRecurses(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	child := mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)
	grand := mustCreateAccount(t, db, "Ноутбук", model.TypeAsset, &child.ID)

	if err := db.MoveAccount(ctx, child.ID, nil); err != nil {
		t.Fatalf("MoveAccount: %v", err)
	}

	child2, err := db.Account(ctx, child.ID)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if child2.FullPath != "Техника" || child2.ParentID != nil {
		t.Fatalf("перемещённый счёт: path=%q parent=%v", child2.FullPath, child2.ParentID)
	}
	grand2, err := db.Account(ctx, grand.ID)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if grand2.FullPath != "Техника:Ноутбук" {
		t.Fatalf("внук после перемещения: path=%q", grand2.FullPath)
	}
	// Корень остался без потомков и снова стал листом.
	root2, err := db.Account(ctx, root.ID)
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if !root2.IsLeaf {
		t.Fatal("корень после перемещения ветки не стал листом")
	}
}

// TestASL_5_MoveAccountIntoDescendantRejected проверяет запрет перемещения в потомка.
func TestASL_5_MoveAccountIntoDescendantRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	child := mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)

	if err := db.MoveAccount(ctx, root.ID, &child.ID); !errors.Is(err, ErrMoveIntoDescendant) {
		t.Fatalf("перемещение в потомка: err=%v, ожидалась ErrMoveIntoDescendant", err)
	}
	if err := db.MoveAccount(ctx, root.ID, &root.ID); !errors.Is(err, ErrMoveIntoDescendant) {
		t.Fatalf("перемещение в себя: err=%v, ожидалась ErrMoveIntoDescendant", err)
	}
}

// TestASL_5_ListAccountsFilters проверяет фильтры выборки счетов.
func TestASL_5_ListAccountsFilters(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	child := mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)
	mustCreateAccount(t, db, "Ноутбук", model.TypeAsset, &child.ID)
	other := mustCreateAccount(t, db, "Другое", model.TypeExpense, nil)

	subtree, err := db.ListAccounts(ctx, AccountFilter{Prefix: "Имущество"})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(subtree) != 3 {
		t.Fatalf("поддерево: %d счетов, ожидалось 3", len(subtree))
	}

	leaves, err := db.ListAccounts(ctx, AccountFilter{Prefix: "Имущество", OnlyLeaves: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(leaves) != 1 || leaves[0].FullPath != "Имущество:Техника:Ноутбук" {
		t.Fatalf("листья поддерева: %+v", leaves)
	}

	if err := db.ArchiveAccount(ctx, other.ID); err != nil {
		t.Fatalf("ArchiveAccount: %v", err)
	}
	active, err := db.ListAccounts(ctx, AccountFilter{Prefix: "Другое"})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("заархивированный счёт попал в выборку: %+v", active)
	}
	withArchived, err := db.ListAccounts(ctx, AccountFilter{Prefix: "Другое", IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(withArchived) != 1 {
		t.Fatalf("выборка с архивом: %d счетов, ожидался 1", len(withArchived))
	}
}

// TestASL_5_SystemAccountsCreated проверяет автосид служебных счетов.
func TestASL_5_SystemAccountsCreated(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := []string{
		"Капитал:Начальные",
		"Капитал:Корректировка",
		"Капитал:Переоценка",
		"Активы:Дебиторка",
	}
	for _, path := range want {
		acc, err := db.AccountByPath(ctx, path)
		if err != nil {
			t.Fatalf("служебный счёт %q не создан: %v", path, err)
		}
		if !acc.IsSystem {
			t.Fatalf("счёт %q не помечен служебным", path)
		}
		if !acc.IsLeaf {
			t.Fatalf("служебный счёт %q должен быть листом", path)
		}
	}

	// Повторный вызов не должен создавать дубликаты.
	if err := db.EnsureSystemAccounts(ctx); err != nil {
		t.Fatalf("повторный EnsureSystemAccounts: %v", err)
	}
	for _, path := range want {
		accounts, err := db.ListAccounts(ctx, AccountFilter{Prefix: path})
		if err != nil {
			t.Fatalf("ListAccounts: %v", err)
		}
		if len(accounts) != 1 {
			t.Fatalf("после повторного сида счёт %q встречается %d раз", path, len(accounts))
		}
	}
}

// TestASL_5_SystemAccountProtected проверяет защиту служебных счетов.
func TestASL_5_SystemAccountProtected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	acc, err := db.AccountByPath(ctx, "Капитал:Начальные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}

	if err := db.RenameAccount(ctx, acc.ID, "Другое имя"); !errors.Is(err, ErrSystemAccount) {
		t.Fatalf("переименование служебного счёта: err=%v, ожидалась ErrSystemAccount", err)
	}
	if err := db.SetMinBalance(ctx, acc.ID, -100); !errors.Is(err, ErrSystemAccount) {
		t.Fatalf("изменение служебного счёта: err=%v, ожидалась ErrSystemAccount", err)
	}
	if err := db.ArchiveAccount(ctx, acc.ID); !errors.Is(err, ErrSystemAccount) {
		t.Fatalf("архивирование служебного счёта: err=%v, ожидалась ErrSystemAccount", err)
	}
}

// TestASL_5_ArchiveAccountRules проверяет правила архивирования счёта.
func TestASL_5_ArchiveAccountRules(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	actor := mustCreateUser(t, db, "Андрей")

	root := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	child := mustCreateAccount(t, db, "Техника", model.TypeAsset, &root.ID)
	empty := mustCreateAccount(t, db, "Пустой", model.TypeAsset, nil)

	t.Run("с потомками", func(t *testing.T) {
		if err := db.ArchiveAccount(ctx, root.ID); !errors.Is(err, ErrAccountHasChildren) {
			t.Fatalf("err=%v, ожидалась ErrAccountHasChildren", err)
		}
	})

	t.Run("с проводками", func(t *testing.T) {
		credit, err := db.AccountByPath(ctx, "Капитал:Начальные")
		if err != nil {
			t.Fatalf("AccountByPath: %v", err)
		}
		mustInsertTransaction(t, db, initialTransaction("архив-1", actor, child.ID, credit.ID))
		if err := db.ArchiveAccount(ctx, child.ID); !errors.Is(err, ErrAccountHasOperations) {
			t.Fatalf("err=%v, ожидалась ErrAccountHasOperations", err)
		}
	})

	t.Run("пустой лист", func(t *testing.T) {
		if err := db.ArchiveAccount(ctx, empty.ID); err != nil {
			t.Fatalf("архивирование пустого листа: %v", err)
		}
		acc, err := db.Account(ctx, empty.ID)
		if err != nil {
			t.Fatalf("Account: %v", err)
		}
		if !acc.IsArchived {
			t.Fatal("счёт не помечен заархивированным")
		}
	})
}
