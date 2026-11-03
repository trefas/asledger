-- Кредиты и вклады (эпик #166).
--
-- Кредит — это листовой счёт типа «пассив», вклад — листовой счёт типа
-- «актив». Тип счёта не меняется: настройки лишь дополняют счёт правилами
-- расчёта. Ставки хранятся целыми ppm: 1 ppm = 0.0001 %, 100 % = 1 000 000.
-- Суммы — в копейках, время — в unix-секундах, как и в остальной книге.

-- Настройки кредитного счёта.
CREATE TABLE credit_settings (
    account_id      INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    rate_ppm        INTEGER NOT NULL DEFAULT 0
                    CHECK (rate_ppm BETWEEN 0 AND 1000000),
    -- simple — простая ставка; on_balance — проценты на остаток долга.
    interest_type   TEXT NOT NULL DEFAULT 'simple'
                    CHECK (interest_type IN ('simple','on_balance')),
    -- interest_first — сначала проценты, затем тело; principal_first — наоборот.
    repayment_order TEXT NOT NULL DEFAULT 'interest_first'
                    CHECK (repayment_order IN ('interest_first','principal_first')),
    grace_enabled   INTEGER NOT NULL DEFAULT 0 CHECK (grace_enabled IN (0,1)),
    -- 0 — дата не задана; при grace_enabled = 1 должна быть задана.
    grace_end_date  INTEGER NOT NULL DEFAULT 0 CHECK (grace_end_date >= 0),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    CHECK (grace_enabled = 0 OR grace_end_date > 0)
);

-- Настройки вклада или накопительного счёта.
CREATE TABLE deposit_settings (
    account_id        INTEGER PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    base_rate_ppm     INTEGER NOT NULL DEFAULT 0
                      CHECK (base_rate_ppm BETWEEN 0 AND 1000000),
    min_balance_kop   INTEGER NOT NULL DEFAULT 0 CHECK (min_balance_kop >= 0),
    -- none — без бонуса; fixed — фиксированная сумма; rate — процент сверх ставки.
    bonus_type        TEXT NOT NULL DEFAULT 'none'
                      CHECK (bonus_type IN ('none','fixed','rate')),
    bonus_rate_ppm    INTEGER NOT NULL DEFAULT 0
                      CHECK (bonus_rate_ppm BETWEEN 0 AND 1000000),
    bonus_amount_kop  INTEGER NOT NULL DEFAULT 0 CHECK (bonus_amount_kop >= 0),
    -- daily — на фактический остаток; monthly — на минимальный остаток за месяц.
    accrual_frequency TEXT NOT NULL DEFAULT 'monthly'
                      CHECK (accrual_frequency IN ('daily','monthly')),
    capitalization    INTEGER NOT NULL DEFAULT 0 CHECK (capitalization IN (0,1)),
    opened_at         INTEGER NOT NULL DEFAULT 0 CHECK (opened_at >= 0),
    closed_at         INTEGER NOT NULL DEFAULT 0 CHECK (closed_at >= 0),
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    CHECK (closed_at = 0 OR opened_at = 0 OR closed_at >= opened_at),
    CHECK (bonus_type <> 'rate' OR bonus_rate_ppm > 0),
    CHECK (bonus_type <> 'fixed' OR bonus_amount_kop > 0)
);

-- Ступенчатые ставки вклада.
--
-- to_kop = NULL означает «до бесконечности». Пересечения диапазонов
-- проверяет store-слой: в SQLite их нельзя выразить обычным CHECK.
CREATE TABLE deposit_step_rates (
    account_id INTEGER NOT NULL REFERENCES deposit_settings(account_id) ON DELETE CASCADE,
    from_kop   INTEGER NOT NULL CHECK (from_kop >= 0),
    to_kop     INTEGER CHECK (to_kop IS NULL OR to_kop > from_kop),
    rate_ppm   INTEGER NOT NULL CHECK (rate_ppm BETWEEN 0 AND 1000000),
    PRIMARY KEY (account_id, from_kop)
);

-- Начисления процентов и бонусов.
--
-- Субледжер: начисление фиксируется отдельно от проводки и связывается с ней
-- через transaction_id. Период 0…0 означает ручное начисление на дату.
CREATE TABLE interest_accruals (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid             TEXT NOT NULL UNIQUE,
    account_id       INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date             INTEGER NOT NULL CHECK (date >= 0),
    kind             TEXT NOT NULL
                     CHECK (kind IN ('credit_interest','deposit_interest','deposit_bonus')),
    amount_kop       INTEGER NOT NULL CHECK (amount_kop > 0),
    base_amount_kop  INTEGER NOT NULL DEFAULT 0 CHECK (base_amount_kop >= 0),
    rate_ppm         INTEGER NOT NULL DEFAULT 0
                     CHECK (rate_ppm BETWEEN 0 AND 1000000),
    period_from      INTEGER NOT NULL DEFAULT 0 CHECK (period_from >= 0),
    period_to        INTEGER NOT NULL DEFAULT 0 CHECK (period_to >= 0),
    status           TEXT NOT NULL DEFAULT 'accrued'
                     CHECK (status IN ('accrued','paid','capitalized')),
    transaction_id   INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    CHECK (period_to = 0 OR period_from = 0 OR period_to >= period_from)
);
CREATE INDEX idx_interest_accruals_account_date
    ON interest_accruals(account_id, date);
CREATE INDEX idx_interest_accruals_status
    ON interest_accruals(status);
CREATE INDEX idx_interest_accruals_transaction
    ON interest_accruals(transaction_id);

-- Периодическое начисление идемпотентно: на один счёт, вид и период —
-- не больше одной записи. Ручные начисления (0…0) могут повторяться.
CREATE UNIQUE INDEX idx_interest_accruals_period
    ON interest_accruals(account_id, kind, period_from, period_to)
    WHERE period_from > 0 OR period_to > 0;

-- Настройки кредита допустимы только для листового пассива.
CREATE TRIGGER credit_settings_account_insert
BEFORE INSERT ON credit_settings
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1 FROM accounts
     WHERE id = NEW.account_id AND is_leaf = 1 AND type = 'пассив'
)
BEGIN
    SELECT RAISE(ABORT, 'credit_settings: счёт должен быть листовым пассивом');
