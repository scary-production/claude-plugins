---
name: style
description: Показывает стили строки состояния на примере или включает один из них (full, compact, bars, minimal) в настройках пользователя или проекта.
argument-hint: "[full|compact|bars|minimal] [user|project|local]"
disable-model-invocation: true
---

Выполни одну команду через Bash и покажи пользователю её вывод как есть, в блоке кода,
без пересказа:

```bash
bash "${CLAUDE_PLUGIN_ROOT}/scripts/style.sh" --data "${CLAUDE_PLUGIN_DATA}" --project "${CLAUDE_PROJECT_DIR}" $ARGUMENTS
```

- Без аргументов команда печатает все стили на примере. После вывода спроси, какой
  включить и где: `user` — во всех проектах, `project` — в `.claude/settings.json`
  проекта (уходит в git, увидят все), `local` — только у себя в этом проекте.
- Со стилем команда пишет `statusLine` в файл настроек выбранной области (по умолчанию
  `user`). Скажи, что строка обновится сама в течение нескольких секунд.
- Если команда ответила ошибкой, покажи её и ничего не правь руками.
