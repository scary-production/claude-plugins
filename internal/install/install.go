// Package install включает и выключает плагины маркетплейса в настройках Claude Code.
//
// Установка декларативная: в settings.json нужной области пишутся маркетплейс
// (extraKnownMarketplaces) и плагин (enabledPlugins), а скачивает их сам Claude Code
// при следующем старте. Чего плагин сам задать не может (statusLine, значения
// userConfig), установщик тоже пишет в настройки.
package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scary-production/claude-plugins/internal/catalog"
	"github.com/scary-production/claude-plugins/internal/settings"
)

// DefaultRepo — репозиторий GitHub маркетплейса scary-production, из которого Claude Code
// берёт его плагины. Каталог другого маркетплейса приходит со своим репозиторием.
const DefaultRepo = "scary-production/claude-plugins"

// Scope — чьи настройки правятся.
type Scope string

const (
	ScopeUser    Scope = "user"    // ~/.claude/settings.json: все проекты пользователя
	ScopeProject Scope = "project" // .claude/settings.json: все, кто клонирует проект
	ScopeLocal   Scope = "local"   // .claude/settings.local.json: пользователь в этом проекте
)

// Scopes — все области по порядку.
var Scopes = []Scope{ScopeUser, ScopeProject, ScopeLocal}

// ParseScope разбирает область из командной строки.
func ParseScope(s string) (Scope, error) {
	if slices.Contains(Scopes, Scope(s)) {
		return Scope(s), nil
	}
	return "", fmt.Errorf("неизвестная область %q: user, project или local", s)
}

// Options — что задать вместе с плагинами.
type Options struct {
	// Style — стиль statusline; пусто — прежний стиль или full.
	Style string
	// Set — значения полей userConfig: имя поля → значение.
	Set map[string]string
}

// Installer правит настройки в смонтированных каталогах хоста.
type Installer struct {
	claudeDir  string
	projectDir string
	market     catalog.Marketplace
	repo       string
	out        io.Writer
}

// New возвращает установщик: claudeDir — ~/.claude хоста, projectDir — корень проекта,
// repo — репозиторий GitHub маркетплейса (владелец/имя) для extraKnownMarketplaces.
func New(claudeDir, projectDir string, market catalog.Marketplace, repo string, out io.Writer) *Installer {
	return &Installer{claudeDir: claudeDir, projectDir: projectDir, market: market, repo: repo, out: out}
}

// Install включает плагины в области scope.
func (in *Installer) Install(scope Scope, names []string, opts Options) error {
	plugins, err := in.plugins(names)
	if err != nil {
		return err
	}
	values, err := in.configValues(plugins, opts.Set)
	if err != nil {
		return err
	}
	path, err := in.settingsPath(scope)
	if err != nil {
		return err
	}
	obj, err := settings.Load(path)
	if err != nil {
		return err
	}

	markets, err := obj.Child("extraKnownMarketplaces")
	if err != nil {
		return err
	}
	if _, ok := markets.Get(in.market.Name); !ok {
		source := settings.NewObject().With("source", "github").With("repo", in.repo)
		markets.Set(in.market.Name, settings.NewObject().With("source", source).With("autoUpdate", true))
	}
	enabled, err := obj.Child("enabledPlugins")
	if err != nil {
		return err
	}
	for _, p := range plugins {
		enabled.Set(in.market.ID(p.Name), true)
		if p.Name == statuslinePlugin {
			if err := in.installStatusLine(obj, p, opts.Style); err != nil {
				return err
			}
		}
	}

	if scope == ScopeUser {
		if err := in.setConfig(obj, values); err != nil {
			return err
		}
	}
	if err := in.save(path, obj, scope); err != nil {
		return err
	}
	if scope != ScopeUser && len(values) > 0 {
		// Значения userConfig Claude Code читает только из настроек пользователя.
		if err := in.updateUser(func(user *settings.Object) error { return in.setConfig(user, values) }); err != nil {
			return err
		}
	}

	for _, p := range plugins {
		fmt.Fprintf(in.out, "включён %s (%s)\n", in.market.ID(p.Name), scope)
	}
	fmt.Fprintln(in.out, "Claude Code скачает плагины при следующем запуске; в открытой сессии — /reload-plugins.")
	if scope == ScopeProject {
		fmt.Fprintln(in.out, "Коллеги получат их, когда доверят папку проекта; закоммитьте .claude/settings.json.")
	}
	return nil
}

