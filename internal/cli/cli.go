// Package cli — команды claude-plugins: list, install, remove, styles.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scary-production/claude-plugins/internal/catalog"
	"github.com/scary-production/claude-plugins/internal/install"
)

const usage = `claude-plugins — плагины маркетплейса scary-production в настройках Claude Code.

  list                                       плагины и где они включены
  install [-scope S] [-style X] [-set поле=значение]... ПЛАГИН...
  remove  [-scope S] ПЛАГИН...
  styles                                     стили строки состояния на примере

  -scope    user (по умолчанию) — ~/.claude/settings.json, все проекты;
            project — .claude/settings.json проекта, для всех, кто его клонирует;
            local — .claude/settings.local.json проекта, только у себя
  -style    стиль statusline: full, compact, bars, minimal
  -set      поле userConfig плагина, например -set warn_at=40 для context-guard

Каталоги хоста монтируются в контейнер: ~/.claude — в /claude, проект — в /project.

Другой маркетплейс: его рабочая копия монтируется в /opt/marketplace, а -repo
владелец/имя называет его репозиторий GitHub — оттуда плагины скачает Claude Code.
`

// Run выполняет команду и возвращает код выхода.
func Run(args []string, stdout, stderr io.Writer) int {
	err := run(args, stdout, stderr)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	var usageErr usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &usageErr):
		fmt.Fprintf(stderr, "%v\n\n%s", err, usage)
		return 2
	default:
		fmt.Fprintf(stderr, "claude-plugins: %v\n", err)
		return 1
	}
}

type usageError struct{ error }

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("claude-plugins", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	claudeDir := fs.String("claude-dir", "/claude", "~/.claude хоста")
	projectDir := fs.String("project-dir", "/project", "корень проекта на хосте")
	marketDir := fs.String("marketplace-dir", "/opt/marketplace", "корень маркетплейса")
	repo := fs.String("repo", install.DefaultRepo, "репозиторий GitHub маркетплейса")
	scope := fs.String("scope", string(install.ScopeUser), "user, project или local")
	style := fs.String("style", "", "стиль statusline")
	var set stringList
	fs.Var(&set, "set", "поле=значение userConfig")

	positional, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	if err != nil {
		return usageError{err}
	}
	if len(positional) == 0 || positional[0] == "help" {
		return flag.ErrHelp
	}
	command, names := positional[0], positional[1:]
	if owner, name, ok := strings.Cut(*repo, "/"); !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return usageError{fmt.Errorf("-repo %q: нужно владелец/имя репозитория GitHub", *repo)}
	}
	market, err := catalog.Load(*marketDir)
	if err != nil {
		return err
	}
	in := install.New(*claudeDir, *projectDir, market, *repo, stdout)

	switch command {
	case "list":
		return list(in, market, *repo, stdout)
	case "styles":
		return styles(market, stdout, stderr)
	case "install", "remove":
		sc, err := install.ParseScope(*scope)
		if err != nil {
			return usageError{err}
		}
		if command == "remove" {
			return in.Remove(sc, names)
		}
		values, err := install.ParseSet(set)
		if err != nil {
			return usageError{err}
		}
		return in.Install(sc, names, install.Options{Style: *style, Set: values})
	default:
		return usageError{fmt.Errorf("неизвестная команда %q", command)}
	}
}

func list(in *install.Installer, market catalog.Marketplace, repo string, out io.Writer) error {
	rows, err := in.Status()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Маркетплейс %s — github.com/%s\n\n", market.Name, repo)
	for _, row := range rows {
		marks := make([]string, len(install.Scopes))
		for i, scope := range install.Scopes {
			marks[i] = string(scope) + " " + stateMark(row.States[i])
		}
		fmt.Fprintf(out, "%-16s %s\n  %s\n\n", row.Name, strings.Join(marks, "  "), row.Description)
	}
	fmt.Fprintln(out, "✓ включён, ✗ выключен, — нет в настройках, ? каталог не смонтирован")
	return nil
}

func stateMark(s install.State) string {
	switch s {
	case install.StateEnabled:
		return "✓"
	case install.StateDisabled:
		return "✗"
	case install.StateAbsent:
		return "—"
	default:
		return "?"
	}
}

// styles показывает стили тем же скриптом, что рисует строку состояния.
func styles(market catalog.Marketplace, stdout, stderr io.Writer) error {
	p, err := market.Plugin("statusline")
	if err != nil {
		return err
	}
	cmd := exec.Command("bash", filepath.Join(p.Dir, "scripts", "style.sh")) //nolint:gosec // скрипт из каталога маркетплейса
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.Env = append(os.Environ(), "CLAUDE_LAUNCHER=")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("стили: %w", err)
	}
	return nil
}

// parseInterspersed разбирает флаги и позиционные аргументы вперемешку:
// install statusline -scope project.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }
