---
name: pgx-txmanager
description: >
  Работа с PostgreSQL в Go через pgxpool (pgx v5) и avito-tech/go-transaction-manager:
  пул и устойчивый старт, структуры с тегами db и CollectRows, репозиторий через
  DefaultTrOrDB, транзакции через Do, propagation и savepoint, повтор транзакции при
  40001/40P01/55P03, защита от SQL-инъекций, классификация ошибок и ON CONFLICT, batch
  и CopyFrom. Конкурентная запись и блокировки — скилл postgres-concurrency.
  PostgreSQL in Go via pgxpool and go-transaction-manager: pool, repositories, transactions, retries, errors, bulk loads.
when_to_use: >
  Пишешь или ревьюишь Go-код, который ходит в PostgreSQL: пул, репозиторий, транзакцию,
  повтор транзакции, разбор ошибок драйвера, массовую вставку.
  Триггеры: "как сделать транзакцию", "как описать структуру для БД", "пул соединений",
  "ретрай транзакции", "SQL-инъекция", "pgxpool", "CollectRows", "DefaultTrOrDB",
  "trManager", "savepoint", "batch", "CopyFrom", "pgtype", "ON CONFLICT", "23505",
  "pgx.ErrNoRows", "rows.Close", "transaction", "connection pool".
---

# pgx + go-transaction-manager

**Главное.** Репозиторий берёт соединение через `DefaultTrOrDB`, границы транзакции задаёт
сервис через `trManager.Do`. Значения в запрос — только аргументами `$1, $2`. Ожидаемый
дубль — `ON CONFLICT`, а не пойманный `23505`. На `40001` / `40P01` / `55P03` повторяй всю
транзакцию целиком, цикл — вокруг внешнего `Do`. Что делать с гонками за одни и те же
строки, — скилл `postgres-concurrency`.

Проверка утверждений — `CHECKS.md`.

## Версии

```bash
go get github.com/jackc/pgx/v5@latest
go get github.com/avito-tech/go-transaction-manager/trm/v2@latest
go get github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2@latest
```

- **pgx — не ниже v5.9.2.** В нём закрыта уязвимость: SQL-инъекция через
  dollar-quoted литералы при simple protocol. В v5.9.1 чинили порчу результатов батча при
  кешированных подготовленных запросах. Последнюю патч-версию смотри через
  `go list -m -u github.com/jackc/pgx/v5`.
- **trm и драйвер pgxv5 — не ниже v2.0.4, лучше v2.0.5.** В 2.0.4 из драйвера убрали
  фоновую горутину `awaitDone`: отмена контекста больше не делает rollback из другой
  горутины (`pgx.Tx` не потокобезопасен, на выполняющемся запросе это давало панику).
  Rollback теперь выдаётся после возврата из транзакционной функции. В 2.0.5 закрыта утечка
  контекста в `manager.Init` на ошибках. Модуль версионируется тегами вида `trm/v2.0.5`.

## Пул

```go
cfg, err := pgxpool.ParseConfig(dsn)
if err != nil {
	return nil, fmt.Errorf("parse dsn: %w", err)
}
cfg.MaxConns = 25 // сколько выдержит база, а не сколько ядер у пода
cfg.MinConns = 2
cfg.ConnConfig.ConnectTimeout = 5 * time.Second
pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
```

- **`MaxConns` задавай явно.** Дефолт — `max(4, runtime.NumCPU())`, то есть число ядер
  машины, а размер пула должен зависеть от базы. Сумма `MaxConns` всех сервисов и реплик
  укладывается в `max_connections` (по умолчанию 100): каждое соединение — отдельный
  процесс PostgreSQL.
- Остальные дефолты pgxpool: `MinConns` 0, `MaxConnLifetime` 1 ч, `MaxConnIdleTime` 30 мин,
  `HealthCheckPeriod` 1 мин. Укорачиваешь `MaxConnIdleTime` — держи `MinConns` ≥ 2, иначе
  после простоя запрос заплатит за новое соединение.
- **`MaxConnLifetime` не лечит зависшие запросы.** От них — `statement_timeout` и
  `context.WithTimeout` на запрос: исчерпанный пул вернёт `DeadlineExceeded`, а не повиснет.
- **В сервисе — только `pgxpool`.** `pgx.Conn` не потокобезопасен: из нескольких горутин
  получишь `conn busy` и гонку данных. `pgx.Conn` — для LISTEN/NOTIFY на выделенном
  соединении и одноразовых скриптов.
- **`NewWithConfig` к базе не подключается**: пул ленивый. Недоступность базы на старте
  видна только на первом `Ping`. Без таймаута дозвона pgx ждёт TCP-таймаут ОС (на Linux
  около двух минут). Ping с повтором, классификатор сетевых ошибок, хуки `PrepareConn` и
  `AfterRelease` — [`references/pool-startup.md`](references/pool-startup.md).

## Менеджер транзакций и репозиторий

```go
import (
	trmpgxv5 "github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

trManager := manager.Must(trmpgxv5.NewDefaultFactory(pool))
```