// Remove выключает плагины в области scope. В user и local пишется false: иначе
// уже скачанный плагин остался бы включённым по умолчанию. Из project ключ
// удаляется: false там выключил бы плагин и тем, кто включил его у себя.
func (in *Installer) Remove(scope Scope, names []string) error {
	plugins, err := in.plugins(names)
	if err != nil {
		return err
	}
	path, err := in.settingsPath(scope)
	if err != nil {
		return err
	}
	obj, err := settings.Load(path)
	if err != nil {
		return err
	}
	enabled, err := obj.Child("enabledPlugins")
	if err != nil {
		return err
	}
	for _, p := range plugins {
		if scope == ScopeProject {
			enabled.Delete(in.market.ID(p.Name))
		} else {
			enabled.Set(in.market.ID(p.Name), false)
		}
		if p.Name == statuslinePlugin {
			removeStatusLine(obj, in.market)
		}
	}
	if len(enabled.Keys()) == 0 {
		obj.Delete("enabledPlugins")
	}
	if scope == ScopeProject && !in.anyEnabled(enabled) {
		if markets, ok := obj.Lookup("extraKnownMarketplaces"); ok {
			markets.Delete(in.market.Name)
			if len(markets.Keys()) == 0 {
				obj.Delete("extraKnownMarketplaces")
			}
		}
	}
	if err := in.save(path, obj, scope); err != nil {
		return err
	}
	for _, p := range plugins {
		fmt.Fprintf(in.out, "выключен %s (%s)\n", in.market.ID(p.Name), scope)
	}
	return nil
}

func (in *Installer) plugins(names []string) ([]catalog.Plugin, error) {
	if len(names) == 0 {
		return nil, errors.New("не назван ни один плагин: claude-plugins list покажет их")
	}
	plugins := make([]catalog.Plugin, 0, len(names))
	for _, name := range names {
		p, err := in.market.Plugin(name)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

// settingsPath — файл настроек области; каталог должен быть смонтирован.
func (in *Installer) settingsPath(scope Scope) (string, error) {
	switch scope {
	case ScopeUser:
		if err := mounted(in.claudeDir, `-v "$HOME/.claude:/claude"`); err != nil {
			return "", err
		}
		return filepath.Join(in.claudeDir, "settings.json"), nil
	case ScopeProject, ScopeLocal:
		if err := mounted(in.projectDir, `-v "$PWD:/project"`); err != nil {
			return "", err
		}
		name := "settings.json"
		if scope == ScopeLocal {
			name = "settings.local.json"
		}
		return filepath.Join(in.projectDir, ".claude", name), nil
	default:
		return "", fmt.Errorf("неизвестная область %q", scope)
	}
}

func (in *Installer) save(path string, obj *settings.Object, scope Scope) error {
	// Проектный файл в git — копия не нужна и мешала бы в рабочем дереве.
	changed, err := settings.Save(path, obj, scope != ScopeProject)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(in.out, "записан %s\n", in.hostPath(path))
	}
	return nil
}

func (in *Installer) updateUser(edit func(*settings.Object) error) error {
	path, err := in.settingsPath(ScopeUser)
	if err != nil {
		return err
	}
	obj, err := settings.Load(path)
	if err != nil {
		return err
	}
	if err := edit(obj); err != nil {
		return err
	}
	return in.save(path, obj, ScopeUser)
}

func (in *Installer) anyEnabled(enabled *settings.Object) bool {
	suffix := "@" + in.market.Name
	for _, k := range enabled.Keys() {
		if v, _ := enabled.Get(k); v == true && strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// hostPath показывает путь так, как его видит хост, а не контейнер.
func (in *Installer) hostPath(path string) string {
	if rel, ok := strings.CutPrefix(path, in.claudeDir+string(filepath.Separator)); ok {
		return "~/.claude/" + filepath.ToSlash(rel)
	}
	if rel, ok := strings.CutPrefix(path, in.projectDir+string(filepath.Separator)); ok {
		return filepath.ToSlash(rel)
	}
	return path
}

func mounted(dir, hint string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("нет каталога %s: смонтируйте его в docker run: %s", dir, hint)
	}
	if err != nil {
		return fmt.Errorf("каталог %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s — не каталог", dir)
	}
	return nil
}
