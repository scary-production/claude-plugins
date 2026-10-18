# postgres-migrations — проверка

Файл для того, кто правит скилл: какие утверждения проверены и как их перепроверить.
Claude его не загружает.

## Проверено опытом

2026-10-18, PostgreSQL 18.6, psql.

| Утверждение | Как проверено | Итог |
|---|---|---|
| CIC в блоке транзакции запрещён | `BEGIN; CREATE INDEX CONCURRENTLY ...` | ✓ `cannot run inside a transaction block` |
| Две команды одним запросом — неявная транзакция, CIC падает | `psql -c "select 1; create index concurrently ..."` (одним simple query — так шлёт `Exec` без аргументов в pgx) | ✓ та же ошибка; одна команда — проходит |
| Упавший CIC оставляет INVALID-индекс | `CREATE UNIQUE INDEX CONCURRENTLY` на дублях | ✓ `indisvalid = f` |
| Повтор без `IF NOT EXISTS` падает, с ним — молча пропускает INVALID | повтор той же команды | ✓ `already exists`; `NOTICE ... skipping`, индекс остался `indisvalid = f` |
| `DROP INDEX CONCURRENTLY IF EXISTS` идемпотентен | дважды | ✓ |
| CIC ждёт транзакции со старым снимком, даже по другой таблице | открытая транзакция с чтением другой таблицы | ✓ CIC ждал её конца |
| `indcheckxmin` воспроизводится | старый снимок RR + UPDATE + `CREATE INDEX` | ✓ `indcheckxmin = t` |
| `ADD VALUE` в транзакции работает, новое значение до коммита не использовать | `BEGIN; ALTER TYPE ... ADD VALUE; INSERT ...` | ✓ `55P04 unsafe use of new value`; после коммита — можно |
| `DROP VALUE` нет | `ALTER TYPE ... DROP VALUE` | ✓ `0A000 not implemented` |
| Константный `DEFAULT` не переписывает таблицу, volatile — переписывает | `relfilenode` до и после `ADD COLUMN` | ✓ не изменился / изменился |

## Не вошло

- `ALTER TABLE ... SET (vacuum_truncate = false)` против замираний из-за усечения хвоста
  автовакуумом — нужно только терабайтным таблицам с огромным `shared_buffers`.
- Временные таблицы `ON COMMIT DELETE ROWS` замедляют commit — не про миграции.
- Безопасные `ALTER` на живых таблицах (`lock_timeout` для DDL, `ADD CONSTRAINT ... NOT
  VALID` и `VALIDATE`, `SET NOT NULL` на большой таблице) — кандидат в скилл.