Репозиторий не знает, идёт ли он в транзакции: `DefaultTrOrDB` отдаёт транзакцию из
контекста, если она есть, иначе пул.

```go
type UserRepository struct {
	pool   *pgxpool.Pool
	getter *trmpgxv5.CtxGetter
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool, getter: trmpgxv5.DefaultCtxGetter}
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (User, error) {
	rows, err := r.getter.DefaultTrOrDB(ctx, r.pool).Query(ctx,
		`SELECT id, name, email, bio FROM users WHERE id = $1`, id)
	if err != nil {
		return User{}, fmt.Errorf("select user %d: %w", id, err)
	}
	u, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[User])
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("user %d: %w", id, ErrNotFound)
	}
	return u, err
}
```

## Структуры и чтение строк

- Обычный `Scan` по тегам не маппит. Стандарт — тег `db:"..."` и
  `pgx.RowToStructByName[T]`; список — `pgx.CollectRows`, одна строка — `pgx.CollectOneRow`
  (на пустой выборке — `pgx.ErrNoRows`). `rows.Close()` и `rows.Err()` они делают сами.
- Читаешь вручную через `rows.Next()` (с ранним выходом) — сразу после `Query` ставь
  `defer rows.Close()`. Незакрытые rows держат соединение: при `MaxConns=3` после трёх
  утечек четвёртый запрос висит до таймаута контекста.
- Nullable-колонка — `pgtype.Text`, `pgtype.Timestamptz` и т. п.: `Valid == false` — это
  NULL и при чтении, и при записи. Слайсы связей (`Orders []Order`) — без тега, второй
  запрос и сборка руками.

```go
type User struct {
	ID    int64       `db:"id"`
	Name  string      `db:"name"`
	Email string      `db:"email"`
	Bio   pgtype.Text `db:"bio"` // nullable
}
```

## Параметры запроса — только `$1, $2`

Любое значение — телефон, имя, строку поиска — передавай аргументом `$N`. Не собирай SQL
через `+` или `fmt.Sprintf`, даже если строка выглядит безопасной.

- `Exec` без аргументов pgx всегда шлёт по **simple protocol** (в `conn.go`: «Always use
  simple protocol when there are no arguments»), а simple protocol выполняет несколько
  команд подряд: `'; DELETE FROM ...; --` пройдёт. `Exec` с аргументами вторую команду
  отвергнет (`cannot insert multiple commands into a prepared statement`). `Query` без
  аргументов на simple protocol не переключается, но клеить строки нельзя и в нём.
- Extended protocol от инъекции не спасает: `UNION SELECT` — один запрос, и через `Query`
  он проходит. Так утекают персональные данные.
- Имена колонок, `ORDER BY` и направление сортировки аргументом не передать. Бери их из
  белого списка: `map` от значения из API к готовому фрагменту SQL, на неизвестное —
  ошибка.

## Транзакция через Do

```go
func (s *UserService) CreateWithProfile(ctx context.Context, name, email, bio string) error {
	return s.trManager.Do(ctx, func(ctx context.Context) error {
		userID, err := s.users.Create(ctx, name, email)
		if err != nil {
			return err
		}
		return s.profiles.Create(ctx, userID, bio)
	}) // ошибка — rollback, nil — commit
}
```

**Вложенный `Do` присоединяется к открытой транзакции, а не создаёт savepoint.** Дефолтная
propagation — `PropagationRequired`: ошибка во вложенном вызове откатывает всё целиком.
Частичный откат проси явно — `PropagationNested`, драйвер сделает `SAVEPOINT`:

```go
nested := settings.Must(settings.WithPropagation(trm.PropagationNested))

return s.trManager.Do(ctx, func(ctx context.Context) error {
	if err := s.repo.Save(ctx, u); err != nil {
		return err
	}
	// Ошибка откатится до savepoint, внешняя транзакция продолжится.
	if err := s.trManager.DoWithSettings(ctx, nested, func(ctx context.Context) error {
		return s.repo.SaveOptionalPart(ctx, p)
	}); err != nil {
		s.log.Warn("optional part skipped", "err", err)
	}
	return nil
})
```

Частичный откат не нужен — не усложняй: вложенного `Do` достаточно, просто он ничего не
изолирует.

## Повтор транзакции

Когда повтор нужен (repeatable read и serializable, deadlock, `lock_timeout`, `NOWAIT`), —
скилл `postgres-concurrency`. Здесь — как это сделать в Go.

- Повторяй **всю транзакцию**, вместе с чтением и логикой, которая решает, что писать.
  Повтор последнего оператора бесполезен: транзакция уже оборвана.
- Цикл — вокруг **внешнего** `Do`. Вложенный `Do` присоединяется к открытой транзакции, и
  повтор внутри него новую не начнёт.
- `40001` может прийти и на `COMMIT`: менеджер вернёт его обёрнутым («transaction:
  commit; ...»), `errors.As` его найдёт. Это ещё одна причина держать в цикле весь `Do`.
