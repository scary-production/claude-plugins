#!/usr/bin/env bash
# /statusline:style — показывает стили на примере или включает один из них.
#
#   style.sh --data DIR [--project DIR]                 все стили на примере
#   style.sh --data DIR [--project DIR] СТИЛЬ [ОБЛАСТЬ]  включить стиль
#
# --data — постоянный каталог плагина (${CLAUDE_PLUGIN_DATA}), --project — корень проекта.
# ОБЛАСТЬ: user — ~/.claude/settings.json (по умолчанию); project — .claude/settings.json
# проекта, уходит в git; local — .claude/settings.local.json проекта, только у себя.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
styles="full compact bars minimal"
data="" project=$PWD
while [ $# -gt 0 ]; do
  case "$1" in
    --data) data=$2; shift 2 ;;
    --project) project=$2; shift 2 ;;
    *) break ;;
  esac
done

command -v jq >/dev/null 2>&1 || { echo "строке состояния нужен jq: apt install jq или brew install jq" >&2; exit 1; }

if [ $# -eq 0 ]; then
  for s in $styles; do
    printf '%s:\n' "$s"
    STATUSLINE_NOW=1800000000 bash "$here/statusline.sh" "$s" <"$here/sample.json" | sed 's/^/  /'
  done
  exit 0
fi

style=$1 scope=${2:-user}
case " $styles " in
  *" $style "*) ;;
  *) echo "неизвестный стиль «$style»: $styles" >&2; exit 2 ;;
esac
case "$scope" in
  user) settings="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/settings.json" ;;
  project) settings="$project/.claude/settings.json" ;;
  local) settings="$project/.claude/settings.local.json" ;;
  *) echo "неизвестная область «$scope»: user, project, local" >&2; exit 2 ;;
esac
# Пустые значения — если Claude Code не подставил переменные плагина в текст скилла.
[ -n "$project" ] || project=$PWD
[ -n "$data" ] || data="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/plugins/data/statusline-scary-production"

# Скрипт — в постоянный каталог сразу, не дожидаясь следующего старта сессии.
CLAUDE_PLUGIN_ROOT="${here%/scripts}" CLAUDE_PLUGIN_DATA="$data" bash "$here/sync.sh" >/dev/null

# В настройки — путь от ~: файл проекта уходит в git и читается на чужих машинах.
script="$data/statusline.sh"
# shellcheck disable=SC2088 # тильда раскроется в оболочке, которая запустит команду
case "$script" in "$HOME"/*) script="~${script#"$HOME"}" ;; esac

mkdir -p "$(dirname "$settings")"
[ -s "$settings" ] || echo '{}' >"$settings"
next=$(jq --arg cmd "$script $style" \
  '.statusLine = {type: "command", command: $cmd, padding: 0, refreshInterval: 10}' "$settings")
# Запись поверх, а не mv: у файла остаются права и владелец.
printf '%s\n' "$next" >"$settings"
echo "строка состояния: стиль $style, $settings"
