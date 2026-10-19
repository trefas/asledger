-- Идемпотентность импорта: пара «источник + хеш содержимого строки импорта»
-- уникальна, поэтому повторный импорт того же файла не создаёт дублей даже
-- при одновременной работе демона и утилиты asledgerctl.
CREATE UNIQUE INDEX idx_transactions_source_hash
    ON transactions(source, source_hash)
    WHERE source_hash IS NOT NULL AND source_hash <> '';
