package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
)

// TestASL_5_ChangelogOneEntryPerMutation проверяет, что каждая мутация даёт
// ровно одну запись журнала.
func TestASL_5_ChangelogOneEntryPerMutation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM changelog WHERE entity = 'accounts'`).Scan(&n); err != nil {
			t.Fatalf("подсчёт журнала: %v", err)
		}
		return n
	}

	before := count()
	acc := mustCreateAccount(t, db, "Имущество", model.TypeAsset, nil)
	if got := count(); got != before+1 {
		t.Fatalf("после создания счёта записей %d, ожидалось %d", got, before+1)
	}

	if err := db.RenameAccount(ctx, acc.ID, "Собственность"); err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}
	if got := count(); got != before+2 {
		t.Fatalf("после переименования записей %d, ожидалось %d", got, before+2)
	}

	if err := db.ArchiveAccount(ctx, acc.ID); err != nil {
		t.Fatalf("ArchiveAccount: %v", err)
	}
	if got := count(); got != before+3 {
		t.Fatalf("после архивирования записей %d, ожидалось %d", got, before+3)
	}

	// Все три записи относятся к одному счёту и идут по возрастанию seq.
	entries, err := db.Changelog(ctx, 0)
	if err != nil {
		t.Fatalf("Changelog: %v", err)
	}
	wantOps := []string{model.OpInsert, model.OpUpdate, model.OpArchive}
	var gotOps []string
	var lastSeq int64
	for _, e := range entries {
		if e.Seq <= lastSeq {
			t.Fatalf("seq не возрастает: %d после %d", e.Seq, lastSeq)
		}
		lastSeq = e.Seq
		if e.Entity == model.EntityAccount && e.EntityID == acc.ID {
			gotOps = append(gotOps, e.Op)
		}
	}
	if len(gotOps) != len(wantOps) {
		t.Fatalf("операций по счёту %d: %v, ожидалось %v", len(gotOps), gotOps, wantOps)
	}
	for i := range wantOps {
		if gotOps[i] != wantOps[i] {
			t.Fatalf("операции по счёту: %v, ожидалось %v", gotOps, wantOps)
		}
	}
}

// TestASL_5_ChangelogSince проверяет выдачу дельты журнала от заданного seq.
func TestASL_5_ChangelogSince(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	mustCreateAccount(t, db, "Первое", model.TypeAsset, nil)
	mark, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	mustCreateAccount(t, db, "Второе", model.TypeAsset, nil)
	mustCreateAccount(t, db, "Третье", model.TypeAsset, nil)

	delta, err := db.Changelog(ctx, mark)
	if err != nil {
		t.Fatalf("Changelog: %v", err)
	}
	if len(delta) != 2 {
		t.Fatalf("дельта от seq=%d содержит %d записей, ожидалось 2", mark, len(delta))
	}
	for _, e := range delta {
		if e.Seq <= mark {
			t.Fatalf("дельта содержит устаревшую запись seq=%d", e.Seq)
		}
	}
}

// TestASL_5_ChangelogRollsBack проверяет, что откат транзакции откатывает и
// запись журнала изменений.
func TestASL_5_ChangelogRollsBack(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	before, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}

	sentinel := errors.New("искусственный откат")
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		if err := appendChangelog(ctx, tx, model.EntityAccount, 1, model.OpInsert); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Tx вернул %v, ожидалась искусственная ошибка", err)
	}

	after, err := db.HeadSeq(ctx)
	if err != nil {
		t.Fatalf("HeadSeq: %v", err)
	}
	if after != before {
		t.Fatalf("откат транзакции оставил запись журнала: %d → %d", before, after)
	}
}
