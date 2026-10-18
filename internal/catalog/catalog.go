// Package catalog читает маркетплейс: .claude-plugin/marketplace.json и манифесты плагинов.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// Marketplace — каталог плагинов.
type Marketplace struct {
	Name    string
	Dir     string
	Plugins []Plugin
}

// Plugin — плагин каталога.
type Plugin struct {
	Name        string
	Description string
	Dir         string
	Options     map[string]Option
}

// Option — поле userConfig из plugin.json.
type Option struct {
	Type    string   `json:"type"`
	Title   string   `json:"title"`
	Default any      `json:"default"`
	Min     *float64 `json:"min"`
	Max     *float64 `json:"max"`
	Options []string `json:"options"`
}

// Load читает каталог из корня маркетплейса.
func Load(dir string) (Marketplace, error) {
	var raw struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name        string `json:"name"`
			Source      string `json:"source"`
			Description string `json:"description"`
		} `json:"plugins"`
	}
	if err := readJSON(filepath.Join(dir, ".claude-plugin", "marketplace.json"), &raw); err != nil {
		return Marketplace{}, err
	}
	m := Marketplace{Name: raw.Name, Dir: dir}
	for _, p := range raw.Plugins {
		pluginDir := filepath.Join(dir, filepath.FromSlash(p.Source))
		var manifest struct {
			UserConfig map[string]Option `json:"userConfig"`
		}
		if err := readJSON(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), &manifest); err != nil {
			return Marketplace{}, err
		}
		m.Plugins = append(m.Plugins, Plugin{
			Name:        p.Name,
			Description: p.Description,
			Dir:         pluginDir,
			Options:     manifest.UserConfig,
		})
	}
	return m, nil
}

// Plugin находит плагин по имени.
func (m Marketplace) Plugin(name string) (Plugin, error) {
	i := slices.IndexFunc(m.Plugins, func(p Plugin) bool { return p.Name == name })
	if i < 0 {
		return Plugin{}, fmt.Errorf("в маркетплейсе %s нет плагина %q", m.Name, name)
	}
	return m.Plugins[i], nil
}

// ID — имя плагина в enabledPlugins: <плагин>@<маркетплейс>.
func (m Marketplace) ID(plugin string) string {
	return plugin + "@" + m.Name
}

// Parse переводит строку из командной строки в значение поля userConfig.
func (o Option) Parse(s string) (any, error) {
	switch o.Type {
	case "number":
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("%q — не число", s)
		}
		if o.Min != nil && n < *o.Min || o.Max != nil && n > *o.Max {
			return nil, fmt.Errorf("%s вне границ %s", s, o.bounds())
		}
		return json.Number(strconv.FormatFloat(n, 'f', -1, 64)), nil
	case "boolean":
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("%q — не true или false", s)
		}
		return b, nil
	case "string":
		if len(o.Options) > 0 && !slices.Contains(o.Options, s) {
			return nil, fmt.Errorf("%q — не из %v", s, o.Options)
		}
		return s, nil
	default:
		return nil, fmt.Errorf("поле типа %s задаётся в /config Claude Code", o.Type)
	}
}

func (o Option) bounds() string {
	lo, hi := "-∞", "+∞"
	if o.Min != nil {
		lo = strconv.FormatFloat(*o.Min, 'f', -1, 64)
	}
	if o.Max != nil {
		hi = strconv.FormatFloat(*o.Max, 'f', -1, 64)
	}
	return "[" + lo + ", " + hi + "]"
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec // каталог маркетплейса задаёт пользователь
	if err != nil {
		return fmt.Errorf("чтение каталога: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("разбор %s: %w", path, err)
	}
	return nil
}
