# postgres-query-tuning — проверка

Файл для того, кто правит скилл: какие утверждения проверены и как их перепроверить.
Claude его не загружает.

## Сверено с документацией PostgreSQL

| Утверждение | Раздел | Итог |
|---|---|---|
| `track = all`, иначе вложенные запросы не видны; по умолчанию `top` | pg_stat_statements | ✓ |
| `pg_stat_statements_reset()` — суперпользователь или право; стирает статистику всех | pg_stat_statements | ✓ |
| Столбец `toplevel`; время верхнего уровня включает вложенные | pg_stat_statements, release notes 14 | ✓ название — `toplevel`, не `top_level` |
| Константы нормализуются, разбивки по сеансам нет, хранятся min/max/stddev | pg_stat_statements | ✓ |
| Связь с `pg_stat_activity` и `EXPLAIN VERBOSE` по queryid, нужен `compute_query_id` | monitoring-stats, EXPLAIN | ✓ |
| auto_explain: `log_min_duration` (-1), `log_analyze`, `log_nested_statements` (off) | auto-explain, runtime-config-logging | ✓ |
| Логирование для одного сеанса штатно возможно: `SET log_min_duration_statement`, `LOAD 'auto_explain'` | runtime-config-logging, auto-explain | ✓ нужны права, см. опыт ниже |
| relpages/reltuples — оценки для планировщика | pg_class | ✓ |
| Буферы верхнего узла включают дочерние; temp read/written | EXPLAIN | ✓ |
| В PG18 BUFFERS включается с ANALYZE сам | release notes 18 | ✓ |
| `actual rows` — на один проход, умножать на `loops` | EXPLAIN | ✓ |
| `max_parallel_workers_per_gather` по умолчанию 2 | runtime-config-resource | ✓ |
| Селективность `=` над выражением по умолчанию 0,5% | `selfuncs.h`: `DEFAULT_EQ_SEL = 0.005` | ✓ не 5% |
| Статистика по выражению — с PG14 | release notes 14 | ✓ |
| Skip scan по составному индексу — с PG18 | release notes 18, indexes-multicolumn | ✓ |
| Оценка join по составному FK — с PG 9.6 | release notes 9.6 | ✓ |
| `default_statistics_target` 100, `SET STATISTICS n` | planner-stats | ✓ |
| `work_mem` 4MB, `hash_mem_multiplier` 2.0 с PG15 (раньше 1.0) | runtime-config-resource, release notes 15 | ✓ |
| `enable_*` — грубое временное средство | runtime-config-query | ✓ |
| `random_page_cost` 4.0, `seq_page_cost` 1.0, `effective_cache_size` 4GB | runtime-config-query | ✓ дефолты; 1.1 для SSD — практика, не документация |
| pg_query_state требует патченного ядра | README pg_query_state | ✓ |

## Проверено опытом

2026-10-18, PostgreSQL 18.6 с `pg_stat_statements` (`track = all`), таблица на 200 тыс.
строк.

| Утверждение | Итог |
|---|---|
| `toplevel`: строка верхнего уровня включает вложенные | `select slow()` 301 мс, вложенный `pg_sleep` 300 мс с `toplevel = f` |
| BUFFERS по умолчанию в `EXPLAIN ANALYZE` (PG18) | `Buffers: shared hit=4` без опции |
| skip scan (PG18) | индекс `(a, b)`, фильтр только по `b`: `Index Searches: 4` |
| Равенство над выражением — 0,5% | `date_trunc(...) = ...` и `round(...) = ...`: rows=1000 из 200 000; диапазон по колонке — 1458 при факте 1440 |
| Статистика по выражению чинит оценку | после `CREATE STATISTICS ... ON (date_trunc(...))` — 1440 |
| Составной FK и join по всем его колонкам | без FK 1000 строк при факте 100 000; с FK — 100 000; FK покрыт не целиком — 1000 |
| inner join не убирается, LEFT JOIN убирается | Hash Join против одного Seq Scan; self-join по PK в PG18 убирается |
| Права на логирование | `SET log_min_duration_statement` — `42501` без `GRANT SET`; `LOAD 'auto_explain'` — только суперпользователь; `pg_stat_statements_reset()` — `42501` без права |
| Дефолты | `hash_mem_multiplier` 2, `work_mem` 4MB, `max_parallel_workers_per_gather` 2 |
