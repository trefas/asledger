package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// testImportAccounts — цепочка счетов для проверки импорта.
func testImportAccounts() []AccountSpec {
	return []AccountSpec{
		{Path: "Расходы", Type: model.TypeExpense},
		{Path: "Расходы:Продукты", Type: model.TypeExpense},
		{Path: "Расходы:Продукты:Молочные", Type: model.TypeExpense},
	}
}

// TestASL_8_EnsureAccountsHierarchy проверяет создание цепочки счетов импорта.
func TestASL_8_EnsureAccountsHierarchy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	before, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}

	byPath, created, err := db.EnsureAccounts(ctx, testImportAccounts())
	if err != nil {
		t.Fatalf("EnsureAccounts: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("создано счетов %d, ожидалось 3", len(created))
	}
	// В карте — все счета книги: и существующие служебные, и импортированные.
	if len(byPath) != len(created)+6 {
		t.Fatalf("в карте счетов %d записей, ожидалось %d", len(byPath), len(created)+6)
	}
	for _, spec := range testImportAccounts() {
		account, ok := byPath[spec.Path]
		if !ok {
			t.Fatalf("счёт %q отсутствует в карте", spec.Path)
		}
		if account.Type != spec.Type {
			t.Errorf("тип счёта %q равен %q, ожидался %q", spec.Path, account.Type, spec.Type)
		}
		if account.IsSystem {
			t.Errorf("счёт импорта %q помечен служебным", spec.Path)
		}
	}

	// Промежуточные счета перестают быть листьями, последний остаётся листом.
	middle, err := db.AccountByPath(ctx, "Расходы:Продукты")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if middle.IsLeaf {
		t.Error("счёт с потомком помечен листом")
	}
	leaf, err := db.AccountByPath(ctx, "Расходы:Продукты:Молочные")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if !leaf.IsLeaf {
		t.Error("счёт без потомков не помечен листом")
	}

	after, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	if after-before != 3 {
		t.Errorf("changelog вырос на %d, ожидалось 3", after-before)
	}

	// Повторный вызов идемпотентен: ничего не создаётся.
	byPath, created, err = db.EnsureAccounts(ctx, testImportAccounts())
	if err != nil {
		t.Fatalf("повторный EnsureAccounts: %v", err)
	}
	if len(created) != 0 {
		t.Errorf("повторный вызов создал счета: %v", created)
	}
	if len(byPath) != 3+6 {
		t.Errorf("повторный вызов вернул %d записей", len(byPath))
	}

	// Служебные счета не пересоздаются и не меняют тип.
	byPath, created, err = db.EnsureAccounts(ctx, []AccountSpec{{Path: "Капитал", Type: model.TypeCapital}})
	if err != nil {
		t.Fatalf("EnsureAccounts(Капитал): %v", err)
	}
	if len(created) != 0 {
		t.Errorf("служебный счёт пересоздан: %v", created)
	}
	if byPath["Капитал"].Type != model.TypeCapital {
		t.Errorf("тип служебного счёта %q", byPath["Капитал"].Type)
	}
}

