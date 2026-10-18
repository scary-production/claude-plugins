package install

import (
	"github.com/scary-production/claude-plugins/internal/settings"
)

// State — включён ли плагин в области.
type State int

const (
	StateUnknown  State = iota // каталог области не смонтирован
	StateAbsent                // в настройках области плагина нет
	StateEnabled               // true
	StateDisabled              // false
)

// Row — плагин и его состояние по областям, в порядке Scopes.
type Row struct {
	Name        string
	Description string
	States      []State
}

// Status читает настройки всех смонтированных областей.
func (in *Installer) Status() ([]Row, error) {
	enabled := make([]*settings.Object, len(Scopes))
	for i, scope := range Scopes {
		path, err := in.settingsPath(scope)
		if err != nil {
			continue
		}
		obj, err := settings.Load(path)
		if err != nil {
			return nil, err
		}
		enabled[i], _ = obj.Lookup("enabledPlugins")
		if enabled[i] == nil {
			enabled[i] = settings.NewObject()
		}
	}

	rows := make([]Row, 0, len(in.market.Plugins))
	for _, p := range in.market.Plugins {
		row := Row{Name: p.Name, Description: p.Description, States: make([]State, len(Scopes))}
		for i := range Scopes {
			if enabled[i] == nil {
				continue
			}
			switch v, ok := enabled[i].Get(in.market.ID(p.Name)); {
			case !ok:
				row.States[i] = StateAbsent
			case v == true:
				row.States[i] = StateEnabled
			default:
				row.States[i] = StateDisabled
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
