package hledger

import (
	"context"
	"fmt"
	"io"

	"github.com/trefas/asledger/pkg/core/store"
)

// Export читает книгу и выгружает её в plaintext-формат hledger.
//
// Выгружаются план счетов и все проводки; архивные счета — по флагу
// IncludeArchived. Формат plaintext переносит план счетов (пути и типы),
// проводки и, следовательно, остатки; служебные поля asledger (min_balance,
// признак архивации, секреты устройств) он не описывает — для полной резервной
// копии книги предназначен JSON-снапшот.
func Export(ctx context.Context, db *store.DB, w io.Writer, opts WriteOptions) error {
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return fmt.Errorf("hledger: не удалось прочитать план счетов: %w", err)
	}
	transactions, err := db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		return fmt.Errorf("hledger: не удалось прочитать проводки: %w", err)
	}
	return Write(w, accounts, transactions, opts)
}
