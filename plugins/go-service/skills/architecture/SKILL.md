---
name: architecture
description: >
  Сборка Go-сервиса: тонкий main.go, структура Application как единственное место
  связывания зависимостей, пара Run/Shutdown с корректным кодом возврата, остановка в
  бюджете платформы (stop_grace_period, terminationGracePeriodSeconds) и HTTP-стек на
  net/http + http.ServeMux с мидлварью func(http.Handler) http.Handler.
  Wiring a Go service: main.go, Application struct, DI, Run/Shutdown lifecycle, shutdown budget, HTTP stack and middleware shape.
when_to_use: >
  Пишешь или ревьюишь точку входа, связывание зависимостей, graceful shutdown, запуск
  воркеров, выбор HTTP-роутера или форму мидлвари; сервис теряет запросы при деплое или его
  добивает SIGKILL. Триггеры: "main.go", "application struct", "dependency injection", "DI",
  "graceful shutdown", "run shutdown", "wire dependencies", "app wiring", "http router",
  "middleware", "SIGTERM", "SIGKILL", "stop_grace_period", "terminationGracePeriodSeconds",
  "GOMAXPROCS", "automaxprocs", "запуск приложения", "точка входа", "связывание
  зависимостей", "роутер", "мидлварь", "код возврата", "остановка сервиса".
---

# Архитектура Go-сервиса

Версии Go — по фичам: метод и wildcard в шаблонах `ServeMux` и своя переменная цикла на
каждую итерацию — 1.22; `context.AfterFunc` и `context.WithoutCancel` — 1.21; GOMAXPROCS по
лимиту контейнера — 1.25. Проверка утверждений — `CHECKS.md`.

## `cmd/app/main.go` — тонкий, без бизнес-логики

Обязанности: загрузить конфиг, поднять логгер, собрать `Application`, поймать сигналы,
провести `Run` и `Shutdown`.

```go
func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err) // логгера ещё нет: он настраивается из конфига
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.Log.Level}))
	slog.SetDefault(log)
	log.Info("starting application")

	application, err := app.NewApplication(cfg, log)
	if err != nil {
		log.Error("create application", "error", err)
		os.Exit(1)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Первый сигнал запускает остановку и возвращает обработку по умолчанию: второй
	// Ctrl+C убивает зависшую остановку, а не теряется в уже отменённом контексте.
	context.AfterFunc(rootCtx, stop)

	// Run возвращается сам: сигнал отменяет rootCtx, а Run гасит по нему сервер и воркеров.
	runErr := application.Run(rootCtx)
	if errors.Is(runErr, context.Canceled) {
		runErr = nil // штатная остановка по сигналу, не отказ
	}

	// Shutdown — только после возврата Run: он закрывает пул БД, нужный воркерам до конца.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(rootCtx), cfg.App.ShutdownTimeout)
	shutdownErr := application.Shutdown(shutdownCtx)
	cancel()

	if err := errors.Join(runErr, shutdownErr); err != nil {
		log.Error("application terminated", "error", err)
		os.Exit(1)
	}

	log.Info("application shutdown completed")
}
```

Правила:

- **Любой отказ — `os.Exit(1)`.** Голый `return` из `main` даёт код 0, и упавший старт
  выглядит успешным завершением: CI красит деплой зелёным, Job в Kubernetes засчитывается
  выполненным. Под с `restartPolicy: Always` перезапустит контейнер при любом коде, но и
  там выход выглядит не ошибкой, а штатным завершением.
- `os.Exit` не выполняет `defer`. Всё, что нужно закрыть, закрывается в `Shutdown`; если
  `defer`-ов в `main` становится больше одного `stop()`, вынеси тело в `run() error`, а в
  `main` оставь `os.Exit(1)`.
- `panic` на старте тоже даёт ненулевой код (2), но вместо записи в журнале — стек рантайма
  в stderr.
- Ошибка в лог — постоянным сообщением и атрибутом: `log.Error("create application",
  "error", err)`. По постоянному сообщению группируют записи и строят алерты; текст ошибки
  в сообщении каждый раз другой.
- `slog.SetDefault` — сразу после создания логгера: до него `slog.Info` и пакеты, которые
  пишут в глобальный логгер, выводят текст через пакет `log`, без JSON и атрибутов сервиса.
- `ShutdownTimeout` — из конфига, не константа в коде.
- Контекст остановки — `context.WithoutCancel(rootCtx)`: `rootCtx` к этому моменту уже
  отменён, и таймаут, унаследованный от него, истёк бы сразу.
