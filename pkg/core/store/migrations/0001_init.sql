-- Начальная схема книги asledger (раздел 5 эпика).
--
-- Денежные величины и время хранятся целыми числами: копейки и unix-секунды.
-- Остаток счёта нигде не хранится — он всегда вычисляется свёрткой журнала.

-- План счетов (иерархическое дерево).
CREATE TABLE accounts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,                -- короткое имя, кириллица
    full_path   TEXT NOT NULL UNIQUE,         -- «Расходы:Питание:Продукты»
    type        TEXT NOT NULL CHECK (type IN ('актив','пассив','капитал','доход','расход')),
    parent_id   INTEGER REFERENCES accounts(id),  -- NULL для корневого счёта
    is_leaf     INTEGER NOT NULL DEFAULT 1,   -- TRUE — счёт допустим в проводках
    min_balance INTEGER NOT NULL DEFAULT 0,   -- 0 для наличных, <0 для овердрафта
    is_archived INTEGER NOT NULL DEFAULT 0,
    is_system   INTEGER NOT NULL DEFAULT 0,   -- служебный счёт, не редактируется
    created_at  INTEGER NOT NULL
);
CREATE INDEX idx_accounts_name   ON accounts(name COLLATE NOCASE);
CREATE INDEX idx_accounts_parent ON accounts(parent_id);

-- Пользователи семьи.
CREATE TABLE users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    token_hash TEXT NOT NULL
);

-- Настройки (например, флаг «начальные остатки введены»).
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Транзакции: заголовок многострочной проводки.
CREATE TABLE transactions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,         -- UUID генерирует клиент
    op_type     TEXT NOT NULL CHECK (op_type IN
                ('трата','доход','перевод','погашение_долга','выдача_займа',
                 'начальные_остатки','переоценка','корректировка')),
    date        INTEGER NOT NULL,
    description TEXT,
    actor_id    INTEGER NOT NULL REFERENCES users(id),
    reverses_id INTEGER REFERENCES transactions(id),  -- для сторно
    source      TEXT,                         -- 'hledger', 'web', 'api:<client_id>'
    source_hash TEXT,                         -- хеш содержимого строки импорта
    created_at  INTEGER NOT NULL,
    commit_id   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_transactions_date  ON transactions(date);
CREATE INDEX idx_transactions_uuid  ON transactions(uuid);
CREATE INDEX idx_transactions_actor ON transactions(actor_id);

-- Строки проводок: сумма всегда положительная, сторону назначает движок.
CREATE TABLE transaction_lines (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    transaction_id INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    account_id     INTEGER NOT NULL REFERENCES accounts(id),
    amount_kop     INTEGER NOT NULL CHECK (amount_kop > 0),
    side           TEXT NOT NULL CHECK (side IN ('DEBIT','CREDIT'))
);
CREATE INDEX idx_lines_tx      ON transaction_lines(transaction_id);
CREATE INDEX idx_lines_account ON transaction_lines(account_id);
CREATE UNIQUE INDEX idx_lines_tx_account_side
    ON transaction_lines(transaction_id, account_id, side);

-- Журнал изменений для синхронизации.
CREATE TABLE changelog (
    seq        INTEGER PRIMARY KEY AUTOINCREMENT,
    entity     TEXT NOT NULL,   -- 'accounts' | 'transactions'
    entity_id  INTEGER NOT NULL,
    op         TEXT NOT NULL,   -- 'insert' | 'update' | 'archive'
    changed_at INTEGER NOT NULL
);
CREATE INDEX idx_changelog_entity ON changelog(entity, entity_id);

-- Коммиты синхронизации (хэш-цепочка, аналог git-истории).
CREATE TABLE commits (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id  TEXT NOT NULL,
    actor_id   INTEGER NOT NULL REFERENCES users(id),
    base_seq   INTEGER NOT NULL,
    ops_json   TEXT NOT NULL,
    result_seq INTEGER NOT NULL,
    prev_hash  TEXT NOT NULL,   -- SHA-256(prev_hash || ops_json)
    warnings   TEXT,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_commits_client ON commits(client_id);
