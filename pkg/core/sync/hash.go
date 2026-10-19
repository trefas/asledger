package sync

import (
	"context"
	"errors"

	"github.com/trefas/asledger/pkg/core/store"
)

// ChainHash вычисляет звено хэш-цепочки: SHA-256(prevHash || opsJSON).
//
// Вычисления живут в store: звено цепочки вычисляется там же, где коммит
// записывается, — под блокировкой и в одной транзакции с head_seq.
func ChainHash(prevHash, opsJSON string) string {
	return store.ChainHash(prevHash, opsJSON)
}

// chainHash возвращает текущий хеш цепочки (пустая строка для пустой истории).
func (s *Service) chainHash(ctx context.Context) (string, error) {
	commit, err := s.db.LastCommit(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return commit.Hash, nil
}

// chainMatches проверяет последний известный клиенту хеш цепочки.
//
// Клиент, стоящий на seq, должен знать хеш коммита с result_seq = seq;
// расхождение означает, что реплики разошлись и нужен полный ресинк.
func (s *Service) chainMatches(ctx context.Context, since int64, knownHash string) bool {
	if since == 0 {
		return knownHash == ""
	}
	commit, err := s.db.CommitByResultSeq(ctx, since)
	if errors.Is(err, store.ErrNotFound) {
		return false
	}
	if err != nil {
		return false
	}
	return commit.Hash == knownHash
}
