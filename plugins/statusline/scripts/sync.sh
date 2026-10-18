#!/usr/bin/env bash
# SessionStart: копирует statusline.sh в постоянный каталог плагина ${CLAUDE_PLUGIN_DATA}.
# statusLine в настройках ссылается туда: каталог версии плагина меняется с каждым
# обновлением, а этот остаётся, и новая версия скрипта доходит без правки настроек.
set -euo pipefail

src="$CLAUDE_PLUGIN_ROOT/scripts/statusline.sh"
dst="$CLAUDE_PLUGIN_DATA/statusline.sh"

if ! cmp -s "$src" "$dst"; then
  mkdir -p "$CLAUDE_PLUGIN_DATA"
  # Через временный файл: строка состояния может запуститься посреди копирования.
  tmp="$dst.$$"
  cp "$src" "$tmp" && chmod 0755 "$tmp" && mv -f "$tmp" "$dst"
fi

# Вывод SessionStart без JSON ушёл бы модели в контекст; systemMessage видит только человек.
if ! command -v jq >/dev/null 2>&1; then
  printf '%s\n' '{"systemMessage":"statusline: строке состояния нужен jq — apt install jq или brew install jq"}'
fi
