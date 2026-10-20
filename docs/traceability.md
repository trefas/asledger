# Карта трассировки: критерий раздела 14 → автотест

Документ связывает каждый проверяемый критерий приёмки эпика (#1, раздел 14)
с автоматическим тестом. Карта обязательна к обновлению: при появлении нового
критерия добавляется строка с тестом, при переименовании теста правится имя.

Формат строки:

```
| K-NN | текст критерия | TestASL_раздел_пункт, TestASL_раздел_пункт |
```

Колонку тестов разбирает `scripts/check-traceability.sh` (цель `make
lint-trace`, входит в `make check`): каждое имя должно существовать в файлах
`*_test.go`, набор K-01…K-19 — без пропусков и повторов. Непокрытый критерий
помечается отложенным: в колонке тестов пишется `план: #N`, где `#N` — задача,
которая закроет критерий.

| Критерий | Что проверяет | Автотест |
| --- | --- | --- |
| K-01 | ΣDEBIT = ΣCREDIT в каждой сохранённой транзакции | TestASL_6_EngineUnbalanced, TestASL_6_Expense, TestASL_10_Transactions |
| K-02 | В проводках только листовые счета; выбор папки блокируется | TestASL_6_EngineLeafOnly, TestASL_11_SuggestAndMatrix |
| K-03 | Каждая ячейка матрицы правил покрыта тестом; «расход» с дебетом на счёт типа «доход» отклоняется | TestASL_6_MatrixAllCells, TestASL_6_MatrixCells, TestASL_6_EngineMatrixCell |
| K-04 | Трата с несколькими источниками сохраняется одной транзакцией | TestASL_11_EntryMergesSources, TestASL_6_Expense |
| K-05 | Начальные остатки вводятся один раз; строка капитала генерируется движком | TestASL_11_InitialBalances, TestASL_6_Initial |
| K-06 | Две операции с одинаковым uuid дают одну запись и одинаковый ответ | TestASL_6_EngineIdempotentUUID, TestASL_5_InsertTransactionDuplicateUUID, TestASL_10_PushDuplicateUUID, TestASL_13_IdempotentUUIDAndImport |
| K-07 | Коммит из 10 операций с невалидной 7-й не оставляет ни одной | TestASL_10_PushTenOpsAtomicity, TestASL_10_PushAtomicity |
| K-08 | Rebase отставшего base_seq проходит; конфликты и дубли — в warnings | TestASL_10_PushRebase, TestASL_13_ConflictsAndWarnings |
| K-09 | Сторно уже отменённой записи не создаёт второй инверсной | TestASL_6_ReverseIdempotent, TestASL_6_ReverseOfReverse |
| K-10 | Сумма 123.45 хранится как 12345; остаток совпадает со свёрткой журнала | TestASL_14_money_123_45, TestASL_5_BalanceByNature, TestASL_5_BalancesAll |
| K-11 | Два клиента с пересекающимися проводками после pull идентичны (хэш-цепочка совпала) | TestASL_13_HashChainAndFullResync, TestASL_13_ParallelCommitsConverge, TestASL_7_ChainVerification |
| K-12 | Повторный импорт того же hledger-файла не создаёт дублей; строгий отказывается, гибкий создаёт | TestASL_8_ImportIdempotent, TestASL_8_ImportStrictRejectsUnknownAccount, TestASL_8_ImportFlexibleCreatesAccounts, TestASL_13_IdempotentUUIDAndImport |
| K-13 | Экспорт в hledger + повторный импорт дают идентичное состояние | TestASL_8_ExportImportRoundTrip, TestASL_8_WriteRoundTrip |
| K-14 | Импорт шаблонного плана создаёт служебные счета | TestASL_8_ChartTemplatesImport, TestASL_5_SystemAccountsCreated |
| K-15 | accounts/suggest возвращает листовые счета по частичному вводу в кириллице | TestASL_10_Suggest, TestASL_11_SuggestAndMatrix |
| K-16 | Проводка ниже min_balance отклоняется в лимитном режиме, в свободном — принимается с подсветкой | TestASL_6_EngineMinBalance, TestASL_11_ReportsPage |
| K-17 | Лого в README и хедере; favicon отдаётся корректно | TestASL_11_BrandAssetsSet, TestASL_11_WebJournalAndBrand |
| K-18 | Всё на кириллице, валюта «₽»; web :8777, API :8778; демон работает как systemd-служба | TestASL_8_Rubles, TestASL_10_HealthAndRouting, TestASL_11_WebJournalAndBrand, TestASL_13_SystemdUnit |
| K-19 | Тег порождает релиз с артефактами Linux/Windows, SHA-256, шаблонами и версией | TestASL_13_ReleaseConfig, TestASL_13_InstallDocs, TestASL_13_VersionDefaults |

## Отложенные критерии

Нет: все критерии K-01…K-19 покрыты автотестами (K-19 — статической проверкой
конфигурации GoReleaser, workflow публикации и документации релиза; сам факт
сборки артефактов проверяется в CI на теге).

Критерий K-18 покрыт юнит-тестом `TestASL_13_SystemdUnit` (юнит, порты,
ограничения безопасности) и тестами web/API; запуск службы после перезагрузки
проверяется вручную по инструкции `docs/install-linux.md`.

