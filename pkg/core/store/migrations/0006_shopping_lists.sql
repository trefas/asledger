-- Списки покупок (эпик #165).
--
-- Денежные величины и время хранятся целыми числами: копейки и unix-секунды.
-- Список покупок — вспомогательная сущность книги: в проводки попадают только
-- подтверждённые позиции, а неподтверждённые остаются для следующих походов.

-- Списки покупок.
CREATE TABLE shopping_lists (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    planned_date INTEGER NOT NULL DEFAULT 0,   -- 0 — дата не задана
    status       TEXT NOT NULL DEFAULT 'active'
                 CHECK (status IN ('draft','active','shopping','completed','archived')),
    owner_id     INTEGER NOT NULL REFERENCES users(id),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    archived_at  INTEGER
);
CREATE INDEX idx_shopping_lists_status ON shopping_lists(status);
CREATE INDEX idx_shopping_lists_date   ON shopping_lists(planned_date);

-- Позиции списка покупок.
--
-- account_id — статья расходов из плана счетов; может быть не задана в
-- черновике, но обязательна при проведении покупки.
-- version используется для оптимистичной блокировки при синхронизации.
CREATE TABLE shopping_items (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid              TEXT NOT NULL UNIQUE,
    list_id           INTEGER NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    account_id        INTEGER REFERENCES accounts(id),
    assignee_id       INTEGER REFERENCES users(id),
    priority          TEXT NOT NULL DEFAULT 'normal'
                      CHECK (priority IN ('low','normal','high')),
    planned_price_kop INTEGER NOT NULL DEFAULT 0 CHECK (planned_price_kop >= 0),
    actual_price_kop  INTEGER NOT NULL DEFAULT 0 CHECK (actual_price_kop >= 0),
    in_cart           INTEGER NOT NULL DEFAULT 0 CHECK (in_cart IN (0,1)),
    price_mismatch    INTEGER NOT NULL DEFAULT 0 CHECK (price_mismatch IN (0,1)),
    comment           TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'planned'
                      CHECK (status IN ('planned','confirmed','purchased','skipped')),
    position          INTEGER NOT NULL DEFAULT 0,
    version           INTEGER NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX idx_shopping_items_list     ON shopping_items(list_id, position);
CREATE INDEX idx_shopping_items_account  ON shopping_items(account_id);
CREATE INDEX idx_shopping_items_assignee ON shopping_items(assignee_id);

-- Шаблоны списков покупок.
CREATE TABLE shopping_list_templates (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Базовые позиции шаблона: наименование, статья расходов и приоритет.
CREATE TABLE shopping_template_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id INTEGER NOT NULL REFERENCES shopping_list_templates(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    account_id  INTEGER REFERENCES accounts(id),
    priority    TEXT NOT NULL DEFAULT 'normal'
                CHECK (priority IN ('low','normal','high')),
    position    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_shopping_template_items_template
    ON shopping_template_items(template_id, position);
