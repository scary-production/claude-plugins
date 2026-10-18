# CLAUDE.md

`claude-plugins` — публичный маркетплейс плагинов Claude Code (`.claude-plugin/marketplace.json`,
`plugins/`) и установщик `claude-plugins` в Docker-образе (`cmd/`, `internal/`,
`docker/Dockerfile`). Как пользоваться — `README.md`.

## Правила

- Репозиторий публичный: сюда не попадает ничего из проектов scary-production — имена
  сервисов и продукта, клиенты, внутренние адреса, скиллы сервисов. Только общие
  плагины, которые полезны любому пользователю Claude Code.
- Скилл — пересказ своими словами: без ссылок, имён авторов, названий курсов и докладов,
  ID роликов и хэшей коммитов. Утверждения проверяются опытом или документацией, итог — в
  `CHECKS.md` рядом со `SKILL.md`, без ссылок. Без привязки к своим проектам: внутренние
  библиотеки и сервисы сюда не идут.
- Плагин — каталог `plugins/<имя>/` с `.claude-plugin/plugin.json` и записью в
  маркетплейсе. Версию в `plugin.json` не ставить: версия — коммит, и автообновление
  доставляет каждый коммит в `main`.
- Мод (`hooks/hooks.json` с `modules`) пишется по API модов той версии Claude Code, что
  в `Makefile` (`CLAUDE_CODE_VERSION`): API в раннем доступе. Типы — скилл
  `plugin-authoring`; `$` передаётся только функциям верхнего уровня файла, иначе
  `claude plugin validate` не пропустит модуль. Состояние — в `$.state` с контрактом
  в `types/index.d.ts`.
- Чего плагин сам задать не может (`statusLine`, значения `userConfig`), пишет
  установщик (`internal/install`) и, где можно, скилл плагина. Установщик меняет в
  чужом `settings.json` только свои ключи и сохраняет порядок остальных
  (`internal/settings`).
- Скрипты плагинов — bash и jq, проходят `shellcheck`; вывод хука `SessionStart` без
  JSON попадает модели в контекст, поэтому сообщения человеку — `systemMessage`.
- Комментарии и сообщения — на русском; Go — по скиллам `go-code-style` и `go-testing`,
  файлы до 300 строк.

## Команды

`make ci` (gofmt, vet, тесты, сборка, `claude plugin validate` и `claude plugin test`),
`make lint`, `make shellcheck`, `make image`, `make styles`.
