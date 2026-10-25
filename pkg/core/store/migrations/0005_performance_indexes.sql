-- Индексы для горячих запросов: остатки по счёту, синхронизация и расходы.
--
-- Покрывающий индекс по (account_id, side, amount_kop) позволяет считать
-- остаток счёта без обращения к таблице строк проводок. Остальные индексы
-- ускоряют выборку коммитов по result_seq и счетов по типу.

CREATE INDEX idx_lines_account_side_amount
    ON transaction_lines(account_id, side, amount_kop);
CREATE INDEX idx_commits_result_seq ON commits(result_seq);
CREATE INDEX idx_accounts_type ON accounts(type);
