# pgx-txmanager — проверка

Файл для того, кто правит скилл: какие утверждения проверены и как их перепроверить.
Claude его не загружает.

## Проверено опытом

2026-10-18: PostgreSQL 18.6, pgx v5.11.0, trm и drivers/pgxv5 v2.0.4, программа на Go
против живой базы.

| Утверждение | Как проверено | Итог |
|---|---|---|
| `Exec` без аргументов выполняет несколько команд; с аргументами — нет | `Exec("select 1; select 2")` и то же с `$1` | ✓ второе: `cannot insert multiple commands into a prepared statement` |
| `Query` без аргументов на simple protocol не переключается | `conn.go`: переключение только при пустой строке запроса | ✓ |
| Вложенный `Do` — та же транзакция | `txid_current()` снаружи и внутри | ✓ совпали |
| `PropagationNested` — savepoint, внешняя транзакция живёт | ошибка во вложенном `DoWithSettings` | ✓ вложенная вставка откатилась, внешняя закоммитилась |
| `MustSettings(settings.Must(), WithTxOptions(...))` и `DoWithSettings` | компиляция и запуск | ✓ |
| Повтор вокруг внешнего `Do` на serializable | две конкурентные брони с пересечением | ✓ read committed: 2 брони (write skew); serializable: 1, вторая получила `40001` и на повторе — конфликт |
| `40001` может прийти на `COMMIT`, `errors.As` его находит | тот же опыт | ✓ «transaction: commit; ... (SQLSTATE 40001)» |
| `23505` обрывает транзакцию | следующая команда в той же транзакции | ✓ `25P02 current transaction is aborted` |
| `ON CONFLICT (id) DO NOTHING RETURNING` на дубле — ноль строк | psql | ✓ |
| `CopyFrom` быстрее батча INSERT | 100 тыс. строк, локально | ✓ ~18 раз (374 мс против 21 мс) |
| `*pgconn.ConnectError` ловится `errors.As` | подключение к закрытому порту | ✓ |
| Дефолты пула: `MaxConns` max(4, NumCPU), `MinConns` 0, Lifetime 1 ч, Idle 30 мин, HealthCheck 1 мин | `pgxpool/pool.go` | ✓ |
| `pgx.Conn` не потокобезопасен | doc-комментарий `conn.go`, `doc.go` | ✓ |
| Своего таймаута дозвона у pgx нет | `pgconn/config.go`: `net.Dialer{}` без `Timeout` | ✓ ~2 мин — это TCP ОС (`tcp_syn_retries = 6`); `ConnectTimeout` ограничивает всё подключение (`pgconn.go`) |
| `PrepareConn` с v5.7.6, `BeforeAcquire` устарел | CHANGELOG, `pgxpool/pool.go` | ✓ |
| Инъекция через dollar-quoted литералы закрыта в v5.9.2; v5.9.1 — порча батча | CHANGELOG pgx | ✓ |
| `awaitDone` убран в trm 2.0.4; 2.0.5 — утечка контекста в `Init` | CHANGELOG trm | ✓ |

## Не вошло

- `*string` для NULL — в скилле принят `pgtype`.
- Ориентир `MaxConns ≈ vCPU × 2–4` и оценки «соединение ~5–10 МБ», «новое соединение
  ~5–20 мс» — без замеров, в скилл не вошли числами.
