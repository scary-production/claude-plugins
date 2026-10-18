# Устойчивое подключение к PostgreSQL на старте

Читать, когда пишешь инициализацию пула и хочешь, чтобы приложение переживало
недоступность базы в момент запуска: под стартовал раньше базы, failover, сетевой блип.

`pgxpool.NewWithConfig` **не подключается сразу**: пул ленивый, prefill и health-check
уходят в фоновую горутину. Дозвон форсирует **первый `Ping()`** (или первый запрос).
Значит, единственная точка отказа на старте — первый `Ping`.

- **В рантайме** пул восстанавливается сам: при следующем `Acquire` он дозванивается
  заново, мёртвые соединения выбрасывают хуки `PrepareConn` и `AfterRelease` (ниже).
- **На старте** без повтора один временный `connection refused` роняет процесс. Чинить
  нужно только старт.

Две вещи:
1. **Таймаут дозвона.** Своего дефолта у pgx нет: без `connect_timeout` в DSN или
   `ConnConfig.ConnectTimeout` дозвон ограничен только контекстом и TCP-таймаутом ОС. На
   Linux это около двух минут (`net.ipv4.tcp_syn_retries = 6`), и недоступный адрес
   повесит старт на столько же. `ConnectTimeout` ограничивает всё подключение целиком.
2. **Повтор `Ping` с ограниченным backoff.** Переждать блип, но не висеть вечно: бюджет
   исчерпан — выходим с ошибкой, и решение принимает оркестратор.

```go
const (
	connectDialTimeout    = 5 * time.Second // потолок одного подключения
	connectInitialBackoff = 1 * time.Second
	connectMaxBackoff     = 5 * time.Second
	connectMaxWait        = 90 * time.Second // общий бюджет; обычный failover укладывается
)

func InitDB(dsn string, log *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	// ... MaxConns, MinConns, MaxConnLifetime ...
	poolCfg.ConnConfig.ConnectTimeout = connectDialTimeout

	// NewWithConfig не дозванивается. Background — чтобы фоновый prefill и health-check
	// не отменились, когда InitDB вернётся: короткий ctx с defer cancel сюда не передавай.
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	if err := pingWithRetry(pool, log); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func pingWithRetry(pool *pgxpool.Pool, log *slog.Logger) error {
	deadline := time.Now().Add(connectMaxWait)
	delay := connectInitialBackoff
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), connectDialTimeout)
		err := pool.Ping(ctx)
		cancel()
		if err == nil {
			return nil
		}
		if !isRetryableConnErr(err) { // неверный пароль или база — сразу, бюджет не жжём
			return fmt.Errorf("ping: %w", err)
		}
		if time.Now().Add(delay).After(deadline) {
			return fmt.Errorf("ping: budget exhausted: %w", err)
		}
		log.Warn("db connect failed, retrying", "attempt", attempt, "delay", delay, "err", err)
		time.Sleep(delay)
		delay = min(delay*2, connectMaxBackoff)
	}
}
```

**Классификатор временных ошибок.** Повторяем только сеть и дозвон. На ошибках
конфигурации (пароль, имя базы) падаем сразу, иначе бюджет сгорит на опечатке:

```go
func isRetryableConnErr(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "28P01", "28000", "3D000", "3F000": // пароль, авторизация, база, схема
			return false
		}
		return strings.HasPrefix(pgErr.Code, "08") || pgErr.Code == "57P03" // connection_exception, cannot_connect_now
	}
	var connErr *pgconn.ConnectError // оборачивает дозвон, в том числе «connection refused»
	if errors.As(err, &connErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}
```

> Ошибка вида ``failed to connect to `user=… database=…`: … connect: connection refused``
> — это `*pgconn.ConnectError`, ловится через `errors.As`.

**Гигиена соединений в пуле.** `PrepareConn` выбрасывает мёртвые соединения (например,
после failover) на каждом `Acquire`, поэтому длинный `MaxConnLifetime` безопасен. Хук есть с
pgx v5.7.6:

```go
poolCfg.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
	if conn.IsClosed() {
		return false, nil // false и nil — выбросить соединение, запрос уйдёт на новое
	}
	if err := conn.Ping(ctx); err != nil {
		return false, nil
	}
	return true, nil
}
poolCfg.AfterRelease = func(conn *pgx.Conn) bool {
	return !conn.IsClosed()
}
```

> `pgxpool.Config.BeforeAcquire` устарел, его заменил `PrepareConn`. Если заданы оба,
> `BeforeAcquire` игнорируется.
