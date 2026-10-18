# claude-plugins

Маркетплейс плагинов [Claude Code](https://code.claude.com) и установщик в Docker-образе.
Плагин включается для себя во всех проектах или для одного проекта — для всех, кто его
клонирует.

| Плагин | Что делает |
|---|---|
| `statusline` | Строка состояния в четырёх стилях: модель, каталог и ветка git, заполнение контекста, лимиты подписки за 5 часов и 7 дней со временем до сброса |
| `context-guard` | Пороги контекста: на первом модель доводит текущий шаг и не берётся за новое, на втором — завершает сессию и записывает, что осталось. Всплывающее предупреждение о лимитах 5 часов и 7 дней |
| `go-postgres` | Скиллы для Go-сервисов на PostgreSQL: `pgx-txmanager` (pgx и go-transaction-manager), `postgres-concurrency` (гонки, блокировки, изоляция), `postgres-migrations` (миграции, индексы на живых таблицах), `postgres-query-tuning` (тюнинг запросов). Утверждения проверены опытом на PostgreSQL 18 |

Стили строки состояния (`claude-plugins styles` покажет их в цвете):

```text
full     Opus 5.5 │ shop ⎇ main │ ctx 42% │ 5ч ▰▰▰▰▰▰▰▰▱▱ 73% ⟳ 2ч 13м │ 7д 12% ⟳ 3д 4ч
compact  Opus 5.5 · ctx 42% · 5ч 73% · 7д 12%
bars     Opus 5.5 │ shop ⎇ main
         ctx ▰▰▰▰▰▱▱▱▱▱ 42% 420k/1M │ 5ч ▰▰▰▰▰▰▰▰▱▱ 73% ⟳ 2ч 13м │ 7д ▰▰▱▱▱▱▱▱▱▱ 12% ⟳ 3д 4ч
minimal  ctx 42%
```

Цвет — по заполнению: до 70% зелёный, до 90% жёлтый, дальше красный. Лимиты 5 часов и
7 дней есть только у подписки Pro и Max. Если переменная окружения `CLAUDE_LAUNCHER`
задана (её ставит своя обёртка над `claude`), её значение стоит в начале строки.

## Установка через Docker

Установщик правит настройки Claude Code на хосте: каталог `~/.claude` монтируется в
`/claude`, текущий проект — в `/project`.

```bash
alias claude-plugins='docker run --rm -it --user "$(id -u):$(id -g)" \
  -v "$HOME/.claude:/claude" -v "$PWD:/project" ghcr.io/scary-production/claude-plugins'

claude-plugins list                                    # плагины и где они включены
claude-plugins styles                                  # стили строки состояния
claude-plugins install statusline context-guard        # себе, во всех проектах
claude-plugins install statusline -style bars -scope project   # всем в этом проекте
claude-plugins install context-guard -set warn_at=40 -set wrap_at=60
claude-plugins remove statusline
```

Потом перезапустите Claude Code или выполните `/reload-plugins` в открытой сессии.
Маркетплейс и плагины Claude Code скачивает с GitHub в фоне после старта сессии; если
плагинов нет и после `/reload-plugins`, откройте `/plugin` → Marketplaces →
`scary-production` → Update или выполните на хосте
`claude plugin install statusline@scary-production`.

Область `-scope`:

| Область | Файл | Для кого |
|---|---|---|
| `user` (по умолчанию) | `~/.claude/settings.json` | вы, во всех проектах |
| `project` | `.claude/settings.json` проекта | все, кто клонирует проект: файл коммитится |
| `local` | `.claude/settings.local.json` проекта | вы, только в этом проекте |

Что пишет `install`:

- маркетплейс (`extraKnownMarketplaces`, `github:scary-production/claude-plugins` с
  автообновлением) и плагин (`enabledPlugins`). Скачивает их сам Claude Code при
  следующем старте; коллеги по проекту получают их, когда доверяют папке проекта;
- для `statusline` — `statusLine` со стилем (`-style`, по умолчанию прежний или `full`).
  Прежний `statusLine` установщик называет в выводе;
- значения `-set` — в `pluginConfigs` настроек пользователя: только оттуда Claude Code
  их читает. Их же можно менять в `/config`.

Перед правкой `~/.claude/settings.json` и `settings.local.json` установщик кладёт рядом
копию: `*.claude-plugins.bak`. Проектный файл не копирует: он в git. `remove` пишет
`false` в `user` и `local` и убирает ключ из `project`; удалить плагин совсем —
`/plugin` → Installed → Uninstall.

Тем же образом ставятся плагины другого маркетплейса, в том числе приватного: его рабочая
копия монтируется в `/opt/marketplace`, а `-repo` называет репозиторий GitHub, откуда
плагины потом скачает Claude Code:

```bash
docker run --rm -it --user "$(id -u):$(id -g)" -v "$HOME/.claude:/claude" -v "$PWD:/project" \
  -v "$HOME/src/my-plugins:/opt/marketplace:ro" ghcr.io/scary-production/claude-plugins \
  -repo my-org/my-plugins install my-plugin
```

## Установка без Docker

В сессии Claude Code:

```text
/plugin marketplace add scary-production/claude-plugins
/plugin install statusline@scary-production
/statusline:style            — стили на примере
/statusline:style bars user  — включить стиль
```

Плагин сам задать `statusLine` не может: Claude Code берёт из настроек плагина только
`agent` и `subagentStatusLine`. Поэтому `statusLine` пишет установщик или
`/statusline:style`.

Скиллы для Go и PostgreSQL — `/plugin install go-postgres@scary-production` (или
`claude-plugins install go-postgres`). Claude подхватывает их сам по теме разговора:
транзакция, миграция, медленный запрос; вызвать явно — `/go-postgres:pgx-txmanager` и т. п.

## Что нужно на машине

- Claude Code 2.1.302 или новее. `context-guard` — мод: он написан на API модов Claude
  Code, а этот API пока в раннем доступе и меняется между версиями.
- Для `statusline` — `bash` и `jq` (`apt install jq`, `brew install jq`); ветку
  показывает, если есть `git`.

## Как устроено

**statusline.** Скрипт `plugins/statusline/scripts/statusline.sh` читает JSON сессии на
stdin и печатает строку. Каталог установленной версии плагина меняется с каждым
обновлением, поэтому хук `SessionStart` копирует скрипт в постоянный каталог плагина
`~/.claude/plugins/data/statusline-scary-production/`, и `statusLine` ссылается туда:
новая версия скрипта приходит с обновлением плагина, настройки не меняются.

**context-guard.** Мод (`hooks/register.ts`) смотрит заполнение контекста после
каждого хода (`session.measure`) и после каждого вызова инструмента: долгий ход без
участия человека тоже упирается в порог. При переходе порога модели уходит заметка в
разговор — человек её не видит как своё сообщение, — а человеку всплывашка и строка под
полем ввода. После `/compact` порог срабатывает снова. Настройки:

| Поле | По умолчанию | Что значит |
|---|---|---|
| `warn_at` | 50 | первый порог, % контекста |
| `wrap_at` | 75 | второй порог, % контекста |
| `rate_warn_at` | 90 | порог лимита 5 часов или 7 дней, % — одна всплывашка на окно |

**go-postgres.** Только скиллы, без хуков. У каждого рядом с `SKILL.md` лежит `CHECKS.md`:
таблица «проверено опытом» — что и на какой версии PostgreSQL, pgx и
go-transaction-manager проверено. Claude этот файл не загружает, он для того, кто правит
скилл.

## Разработка

```bash
make ci             # gofmt, vet, тесты Go и скриптов, сборка, проверка плагинов Claude Code
make lint           # golangci-lint в контейнере
make shellcheck     # скрипты плагинов
make image styles   # образ локально и стили из него
```

Плагин из рабочей копии без публикации: `claude plugin marketplace add "$PWD"` и
`claude plugin install <плагин>@scary-production` — плагин читается прямо из каталога,
правки подхватывает `/reload-plugins`.

Новый плагин — каталог в `plugins/` с `.claude-plugin/plugin.json`, запись в
`.claude-plugin/marketplace.json`, тесты: мод — `tests/*.test.ts` под
`claude plugin test`, скрипты — Go-тесты в `test/`. Если плагину нужно что-то, чего
плагин сам задать не может, этому учится установщик (`internal/install`).

CI собирает образ на каждый pull request и публикует `ghcr.io/scary-production/claude-plugins`
(`latest` и коммит, amd64 и arm64) из `main`.

## Лицензия

MIT — `LICENSE`.
