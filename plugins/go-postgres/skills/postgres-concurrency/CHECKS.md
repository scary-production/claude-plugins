# postgres-concurrency — проверка

Файл для того, кто правит скилл: какие утверждения проверены и как их перепроверить.
Claude его не загружает.

## Проверено опытом

2026-10-18, PostgreSQL 18.6: psql в двух сеансах и программа на Go (pgx v5.11.0,
go-transaction-manager v2.0.4).

| Утверждение | Как проверено | Итог |
|---|---|---|
| Write skew на read committed, serializable его ловит | две конкурентные брони с пересечением интервалов | ✓ RC: 2 брони; serializable: 1, вторая — `40001` |
| `40001` может прийти на `COMMIT` | тот же опыт | ✓ |
| Exclusion-ограничение держит пересечения на read committed без повторов | две конкурентные вставки | ✓ вторая ждала первую и получила `23P01` |
| `btree_gist` ставится без суперпользователя | `pg_available_extension_versions.trusted` | ✓ trusted; `pgrowlocks` — нет |
| Снимок repeatable read — с первой команды, не с `BEGIN` | `BEGIN`, вставка из другого сеанса, чтение | ✓ вставка видна |
| Seq Scan в serializable блокирует всю таблицу, индекс — строку и страницу | `pg_locks` с `SIReadLock` | ✓ `relation` против `tuple` + `page` |
| Проверка FK берёт `FOR KEY SHARE`: `FOR UPDATE` на родителе блокирует вставку ребёнка, `FOR NO KEY UPDATE` — нет | два сеанса, `lock_timeout 1s` | ✓ текст ошибки: `SELECT 1 FROM ONLY parent x WHERE id = $1 FOR KEY SHARE OF x` |
| `lock_timeout` и `NOWAIT` дают `55P03` | два сеанса | ✓ |
| У `UPDATE` нет `NOWAIT` | `UPDATE ... NOWAIT` | ✓ синтаксическая ошибка |
| `NOWAIT` ждёт табличную блокировку | `LOCK TABLE ... ACCESS EXCLUSIVE` в другом сеансе | ✓ ждал до `lock_timeout` |
| `pg_advisory_xact_lock` снимается на rollback | `pg_locks` до и после | ✓ |

## Не вошло

- Заморозка мультитранзакций, `vacuum_multixact_*`.
- `log_lock_waits`, `deadlock_timeout` — кандидат в отдельный раздел о разборе зависаний.
- Очередь за `ACCESS EXCLUSIVE` и `lock_timeout` для DDL — кандидат в postgres-migrations.
