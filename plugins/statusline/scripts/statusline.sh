#!/usr/bin/env bash
# Строка состояния Claude Code. Claude Code передаёт на stdin JSON сессии
# (https://code.claude.com/docs/en/statusline), скрипт печатает одну-две строки с цветами ANSI.
# Нужны bash и jq; ветка — из git, если он есть.
#
#   statusline.sh [full|compact|bars|minimal]   стиль, по умолчанию full
#
# CLAUDE_LAUNCHER — метка обёртки, которая запустила claude (стили full и bars);
# STATUSLINE_NOW — «сейчас» в unix-секундах, для примеров и тестов.
set -uo pipefail

style=${1:-full}
input=$(cat)

# Пустое поле @tsv склеило бы соседние табы в read, поэтому отсутствующее — «-» или -1.
IFS=$'\t' read -r MODEL DIR CTX CTX_TOK CTX_SIZE H5 H5_RESET W7 W7_RESET <<<"$(
  printf '%s' "$input" | jq -r '
    def num: if type == "number" then floor else -1 end;
    def str: if . == null or . == "" then "-" else tostring end;
    [ (.model.display_name | str),
      (.workspace.current_dir // .cwd | str),
      (.context_window.used_percentage | num),
      (.context_window.total_input_tokens | num),
      (.context_window.context_window_size | num),
      (.rate_limits.five_hour.used_percentage | num),
      (.rate_limits.five_hour.resets_at | num),
      (.rate_limits.seven_day.used_percentage | num),
      (.rate_limits.seven_day.resets_at | num)
    ] | @tsv' 2>/dev/null
)"
MODEL=${MODEL:--}; DIR=${DIR:--}; CTX=${CTX:--1}; CTX_TOK=${CTX_TOK:--1}; CTX_SIZE=${CTX_SIZE:--1}
H5=${H5:--1}; H5_RESET=${H5_RESET:--1}; W7=${W7:--1}; W7_RESET=${W7_RESET:--1}

R=$'\033[0m'; DIM=$'\033[2m'; CYAN=$'\033[36m'; MAGENTA=$'\033[1;35m'
NOW=${STATUSLINE_NOW:-$(date +%s)}

# Цвет по заполнению: до 70% — зелёный, до 90% — жёлтый, дальше — красный.
hue() {
  if [ "$1" -ge 90 ]; then printf '\033[31m'
  elif [ "$1" -ge 70 ]; then printf '\033[33m'
  else printf '\033[32m'; fi
}

# Полоска из 10 сегментов.
bar() {
  local filled=$(( ($1 + 9) / 10 )) i out=""
  [ "$filled" -gt 10 ] && filled=10
  for ((i = 0; i < 10; i++)); do
    if [ "$i" -lt "$filled" ]; then out+="▰"; else out+="▱"; fi
  done
  printf '%s' "$out"
}

# Время до сброса окна: «3д 4ч», «2ч 05м», «7м».
left() {
  local diff=$(( $1 - NOW )) d h m
  [ "$diff" -lt 0 ] && diff=0
  d=$(( diff / 86400 )); h=$(( diff % 86400 / 3600 )); m=$(( diff % 3600 / 60 ))
  if [ "$d" -gt 0 ]; then printf '%dд %dч' "$d" "$h"
  elif [ "$h" -gt 0 ]; then printf '%dч %02dм' "$h" "$m"
  else printf '%dм' "$m"; fi
}

# Токены коротко: 420k, 1M, 1.5M.
tokens() {
  if [ "$1" -ge 1000000 ]; then
    if [ $(( $1 % 1000000 )) -eq 0 ]; then printf '%dM' $(( $1 / 1000000 ))
    else printf '%d.%dM' $(( $1 / 1000000 )) $(( $1 % 1000000 / 100000 )); fi
  else printf '%dk' $(( $1 / 1000 )); fi
}

# Окно лимита: метка и процент, по флагам — полоска и время до сброса.
window() { # $1 метка, $2 процент, $3 сброс, $4 полоска 0|1, $5 сброс 0|1
  local out
  out="$(hue "$2")$1"
  [ "$4" = 1 ] && out+=" $(bar "$2")"
  out+=" $2%${R}"
  [ "$5" = 1 ] && [ "$3" -gt 0 ] && out+="${DIM} ⟳ $(left "$3")${R}"
  printf '%s' "$out"
}

# Модель, каталог и ветка: начало строк full и bars.
place() {
  local out="" branch=""
  [ -n "${CLAUDE_LAUNCHER:-}" ] && out+="${MAGENTA}${CLAUDE_LAUNCHER}${R}$1"
  out+="${DIM}${MODEL}${R}$1${CYAN}${DIR##*/}${R}"
  if [ "$DIR" != "-" ] && command -v git >/dev/null; then
    branch=$(GIT_OPTIONAL_LOCKS=0 git -C "$DIR" branch --show-current 2>/dev/null)
  fi
  [ -n "$branch" ] && out+="${DIM} ⎇ ${branch}${R}"
  printf '%s' "$out"
}

# Сегменты через разделитель; пустые пропускаются.
join() { # $1 разделитель, дальше сегменты
  local sep=$1 out="" s
  shift
  for s in "$@"; do
    [ -z "$s" ] && continue
    if [ -z "$out" ]; then out=$s; else out+="${sep}${s}"; fi
  done
  printf '%s' "$out"
}

SEP="${DIM} │ ${R}"
DOT="${DIM} · ${R}"
ctx="" ctx_bar=""
if [ "$CTX" -ge 0 ]; then
  ctx="$(hue "$CTX")ctx ${CTX}%${R}"
  ctx_bar="$(hue "$CTX")ctx $(bar "$CTX") ${CTX}%${R}"
  [ "$CTX_TOK" -ge 0 ] && [ "$CTX_SIZE" -gt 0 ] &&
    ctx_bar+="${DIM} $(tokens "$CTX_TOK")/$(tokens "$CTX_SIZE")${R}"
fi
# Окна лимитов есть только у подписки Pro и Max и только после первого ответа модели.
h5() { [ "$H5" -ge 0 ] && window 5ч "$H5" "$H5_RESET" "$1" "$2"; }
w7() { [ "$W7" -ge 0 ] && window 7д "$W7" "$W7_RESET" "$1" "$2"; }

case "$style" in
  full)
    printf '%s\n' "$(join "$SEP" "$(place "$SEP")" "$ctx" "$(h5 1 1)" "$(w7 0 1)")"
    ;;
  compact)
    printf '%s\n' "$(join "$DOT" "${DIM}${MODEL}${R}" "$ctx" "$(h5 0 0)" "$(w7 0 0)")"
    ;;
  bars)
    printf '%s\n' "$(place "$SEP")"
    meters=$(join "$SEP" "$ctx_bar" "$(h5 1 1)" "$(w7 1 1)")
    if [ -n "$meters" ]; then printf '%s\n' "$meters"; fi
    ;;
  minimal)
    printf '%s\n' "${ctx:-${DIM}ctx —${R}}"
    ;;
  *)
    printf 'statusline: неизвестный стиль «%s» — full, compact, bars, minimal\n' "$style"
    ;;
esac
