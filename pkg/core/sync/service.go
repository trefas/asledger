// Package sync реализует синхронизацию по модели git (раздел 7 эпика).
//
// Сервер — master-ветка: единственная точка, задающая порядок коммитов.
// Устройство держит снапшот книги и стейджинг офлайн-проводок, коммитит их
// через Push и подтягивает дельту через Pull. Хэш-цепочка коммитов
// (SHA-256(prev_hash || ops_json)) доказывает идентичность реплик.
package sync

import (
	"context"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// SnapshotFormatVersion — версия формата снапшота и дельты.
const SnapshotFormatVersion = 1

// Service — сервис синхронизации поверх книги и движка проводок.
type Service struct {
	db     *store.DB
	engine *engine.Engine
}

// New создаёт сервис синхронизации.
func New(db *store.DB) *Service {
	return &Service{db: db, engine: engine.New(db)}
}

// Engine возвращает движок проводок — точку входа онлайн-проводок.
func (s *Service) Engine() *engine.Engine { return s.engine }

// User — пользователь в снапшоте: без хеша токена и других секретов.
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Snapshot — полный снапшот книги для клонирования на новое устройство.
type Snapshot struct {
	FormatVersion int                 `json:"format_version"`
	HeadSeq       int64               `json:"head_seq"`
	ChainHash     string              `json:"chain_hash"`
	Accounts      []model.Account     `json:"accounts"`
	Users         []User              `json:"users"`
	Settings      map[string]string   `json:"settings"`
	Transactions  []model.Transaction `json:"transactions"`
}

// Snapshot собирает полный снапшот книги.
//
// План счетов отдаётся целиком; для мобильных клиентов он read-only.
func (s *Service) Snapshot(ctx context.Context) (*Snapshot, error) {
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.db.Settings(ctx)
	if err != nil {
		return nil, err
	}
	head, err := s.db.HeadSeq(ctx)
	if err != nil {
		return nil, err
	}
	transactions, err := s.db.ListTransactions(ctx, store.TransactionFilter{})
	if err != nil {
		return nil, err
	}
	chainHash, err := s.chainHash(ctx)
	if err != nil {
		return nil, err
	}

	snapshot := &Snapshot{
		FormatVersion: SnapshotFormatVersion,
		HeadSeq:       head,
		ChainHash:     chainHash,
		Accounts:      accounts,
		Users:         publicUsers(users),
		Settings:      settings,
		Transactions:  transactions,
	}
	return snapshot, nil
}

// Pull — дельта журнала изменений с данными для верификации хэш-цепочки.
type Pull struct {
	FormatVersion    int                    `json:"format_version"`
	Since            int64                  `json:"since"`
	HeadSeq          int64                  `json:"head_seq"`
	ChainHash        string                 `json:"chain_hash"`
	Changes          []model.ChangelogEntry `json:"changes"`
	Transactions     []model.Transaction    `json:"transactions"`
	Accounts         []model.Account        `json:"accounts"`
	AccountsReadOnly bool                   `json:"accounts_read_only"`
	Commits          []model.Commit         `json:"commits"`
	ResyncRequired   bool                   `json:"resync_required"`
}

// Pull возвращает дельту журнала изменений с seq больше since.
//
// knownChainHash — последний известный клиенту хеш цепочки. Если он задан и
// не совпадает с историей сервера на этой точке, выставляется ResyncRequired:
// клиенту нужен полный ресинк со снапшота.
func (s *Service) Pull(ctx context.Context, since int64, knownChainHash string) (*Pull, error) {
	changes, err := s.db.Changelog(ctx, since)
	if err != nil {
		return nil, err
	}

	var transactions []model.Transaction
	for _, entry := range changes {
		if entry.Entity != model.EntityTransaction || entry.Op != model.OpInsert {
			continue
		}
		tr, err := s.db.Transaction(ctx, entry.EntityID)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, *tr)
	}

	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	head, err := s.db.HeadSeq(ctx)
	if err != nil {
		return nil, err
	}
	commits, err := s.db.CommitsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	chainHash, err := s.chainHash(ctx)
	if err != nil {
		return nil, err
	}

	pull := &Pull{
		FormatVersion:    SnapshotFormatVersion,
		Since:            since,
		HeadSeq:          head,
		ChainHash:        chainHash,
		Changes:          changes,
		Transactions:     transactions,
		Accounts:         accounts,
		AccountsReadOnly: true,
		Commits:          commits,
	}
	if knownChainHash != "" {
		pull.ResyncRequired = !s.chainMatches(ctx, since, knownChainHash)
	}
	return pull, nil
}

// publicUsers убирает из пользователей хеши токенов.
func publicUsers(users []model.User) []User {
	out := make([]User, 0, len(users))
	for _, u := range users {
		out = append(out, User{ID: u.ID, Name: u.Name})
	}
	return out
}
