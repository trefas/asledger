-- Новый тип операции «начисление_процентов» (эпик #166, задача 181).
--
-- CHECK нельзя изменить через ALTER TABLE, поэтому таблица transactions
-- пересоздаётся с сохранением данных. Внешние ключи выключаются только на это
-- соединение и только на время миграции; legacy_alter_table не даёт SQLite
-- переписать ссылки дочерних таблиц на временное имя.

PRAGMA foreign_keys = OFF;
PRAGMA legacy_alter_table = ON;

ALTER TABLE transactions RENAME TO transactions_old;

CREATE TABLE transactions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid        TEXT NOT NULL UNIQUE,         -- UUID генерирует клиент
    op_type     TEXT NOT NULL CHECK (op_type IN
                ('трата','доход','перевод','погашение_долга','выдача_займа',
                 'начальные_остатки','переоценка','корректировка',
                 'начисление_процентов')),
    date        INTEGER NOT NULL,
    description TEXT,
    actor_id    INTEGER NOT NULL REFERENCES users(id),
    reverses_id INTEGER REFERENCES transactions(id),  -- для сторно
    source      TEXT,                         -- 'hledger', 'web', 'api:<client_id>'
    source_hash TEXT,                         -- хеш содержимого строки импорта
    created_at  INTEGER NOT NULL,
    commit_id   INTEGER NOT NULL DEFAULT 0
);

INSERT INTO transactions (id, uuid, op_type, date, description, actor_id,
                          reverses_id, source, source_hash, created_at, commit_id)
SELECT id, uuid, op_type, date, description, actor_id,
       reverses_id, source, source_hash, created_at, commit_id
  FROM transactions_old;

DROP TABLE transactions_old;

CREATE INDEX idx_transactions_date  ON transactions(date);
CREATE INDEX idx_transactions_uuid  ON transactions(uuid);
CREATE INDEX idx_transactions_actor ON transactions(actor_id);
CREATE UNIQUE INDEX idx_transactions_source_hash
    ON transactions(source, source_hash)
    WHERE source_hash IS NOT NULL AND source_hash <> '';

PRAGMA legacy_alter_table = OFF;
PRAGMA foreign_keys = ON;