- `context.AfterFunc(rootCtx, stop)` обязателен: с одним `defer stop()` обработчик сигналов
  подключён до конца `main`, второй SIGINT только отменяет уже отменённый контекст, и
  зависшую остановку не прервать иначе как `kill -9`.
- `automaxprocs` не нужен с Go 1.25: рантайм сам берёт минимум из числа CPU, маски affinity
  и **лимита** CPU контейнера — не request: без лимита GOMAXPROCS равен числу CPU узла.
  Дробный лимит округляется вверх, ниже 2 GOMAXPROCS не опускается, лимит перечитывается не
  чаще раза в секунду. Не работает, если в `go.mod` стоит `go 1.24` или ниже, и если
  GOMAXPROCS задан переменной окружения или вызовом `runtime.GOMAXPROCS`.

## `internal/app/` — единственное место связывания зависимостей

`Application` — обычная структура с полями верхнего уровня. `NewApplication` — единственное
место, где собираются зависимости: явные конструкторы, без глобальных фабрик и `init()`.

```go
type Application struct {
	server          *http.Server // либо несколько: публичный, метрики, gRPC
	workers         []Worker
	db              *pgxpool.Pool
	shutdownTimeout time.Duration
}

// Worker — фоновый компонент: Run работает до отмены ctx, Stop дочищает за ним.
type Worker interface {
	Run(ctx context.Context) error
	Stop(ctx context.Context) error
}

func NewApplication(cfg *config.Config, log *slog.Logger) (*Application, error) {
	// 1. Инфраструктура: пул БД, внешние клиенты
	// 2. Репозитории (получают пул)
	// 3. Usecase (получают репозитории)
	// 4. Хендлеры (получают usecase)
	// 5. http.ServeMux, регистрация хендлеров
	// 6. http.Server поверх мидлварей
	// 7. Воркеры
	// 8. Вернуть Application: всё, что потом нужно останавливать, плюс shutdownTimeout
}
```

Правила:

- Зависимости текут сверху вниз через конструкторы: инфраструктура → репозиторий →
  usecase → хендлер. Обратных ссылок нет.
- Ни глобальных переменных, ни `init()`, ни реестров фабрик: порядок инициализации читается
  сверху вниз в одной функции.
- Всё, что нужно останавливать или закрывать, лежит полем в `Application` — иначе до него не
  дотянется `Shutdown`.
- `shutdownTimeout` заполняется из конфига. Забыть его — тихая поломка: с нулём стоппер в
  `Run` даёт серверу нулевой дедлайн. Простаивающий сервер это переживёт (`Shutdown` вернёт
  `nil`), но если в момент SIGTERM шёл запрос, `Run` вернёт `context.DeadlineExceeded`, и
  штатная остановка закончится кодом 1.

## HTTP-стек: net/http + http.ServeMux

Роутер по умолчанию — `http.ServeMux`. С Go 1.22 метод и wildcard пишутся в шаблоне, а на
чужой метод `ServeMux` сам отвечает 405 с заголовком `Allow`. Этим закрыто то, ради чего
обычно берут роутер. gin и echo приносят свой тип контекста и обработчика; chi совместим с
`net/http`, но это ещё одна зависимость. Брать его есть смысл, когда нужны группы маршрутов
со своей мидлварью или ограничения на параметры пути.

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /v1/orders/{id}", h.Get) // id — r.PathValue("id")
mux.HandleFunc("POST /v1/orders/{id}/cancel", h.Cancel)

