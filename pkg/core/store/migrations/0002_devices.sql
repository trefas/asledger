-- Устройства синхронизации (раздел 7 и 10 эпика).
--
-- Привязка устройства выдаёт токен для client_id; открытый токен не хранится,
-- сохраняется только его хеш (SHA-256 от высокоэнтропийного значения).

CREATE TABLE devices (
    client_id  TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_devices_user ON devices(user_id);
