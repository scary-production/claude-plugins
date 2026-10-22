# architecture — проверка

Файл для того, кто правит скилл: какие утверждения проверены и как их перепроверить.
Claude его не загружает.

## Сверено с документацией

| Утверждение | Раздел | Итог |
|---|---|---|
| `os.Exit` не выполняет `defer` | `os`, `Exit` | ✓ «deferred functions are not run» |
| До `slog.SetDefault` глобальный логгер пишет текстом через пакет `log` | `log/slog`, `Default`, `SetDefault`; исходник Go 1.27.2 (`defaultHandler`) | ✓ |
| `stop()` от `NotifyContext` снимает подписку на сигналы, до него канал подписан | `os/signal`, `NotifyContext`; исходник Go 1.27.2 | ✓ `Stop(c.ch)` — только в `stop()` |
| GOMAXPROCS по умолчанию — минимум из CPU, affinity и лимита cgroup; лимит, не request; округление вверх; не ниже 2; обновление не чаще раза в секунду; выключено переменной окружения, вызовом и `go 1.24` в `go.mod` | `runtime`, документация `GOMAXPROCS`; исходник Go 1.27.2 (`adjustCgroupGOMAXPROCS`) | ✓ |
| Ноль у `ReadTimeout`, `ReadHeaderTimeout`, `WriteTimeout` — без таймаута | `net/http`, `Server` | ✓ |
| У захваченного соединения могут остаться дедлайны сервера, снимает их вызывающий | `net/http`, `Hijacker` | ✓ |
| `ResponseController` идёт к исходному `ResponseWriter` через `Unwrap` | `net/http`, `ResponseController` | ✓ |
| Паника в хендлере: сервер её ловит, пишет стек в `ErrorLog` и закрывает соединение | `net/http`, `Handler` | ✓ |
| `Shutdown` ждёт перехода соединений в idle, пока не истечёт ctx | `net/http`, `Server.Shutdown` | ✓ |
| `pgxpool.Pool.Close` ждёт возврата всех соединений | godoc pgx v5.11.0 | ✓ «Blocks until all connections are returned to pool and closed» |
| `restartPolicy: Always` перезапускает контейнер после любого завершения, и с кодом 0 | Kubernetes, Pod Lifecycle → Container restart policy | ✓ |
| Kubernetes ждёт 30 с по умолчанию, preStop — до SIGTERM, по истечении срока — SIGKILL | Kubernetes, Pod Lifecycle → Termination of Pods | ✓ |
| `stop_grace_period` в Compose — 10 с по умолчанию, перед SIGKILL — SIGTERM | Compose file reference, services | ✓ |
| chi — обработчики и мидлварь `net/http` | README chi | ✓ |

## Проверено опытом

2026-10-22, Go 1.27.2, x/sync errgroup, pgx v5.11.0, PostgreSQL 18.6, Docker 29.9.1 на
cgroup v2.

| Утверждение | Как проверено | Итог |
|---|---|---|
| `return` из `main` после отказа — код 0 | программа печатает ошибку и возвращается | ✓ код 0 |
| `os.Exit(1)` не выполняет `defer` | `defer fmt.Println` перед `os.Exit(1)` | ✓ код 1, вывода нет |
| `panic` на старте — код 2 и стек | `panic` в `main` | ✓ код 2, `panic: …` в stderr |
| С одним `defer stop()` второй SIGINT проглатывается | `NotifyContext`, остановка 3 с, два SIGINT через 0,5 с | ✓ процесс доработал, код 0 |
| `context.AfterFunc(rootCtx, stop)` возвращает обработку по умолчанию | то же | ✓ второй SIGINT убил процесс, код 130 |
| GOMAXPROCS следует лимиту контейнера, а не request | бинарь в контейнере: `--cpus=2`; только `--cpu-shares=512` | ✓ 2; 12 — все CPU узла |
| Дробный лимит округляется вверх, ниже 2 не опускается | `--cpus=1.5`, `2.5`, `1`, `0.5` | ✓ 2, 3, 2, 2 |
| `go 1.24` в `go.mod` выключает учёт лимита | тот же код, `go 1.24`, `--cpus=2` | ✓ 12 |
| Лимит перечитывается на ходу | `--cpus=4`, через 2,5 с `docker update --cpus=1` | ✓ 4 → 2 за ≈1–2 с |
| Нулевой таймаут стоппера: простаивающий сервер — `nil`, запрос в работе — ошибка | `Shutdown` с истёкшим ctx без запроса и с запросом на 2 с | ✓ `nil`; `context deadline exceeded` |
| Без стоппера `Run` не возвращается после отмены ctx | `errgroup`: `Serve` и воркер, `cancel()` | ✓ не вернулся за 2 с |
| Запрос к закрытому пулу — `closed pool` | `pool.Close()`, затем `Exec` | ✓ `closed pool` |
| Повторный `Shutdown` возвращается сразу, хуки `RegisterOnShutdown` — снова | два `Shutdown` подряд | ✓ оба `nil` за 0 мс, хук вызван 2 раза |
| Медленный запрос съедает `ShutdownTimeout` дважды | `Run` и повторный `Shutdown` из скилла, T = 1 с, хендлер спит 10 с | ✓ `Run` 1 с + `Shutdown` 1 с, оба `deadline exceeded` |
| `pgxpool.Pool.Close` висит при невозвращённом соединении | `Acquire` без `Release`, `Close` в горутине | ✓ висит дольше 3 с |
| Чужой метод на маршруте с методом в шаблоне — 405 и `Allow` | `GET` на `POST /v1/orders/{id}` | ✓ 405, `Allow: POST`; `POST` — 200, `PathValue` = 42 |
| Первый мидлварь в `chain` — внешний | `chain(h, A, B)` с трассой | ✓ `A> B> handler <B <A` |
| Без `ReadHeaderTimeout` недописанные заголовки держат соединение | сырое соединение шлёт половину заголовков | ✓ 0 — открыто через 3 с; 1 с — сервер закрыл через 1 с |
| Обёртка без `Unwrap` прячет `Hijack` от `ResponseController`; утверждение типа `Unwrap` не видит | обёртка без и с `Unwrap` | ✓ `feature not supported` / `nil`; `w.(http.Hijacker)` — `false` в обоих |
| Паника без своей мидлвари — оборванное соединение у клиента | хендлер с `panic` | ✓ клиент: `EOF`; `ErrorLog`: `http: panic serving …` |

## Не вошло

- Глобальный «закрыватель» с `Add`/`CloseAll` — против `Application` как единственного места
  связывания: порядок закрытия зависел бы от порядка первого обращения.
- Отдельный таймаут на каждую зависимость и лог длительности каждой остановки — нет опыта,
  каким должен быть код.
- Тест остановки (медленный хендлер, отмена `rootCtx` во время запроса, ответ 200) — не
  написан.
- Readiness и задержка в preStop, чтобы под успел выйти из балансировки до SIGTERM.
- Хвост для echo (`WrapMiddleware`) — не проверен.
