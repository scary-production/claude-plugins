package install

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scary-production/claude-plugins/internal/catalog"
	"github.com/scary-production/claude-plugins/internal/settings"
)

// statuslinePlugin — плагин, которому нужен statusLine в настройках: плагин сам его
// задать не может (из настроек плагина Claude Code берёт только agent и subagentStatusLine).
const statuslinePlugin = "statusline"

// Styles — стили statusline.sh.
var Styles = []string{"full", "compact", "bars", "minimal"}

const defaultStyle = "full"

// installStatusLine ставит statusLine на скрипт в постоянном каталоге плагина и кладёт
// туда скрипт: дальше его обновляет хук SessionStart плагина.
func (in *Installer) installStatusLine(obj *settings.Object, p catalog.Plugin, style string) error {
	if style == "" {
		style = defaultStyle
		if current, ok := statusLineStyle(obj, in.market); ok {
			style = current
		}
	}
	if !slices.Contains(Styles, style) {
		return fmt.Errorf("неизвестный стиль %q: %s", style, strings.Join(Styles, ", "))
	}

	if prev, ok := obj.Lookup("statusLine"); ok {
		if _, ours := statusLineStyle(obj, in.market); !ours {
			cmd, _ := prev.Get("command")
			fmt.Fprintf(in.out, "прежний statusLine заменён: %v\n", cmd)
		}
	}
	obj.Set("statusLine", settings.NewObject().
		With("type", "command").
		With("command", statusLineCommand(in.market, style)).
		With("padding", 0).
		With("refreshInterval", 10))

	// Скрипт — сразу, не дожидаясь первого старта Claude Code с плагином.
	if err := mounted(in.claudeDir, ""); err != nil {
		fmt.Fprintln(in.out, "~/.claude не смонтирован: строка состояния появится после первого старта Claude Code с плагином")
		return nil
	}
	script, err := os.ReadFile(filepath.Join(p.Dir, "scripts", "statusline.sh"))
	if err != nil {
		return fmt.Errorf("скрипт строки состояния: %w", err)
	}
	dst := filepath.Join(in.claudeDir, "plugins", "data", dataID(in.market.ID(p.Name)), "statusline.sh")
	return settings.WriteFile(dst, script, 0o755)
}

// removeStatusLine убирает statusLine, если он наш.
func removeStatusLine(obj *settings.Object, market catalog.Marketplace) {
	if _, ours := statusLineStyle(obj, market); ours {
		obj.Delete("statusLine")
	}
}

// statusLineCommand — команда для настроек. Путь от ~: проектный файл уходит в git и
// читается на чужих машинах, а оболочка, которая запускает команду, раскроет тильду.
func statusLineCommand(market catalog.Marketplace, style string) string {
	return "~/.claude/plugins/data/" + dataID(market.ID(statuslinePlugin)) + "/statusline.sh " + style
}

// statusLineStyle возвращает стиль, если statusLine настроен на наш скрипт.
func statusLineStyle(obj *settings.Object, market catalog.Marketplace) (string, bool) {
	line, ok := obj.Lookup("statusLine")
	if !ok {
		return "", false
	}
	cmd, _ := line.Get("command")
	s, _ := cmd.(string)
	marker := "/" + dataID(market.ID(statuslinePlugin)) + "/statusline.sh"
	_, after, found := strings.Cut(s, marker)
	if !found {
		return "", false
	}
	return strings.TrimSpace(after), true
}

// dataID — имя постоянного каталога плагина (${CLAUDE_PLUGIN_DATA}): идентификатор
// плагина, в котором всё, кроме букв, цифр, _ и -, заменено на -.
func dataID(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, id)
}
