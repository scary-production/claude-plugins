package install

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/scary-production/claude-plugins/internal/catalog"
	"github.com/scary-production/claude-plugins/internal/settings"
)

// configValue — разобранное значение поля userConfig.
type configValue struct {
	id    string // плагин@маркетплейс
	key   string
	value any
}

// configValues находит для каждого «поле=значение» плагин среди устанавливаемых
// и проверяет значение по его userConfig.
func (in *Installer) configValues(plugins []catalog.Plugin, set map[string]string) ([]configValue, error) {
	var values []configValue
	for _, key := range slices.Sorted(maps.Keys(set)) {
		var owners []catalog.Plugin
		for _, p := range plugins {
			if _, ok := p.Options[key]; ok {
				owners = append(owners, p)
			}
		}
		switch len(owners) {
		case 0:
			return nil, fmt.Errorf("поле %q не из userConfig устанавливаемых плагинов", key)
		case 1:
		default:
			return nil, fmt.Errorf("поле %q есть у нескольких плагинов: ставьте их по одному", key)
		}
		p := owners[0]
		v, err := p.Options[key].Parse(set[key])
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", p.Name, key, err)
		}
		values = append(values, configValue{id: in.market.ID(p.Name), key: key, value: v})
	}
	return values, nil
}

// setConfig пишет значения в pluginConfigs.<плагин>.options — туда же, куда их
// пишут /config и claude plugin configure.
func (in *Installer) setConfig(user *settings.Object, values []configValue) error {
	if len(values) == 0 {
		return nil
	}
	configs, err := user.Child("pluginConfigs")
	if err != nil {
		return err
	}
	for _, v := range values {
		plugin, err := configs.Child(v.id)
		if err != nil {
			return err
		}
		options, err := plugin.Child("options")
		if err != nil {
			return err
		}
		options.Set(v.key, v.value)
		fmt.Fprintf(in.out, "%s: %s = %v\n", v.id, v.key, v.value)
	}
	return nil
}

// ParseSet разбирает «поле=значение» из командной строки.
func ParseSet(pairs []string) (map[string]string, error) {
	set := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("%q: нужно поле=значение", pair)
		}
		set[key] = value
	}
	return set, nil
}