// TestASL_8_EnsureAccountsErrors проверяет отказы при дополнении плана.
func TestASL_8_EnsureAccountsErrors(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	t.Run("без родителя в спецификации", func(t *testing.T) {
		_, _, err := db.EnsureAccounts(ctx, []AccountSpec{{Path: "Расходы:Продукты", Type: model.TypeExpense}})
		if err == nil || !strings.Contains(err.Error(), "не найден родитель") {
			t.Fatalf("err=%v, ожидалась ошибка о ненайденном родителе", err)
		}
	})

	t.Run("тип не совпадает с родителем", func(t *testing.T) {
		_, _, err := db.EnsureAccounts(ctx, []AccountSpec{
			{Path: "Расходы", Type: model.TypeExpense},
			{Path: "Расходы:Продукты", Type: model.TypeIncome},
		})
		if err == nil {
			t.Fatal("несовпадение типа с родителем не отклонено")
		}
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Code != CodeAccountTypeMismatch {
			t.Fatalf("err=%v, ожидался код %s", err, CodeAccountTypeMismatch)
		}
	})

	t.Run("родитель в архиве", func(t *testing.T) {
		root := mustCreateAccount(t, db, "Старое", model.TypeExpense, nil)
		if err := db.ArchiveAccount(ctx, root.ID); err != nil {
			t.Fatalf("ArchiveAccount: %v", err)
		}
		_, _, err := db.EnsureAccounts(ctx, []AccountSpec{
			{Path: "Старое", Type: model.TypeExpense},
			{Path: "Старое:Ужин", Type: model.TypeExpense},
		})
		if !errors.Is(err, ErrAccountArchived) {
			t.Fatalf("err=%v, ожидалась ErrAccountArchived", err)
		}
	})

	t.Run("неверный тип и путь", func(t *testing.T) {
		if _, _, err := db.EnsureAccounts(ctx, []AccountSpec{{Path: "Что-то", Type: "неизвестный"}}); err == nil {
			t.Error("недопустимый тип счёта не отклонён")
		}
		if _, _, err := db.EnsureAccounts(ctx, []AccountSpec{{Path: "Активы::Наличные", Type: model.TypeAsset}}); err == nil {
			t.Error("неверный путь счёта не отклонён")
		}
		if _, _, err := db.EnsureAccounts(ctx, []AccountSpec{{Path: "  ", Type: model.TypeAsset}}); err == nil {
			t.Error("пустой путь счёта не отклонён")
		}
	})
}

// TestASL_8_ImportedHashes проверяет идемпотентность импорта на уровне схемы.
func TestASL_8_ImportedHashes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	userID := mustCreateUser(t, db, "Андрей")
	asset := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	counter := mustCreateAccount(t, db, "Капитал2", model.TypeCapital, nil)

	insert := func(uuid, source, hash string) (*model.Transaction, error) {
		tr := &model.Transaction{
			UUID: uuid, OpType: model.OpInitial, Date: time.Now(),
			Description: "импорт", ActorID: userID, Source: source, SourceHash: hash,
			Lines: []model.TransactionLine{
				{AccountID: asset.ID, Amount: money.FromKopecks(100), Side: model.SideDebit},
				{AccountID: counter.ID, Amount: money.FromKopecks(100), Side: model.SideCredit},
			},
		}
		_, err := db.InsertTransaction(ctx, tr)
		return tr, err
	}

	if _, err := insert("u-1", "hledger", "hash-1"); err != nil {
		t.Fatalf("первый импорт: %v", err)
	}
	hashes, err := db.ImportedHashes(ctx, "hledger")
	if err != nil {
		t.Fatalf("ImportedHashes: %v", err)
	}
	if !hashes["hash-1"] || len(hashes) != 1 {
		t.Fatalf("хеши импорта: %v", hashes)
	}

	// Та же пара «источник + хеш» второй раз запрещена.
	if _, err := insert("u-2", "hledger", "hash-1"); err == nil {
		t.Error("повторный импорт той же проводки не отклонён схемой")
	}
	// Другой источник с тем же хешем допустим.
	if _, err := insert("u-3", "web", "hash-1"); err != nil {
		t.Fatalf("проводка другого источника: %v", err)
	}
	// Проводка без source_hash индексом не ограничивается.
	if _, err := insert("u-4", "hledger", ""); err != nil {
		t.Fatalf("проводка без хеша: %v", err)
	}
	hashes, err = db.ImportedHashes(ctx, "hledger")
	if err != nil {
		t.Fatalf("ImportedHashes: %v", err)
	}
	if len(hashes) != 1 || !hashes["hash-1"] {
		t.Fatalf("хеши импорта после проверок: %v", hashes)
	}
}