- Попытки ограничь, между ними — небольшая растущая пауза.

```go
var serializable = trmpgxv5.MustSettings(
	settings.Must(),
	trmpgxv5.WithTxOptions(pgx.TxOptions{IsoLevel: pgx.Serializable}),
)

func isRetryableTxErr(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01", "55P03": // serialization_failure, deadlock_detected, lock_not_available
		return true
	}
	return false
}

func (s *BookingService) Book(ctx context.Context, req BookRequest) error {
	const attempts = 3
	var err error
	for i := range attempts {
		err = s.trManager.DoWithSettings(ctx, serializable, func(ctx context.Context) error {
			return s.book(ctx, req) // всё чтение и все решения — внутри
		})
		if !isRetryableTxErr(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * 20 * time.Millisecond):
		}
	}
	return err
}
```

## Ошибки

Разбирай ошибки драйвера в репозитории, наружу отдавай доменные: usecase не должен знать
про pgx.

```go
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
	return 0, fmt.Errorf("create user %q: %w", email, ErrConflict)
}
```

**Ожидаемый дубль — `ON CONFLICT`, а не пойманный `23505`.** Любая ошибка внутри `Do`,
включая `23505`, обрывает транзакцию: дальше каждая команда получит `25P02` («current
transaction is aborted»). Если дубль — штатный исход (повторная отправка, двойной клик):

```go
err := q.QueryRow(ctx,
	`INSERT INTO users (name, email) VALUES ($1, $2)
	 ON CONFLICT (email) DO NOTHING
	 RETURNING id`, name, email,
).Scan(&id)
if errors.Is(err, pgx.ErrNoRows) { // не создано: email уже занят
	return 0, fmt.Errorf("create user %q: %w", email, ErrConflict)
}
```

- Цель конфликта `(колонки)` указывай всегда. Без неё `DO NOTHING` глушит нарушение любого
  уникального ограничения, и какое сработало, уже не узнать.
- Ноль строк в `RETURNING` — запись не создана: строка уже есть или её только что
  зафиксировала параллельная транзакция. Это не успех.
- Нарушение **другого** уникального ограничения всё равно придёт как `23505` — различай по
  `pgErr.ConstraintName`.
- Уникальность держи индексом или ограничением, а не SELECT перед INSERT: между ними
  параллельная транзакция успеет занять значение.

Частые коды: `23505` unique_violation, `23503` foreign_key_violation, `23502`
not_null_violation, `23P01` exclusion_violation, `40001` serialization_failure, `40P01`
deadlock_detected, `55P03` lock_not_available. Повторять имеет смысл три последних, и
только всю транзакцию.

## Batch и CopyFrom

- **`pgx.Batch`** — несколько **разных** запросов за один раундтрип. После `SendBatch`
  прочитай результат каждого запроса и закрой `BatchResults`.
- **`CopyFrom`** — массовая загрузка по протоколу `COPY`: разбор один раз, а не на каждую
  строку. На 100 тыс. строк он быстрее батча из INSERT примерно на порядок.
- `CopyFrom` не умеет `ON CONFLICT` и не возвращает id. Нужен upsert или `RETURNING` —
  грузи `CopyFrom` во временную таблицу и делай `INSERT ... SELECT ... ON CONFLICT`.

```go
n, err := r.getter.DefaultTrOrDB(ctx, r.pool).CopyFrom(ctx,
	pgx.Identifier{"orders"},
	[]string{"user_id", "total"},
	pgx.CopyFromSlice(len(orders), func(i int) ([]any, error) {
		return []any{orders[i].UserID, orders[i].Total}, nil
	}),
)
```

## Шпаргалка

| Задача | Инструмент |
|---|---|
| Старт при недоступной базе | `ConnConfig.ConnectTimeout` + Ping с повтором (`references/pool-startup.md`) |
| Список / одна запись | `CollectRows` / `CollectOneRow` + `RowToStructByName` |
| Ручной `rows.Next()` | `defer rows.Close()` сразу после `Query` |
| Запрос в репозитории | `getter.DefaultTrOrDB(ctx, pool)` |
| Значения из ввода | только `$1, $2`; колонки и `ORDER BY` — белый список |
| Транзакция | `trManager.Do(ctx, fn)` |
| Частичный откат | `DoWithSettings` + `trm.PropagationNested` |
| Уровень изоляции | `trmpgxv5.MustSettings(settings.Must(), trmpgxv5.WithTxOptions(...))` |
| `40001` / `40P01` / `55P03` | повтор всей транзакции вокруг внешнего `Do` |
| Нет строки / дубль | `errors.Is(err, pgx.ErrNoRows)` / `PgError.Code == "23505"` |
| Дубль — штатный исход | `INSERT ... ON CONFLICT (cols) DO NOTHING RETURNING id` |
| Массовая загрузка | `CopyFrom`; разные запросы разом — `pgx.Batch` |
| Nullable | `pgtype.Text` / `Timestamptz` / ... |
| Гонки за строки, блокировки, очереди | скилл `postgres-concurrency` |
