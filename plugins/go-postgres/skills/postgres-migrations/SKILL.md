---
name: postgres-migrations
description: >
  SQL-миграции PostgreSQL: парные up/down-файлы, современные типы и ограничения
  (identity, text с CHECK, timestamptz, jsonb), откат каждой операции, добавление
  значения в enum, индексы на живых таблицах через CREATE INDEX CONCURRENTLY без
  блокировки записи, lock_timeout перед ALTER на живой таблице, что требовать от
  мигратора, разбор «индекс создан, а план — Seq Scan».
  PostgreSQL migrations: paired up/down files, modern types, reversible changes, lock_timeout for DDL, non-blocking indexes, migrator requirements.
when_to_use: >
  Создаёшь таблицу, меняешь схему, пишешь up/down-миграцию или индекс; новый индекс не
  используется планировщиком; миграция с CONCURRENTLY падает.
  Триггеры: "миграция", "migration", "create table", "alter table", "создай таблицу",
  "добавь колонку", "schema", "up.sql", "down.sql", "индекс", "create index",
  "concurrently", "invalid index", "indisvalid", "enum", "add value", "seq scan",
  "индекс не используется", "lock_timeout", "access exclusive", "миграция повесила
  таблицу".
---

# Миграции PostgreSQL

**Главное.** Каждая миграция — пара up/down, и down полностью откатывает up. Миграция
выполняется в своей транзакции. Исключение одно: индекс на таблице, где уже есть данные и
идёт запись, — `CREATE INDEX CONCURRENTLY` в отдельной миграции без транзакции, одна
команда на файл.

Проверка утверждений — `CHECKS.md`.

## Формат файлов

```
migrations/
├── 20250524120000_create_users.up.sql
├── 20250524120000_create_users.down.sql
├── 20250525090000_add_users_phone.up.sql
└── 20250525090000_add_users_phone.down.sql
```

- Префикс — UTC-время `YYYYMMDDHHmmss`, имя — краткое описание в snake_case.
- `*.up.sql` применяет, `*.down.sql` откатывает. Оба файла обязательны.
- Применённую миграцию не правь — пиши новую.

## Типы и ограничения

| Вместо | Пиши | Почему |
|---|---|---|
| `bigserial` | `bigint GENERATED ALWAYS AS IDENTITY` | стандарт SQL; `BY DEFAULT`, если нужно вставлять свои id (импорт) |
| `varchar(n)` | `text NOT NULL CHECK (length(x) <= n)` | лимит — правило данных, менять его проще |
| `timestamp` | `timestamptz` | хранит момент времени, а не «часы на стене» |
| `integer`-флаг | `boolean` | |
| `json` | `jsonb` | бинарный формат, индексы и операторы |
| uuid из расширения | `uuid DEFAULT gen_random_uuid()` | встроена с PostgreSQL 13 |

Длину текстовой колонки ограничивай `CHECK` всегда, кроме заведомо длинного текста
(описание, комментарий). Неочевидна максимальная длина — спроси пользователя.

| Данные | Ограничение |
|---|---|
| email | `CHECK (length(email) <= 320)` |
| имя | `CHECK (length(name) <= 255)` |
| телефон | `CHECK (length(phone) <= 50)` |
| slug, код | `CHECK (length(code) <= 100)` |
| URL | `CHECK (length(url) <= 2048)` |
| описание | без ограничения или `CHECK (length(bio) <= 10000)` |

Шаблон таблицы:

```sql
CREATE TABLE users (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       text        NOT NULL CHECK (length(name) <= 255),
    email      text        NOT NULL CHECK (length(email) <= 320),
    is_active  boolean     NOT NULL DEFAULT true,
    metadata   jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_users_email ON users (email);
```

## Откат

| up | down |
|---|---|
| `CREATE TABLE t` | `DROP TABLE IF EXISTS t` |
| `ALTER TABLE t ADD COLUMN c` | `ALTER TABLE t DROP COLUMN IF EXISTS c` |
| `CREATE INDEX ix` | `DROP INDEX IF EXISTS ix` |
| `CREATE INDEX CONCURRENTLY ix` | `DROP INDEX CONCURRENTLY IF EXISTS ix` — тоже без транзакции |
| `CREATE TYPE t AS ENUM (...)` | `DROP TYPE IF EXISTS t` |
| `ALTER TYPE t ADD VALUE 'v'` | `DROP VALUE` в PostgreSQL нет — пересоздание типа или пустой down |
| `ALTER TABLE t ALTER COLUMN c SET NOT NULL` | `ALTER TABLE t ALTER COLUMN c DROP NOT NULL` |
| `ALTER TABLE t ADD CONSTRAINT c` | `ALTER TABLE t DROP CONSTRAINT IF EXISTS c` |

`IF EXISTS` в down — чтобы откат был идемпотентным.

## Типовые изменения

**ALTER на живой таблице — с `lock_timeout`.** Многие формы `ALTER TABLE` (и `DROP TABLE`,
`TRUNCATE`, `VACUUM FULL`, `REINDEX`, `CLUSTER`) берут `ACCESS EXCLUSIVE`, даже мгновенные.
Ждущий такой замок встаёт в очередь, и все новые запросы к таблице, даже `SELECT`,
становятся за ним: одна долгая транзакция или «idle in transaction» плюс миграция
останавливают всю таблицу. Короткий `lock_timeout` превращает ожидание в ошибку `55P03`, и
миграцию можно повторить позже. Перед запуском посмотри долгие транзакции в
`pg_stat_activity` (`state = 'idle in transaction'`, старый `xact_start`).