END;

CREATE TRIGGER credit_settings_account_update
BEFORE UPDATE OF account_id ON credit_settings
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1 FROM accounts
     WHERE id = NEW.account_id AND is_leaf = 1 AND type = 'пассив'
)
BEGIN
    SELECT RAISE(ABORT, 'credit_settings: счёт должен быть листовым пассивом');
END;

-- Настройки вклада допустимы только для листового актива.
CREATE TRIGGER deposit_settings_account_insert
BEFORE INSERT ON deposit_settings
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1 FROM accounts
     WHERE id = NEW.account_id AND is_leaf = 1 AND type = 'актив'
)
BEGIN
    SELECT RAISE(ABORT, 'deposit_settings: счёт должен быть листовым активом');
END;

CREATE TRIGGER deposit_settings_account_update
BEFORE UPDATE OF account_id ON deposit_settings
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1 FROM accounts
     WHERE id = NEW.account_id AND is_leaf = 1 AND type = 'актив'
)
BEGIN
    SELECT RAISE(ABORT, 'deposit_settings: счёт должен быть листовым активом');
END;

-- Начисление относится только к счёту с настройками кредита или вклада.
CREATE TRIGGER interest_accruals_account_insert
BEFORE INSERT ON interest_accruals
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM credit_settings WHERE account_id = NEW.account_id)
 AND NOT EXISTS (SELECT 1 FROM deposit_settings WHERE account_id = NEW.account_id)
BEGIN
    SELECT RAISE(ABORT, 'interest_accruals: у счёта нет настроек кредита или вклада');
END;

CREATE TRIGGER interest_accruals_account_update
BEFORE UPDATE OF account_id ON interest_accruals
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM credit_settings WHERE account_id = NEW.account_id)
 AND NOT EXISTS (SELECT 1 FROM deposit_settings WHERE account_id = NEW.account_id)
BEGIN
    SELECT RAISE(ABORT, 'interest_accruals: у счёта нет настроек кредита или вклада');
END;