srv := &http.Server{
	Addr:              cfg.HTTP.Addr,
	Handler:           chain(mux, requestLogger(log), recoverer(log), auth),
	ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout, // например, 5 с
	ReadTimeout:       cfg.HTTP.ReadTimeout,
	WriteTimeout:      cfg.HTTP.WriteTimeout,
	IdleTimeout:       cfg.HTTP.IdleTimeout,
}
```

- **`ReadHeaderTimeout` не опционален.** Ноль у таймаутов `http.Server` означает «без
  таймаута»: соединение, которое начало заголовки и не закончило их, держит горутину и
  дескриптор сколько угодно.
- `WriteTimeout` обрывает и долгие ответы. Стриминг продлевает дедлайн сам:
  `http.NewResponseController(w).SetWriteDeadline`. У захваченного (hijack) соединения
  дедлайны сервера могут остаться — снимает их тот, кто захватил.

Мидлварь — только `func(http.Handler) http.Handler`: любая другая форма не собирается в
цепочку и не переносится между сервисами.

```go
func chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
```

Первый в списке — внешний: он первым видит запрос и последним — ответ.

- **Обёртка `ResponseWriter`** в мидлвари (чтобы узнать статус и размер ответа) реализует
  `Unwrap() http.ResponseWriter`. Без него `http.ResponseController` не достаёт `Hijack` и
  `Flush` сквозь обёртку (`feature not supported`), и вебсокеты со стримингом за мидлварью
  ломаются. Библиотека, которая проверяет `w.(http.Hijacker)` напрямую, `Unwrap` не видит:
  ей нужен метод `Hijack` у самой обёртки.
- **Паника — своей мидлварью**, внутри логирующей, чтобы та записала ответ 500. Без неё
  `net/http` сам поймает панику, но клиент получит оборванное соединение вместо 500, а стек
  уйдёт в `ErrorLog` сервера мимо журнала.

## `Run` и `Shutdown`

```go
func (a *Application) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := a.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	// ListenAndServe про ctx не знает — гасим сервер отсюда.
	g.Go(func() error {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.shutdownTimeout)
		defer cancel()

		return a.server.Shutdown(shutdownCtx)
	})

	for _, w := range a.workers {
		g.Go(func() error { return w.Run(ctx) })
	}

	return g.Wait()
}

func (a *Application) Shutdown(ctx context.Context) error {
	var errs error

	if err := a.server.Shutdown(ctx); err != nil {
		errs = errors.Join(errs, err)
	}

	for i := len(a.workers) - 1; i >= 0; i-- {
		if err := a.workers[i].Stop(ctx); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	// Close ждёт, пока все соединения вернутся в пул: одно утёкшее (незакрытые rows,
	// незавершённая транзакция) повесит его навсегда. Ждём не дольше ctx.
	closed := make(chan struct{})
	go func() {
		a.db.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-ctx.Done():
		errs = errors.Join(errs, fmt.Errorf("close db pool: %w", ctx.Err()))
	}

	return errs
}
```

Правила:

- **Горутина-стоппер обязательна.** `ListenAndServe` блокируется до `Shutdown` или `Close` и
  про контекст ничего не знает. Без стоппера отмена `ctx` — снаружи по сигналу или изнутри
  группы упавшим воркером — гасит воркеров, а `g.Wait()` ждёт сервер, и `Run` не
  возвращается никогда. Тот же эффект даёт `context.AfterFunc`, но с `g.Go` ошибка
  `Shutdown` видна в `g.Wait()`.
- `Shutdown` — только после возврата `Run`. Параллельно нельзя: закрытый пул вернёт ещё
  живым воркерам `closed pool`, и штатный SIGTERM закончится ненулевым кодом.
- Повторный `Shutdown` сервера (из стоппера и из `main`) безопасен: на остановленном сервере
  он возвращается сразу, заново выполняются только хуки `RegisterOnShutdown`. Если первый
  вышел по таймауту, второй снова ждёт те же соединения — см. «Бюджет остановки».
- Воркеры останавливаются в порядке, обратном запуску; пул БД закрывается последним:
  репозитории могут понадобиться воркерам на выходе.
- Ошибки собираются через `errors.Join`, выход на первой запрещён: недоостановленные
  компоненты утекут вместе с процессом.

## Бюджет остановки

Платформа ждёт ограниченное время, потом шлёт SIGKILL: ни `Shutdown`, ни последняя запись в
журнале уже не выполнятся. Лимит в Compose — `stop_grace_period` сервиса (по умолчанию
10 с), в Kubernetes — `terminationGracePeriodSeconds` (по умолчанию 30 с, preStop-хук
выполняется в счёт того же срока).

- **`ShutdownTimeout` расходуется дважды.** Стоппер в `Run` даёт его на дренаж HTTP, `main` —
  ещё раз на `Shutdown`, и медленный запрос ждут оба раза. Худший случай —
  `2 × ShutdownTimeout` плюс время, за которое воркеры выходят из `Run`. Отсюда лимит
  платформы `≥ 2 × ShutdownTimeout + запас` в несколько секунд: при 15 с — не меньше 35 с.
- Лимит платформы задавай явно рядом с деплоем и меняй вместе с `ShutdownTimeout` конфига:
  дефолтные 10 с Compose меньше даже одного 15-секундного таймаута.
- Воркер выходит из `Run` по отмене `ctx`, доделав текущую задачу: этот дренаж ничем не
  ограничен, кроме самого воркера.
