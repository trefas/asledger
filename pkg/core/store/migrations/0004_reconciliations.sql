-- Сверка счетов с фактическим остатком (раздел 9.4 эпика).
--
-- Фактический остаток вводит пользователь; расчётный берётся свёрткой журнала
-- на момент сверки. Хранится история: последняя запись по счёту показывает
-- текущее расхождение, предыдущие — историю. Сгенерированная корректировка
-- связана со сверкой через transaction_id.

CREATE TABLE reconciliations (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id     INTEGER NOT NULL REFERENCES accounts(id),
    fact_kop       INTEGER NOT NULL,                    -- фактический остаток
    expected_kop   INTEGER NOT NULL,                    -- расчётный остаток при сверке
    checked_at     INTEGER NOT NULL,                    -- время сверки (unix-секунды)
    transaction_id INTEGER REFERENCES transactions(id)  -- сгенерированная корректировка
);
CREATE INDEX idx_reconciliations_account ON reconciliations(account_id, id DESC);