```sql
SET LOCAL lock_timeout = '3s'; -- миграция в транзакции: действует до её конца
ALTER TABLE orders ADD COLUMN note text;
```

**Колонка с дефолтом.** С константным `DEFAULT` таблица не переписывается (PostgreSQL 11+),
команда мгновенная. С volatile-дефолтом (`gen_random_uuid()`, `clock_timestamp()`) таблица
переписывается целиком под эксклюзивной блокировкой — на большой живой таблице добавь
колонку без дефолта и заполняй пачками.

```sql
ALTER TABLE users ADD COLUMN role text NOT NULL DEFAULT 'user' CHECK (length(role) <= 50);
```

**Внешний ключ** — индекс по ссылающейся колонке заводи сразу: PostgreSQL его сам не
создаёт, а без него удаление родителя и join идут перебором.

```sql
CREATE TABLE orders (
    id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users (id),
    total   numeric NOT NULL
);
CREATE INDEX ix_orders_user_id ON orders (user_id);
```

**Значение в enum.** `ALTER TYPE ... ADD VALUE` работает в транзакции, но новое значение
нельзя использовать до коммита (`55P04 unsafe use of new value`). Если мигратор
выполняет миграцию в транзакции, добавляй значение и используй его (UPDATE, DEFAULT,
CHECK) в **разных** миграциях.

```sql
ALTER TYPE payment_method ADD VALUE 'bank_transfer';
```

down: `DROP VALUE` нет. Значение безвредно — оставь down пустым с комментарием; нет —
пересоздай тип (новый тип, `ALTER COLUMN ... TYPE ... USING`, удаление старого).

## Индексы на живых таблицах

Таблица уже с данными и в неё пишут — индекс через `CREATE INDEX CONCURRENTLY` (CIC).
Обычный `CREATE INDEX` блокирует запись на всё время сборки. Индекс вместе с новой
таблицей в той же миграции — обычный `CREATE INDEX`. Размер базы не спасает: висящая
транзакция ломает новый индекс при любом размере (ниже).

up:
```sql
CREATE INDEX CONCURRENTLY ix_orders_customer_id ON orders (customer_id);
```

down:
```sql
DROP INDEX CONCURRENTLY IF EXISTS ix_orders_customer_id;
```

- **Отдельная миграция без транзакции, одна команда в файле.** В блоке транзакции CIC
  запрещён («cannot run inside a transaction block»). Несколько команд, отправленных одним
  запросом, PostgreSQL выполняет неявной транзакцией, и CIC падает с той же ошибкой.
- **Мигратор не умеет миграцию без транзакции — не клади CIC в обычную миграцию.** Скажи
  пользователю, что нужен этот режим (раздел «Что требовать от мигратора»), или выполни CIC
  отдельно и запиши миграцию вручную.
- **CIC ждёт все транзакции со старым снимком**, даже те, что читают другие таблицы.
  Миграция может идти долго.
- **После применения проверь индекс.** CIC не транзакционный: при сбое остаётся
  INVALID-индекс, а сама миграция этого не покажет.

```sql
SELECT c.relname, i.indisvalid, i.indcheckxmin
FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
WHERE c.relname = 'ix_orders_customer_id';
```

- `indisvalid = false` — `DROP INDEX CONCURRENTLY IF EXISTS` и создай заново, либо
  `REINDEX INDEX CONCURRENTLY`.
- **Повтор упавшей CIC-миграции.** Без `IF NOT EXISTS` она упадёт с «relation already
  exists» на INVALID-индексе. С `IF NOT EXISTS` — хуже: команда молча пропустит
  INVALID-индекс, и миграция «пройдёт». Поэтому `IF NOT EXISTS` в CIC не пиши: сначала
  убери невалидный индекс, потом повторяй.

### Индекс есть, а план — Seq Scan

Индекс валиден, а планировщик его не берёт — проверь `indcheckxmin` (запрос выше). `true`:
запросы не используют индекс, пока живут транзакции, начатые до его создания. Причина —
UPDATE: в таблице остались цепочки HOT-версий, на которые индекс не ссылается. Одни INSERT
такого не дают. Не пересоздавай индекс и не ищи причину в статистике. Найди старые
транзакции:

```sql
SELECT pid, xact_start, backend_xmin, state, left(query, 60) AS query
FROM pg_stat_activity
WHERE backend_xmin IS NOT NULL
ORDER BY xact_start;
```

Дождись их завершения или прекрати (`pg_terminate_backend(pid)`) — только после
согласования с пользователем. Воспроизвести перед релизом: в одной сессии открытая
транзакция repeatable read с чтением таблицы, в другой — UPDATE строк и `CREATE INDEX`;
`indcheckxmin` станет `true`.

## Что требовать от мигратора

- **Каждая миграция — своя транзакция**, вместе с записью в журнал применённых: упавшая
  откатывается целиком, уже применённые остаются.
- **Блокировка от параллельного запуска** (например, advisory lock): реплики, стартовавшие
  вместе, не накатывают одно и то же дважды.
- **Режим без транзакции для отдельной миграции** — пометка в файле (например, первая строка
  `-- migrate:no-transaction`). Такой файл отправляется одной командой, мимо транзакции.
  Цена: применение и запись в журнал уже не атомарны. Упал процесс между ними — повтор
  упадёт на «already exists»; проверь `indisvalid`, при валидном индексе запиши миграцию в
  журнал вручную.
- **Создание пары файлов без подключения к базе.**
