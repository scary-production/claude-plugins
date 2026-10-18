package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scary-production/claude-plugins/internal/catalog"
	"github.com/scary-production/claude-plugins/internal/settings"
)

type fixture struct {
	in      *Installer
	claude  string
	project string
	out     *bytes.Buffer
}

// newFixture — установщик над настоящим каталогом маркетплейса и пустыми каталогами хоста.
func newFixture(t *testing.T) fixture {
	t.Helper()
	market, err := catalog.Load(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	f := fixture{claude: filepath.Join(root, "claude"), project: filepath.Join(root, "project"), out: &bytes.Buffer{}}
	for _, dir := range []string{f.claude, f.project} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.in = New(f.claude, f.project, market, DefaultRepo, f.out)
	return f
}

func (f fixture) load(t *testing.T, path string) *settings.Object {
	t.Helper()
	obj, err := settings.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func value(t *testing.T, obj *settings.Object, path ...string) any {
	t.Helper()
	for _, key := range path[:len(path)-1] {
		child, ok := obj.Lookup(key)
		if !ok {
			return nil
		}
		obj = child
	}
	v, _ := obj.Get(path[len(path)-1])
	return v
}

func TestInstallUserReplacesStatusLineAndSetsConfig(t *testing.T) {
	f := newFixture(t)
	user := filepath.Join(f.claude, "settings.json")
	prev := `{"statusLine": {"type": "command", "command": "~/.claude/statusline.sh"}, "model": "opus"}`
	if err := os.WriteFile(user, []byte(prev), 0o644); err != nil {
		t.Fatal(err)
	}

	err := f.in.Install(ScopeUser, []string{"statusline", "context-guard"}, Options{Set: map[string]string{"warn_at": "40"}})
	if err != nil {
		t.Fatal(err)
	}

	obj := f.load(t, user)
	if got := value(t, obj, "statusLine", "command"); got != "~/.claude/plugins/data/statusline-scary-production/statusline.sh full" {
		t.Errorf("statusLine: %v", got)
	}
	if got := value(t, obj, "enabledPlugins", "context-guard@scary-production"); got != true {
		t.Errorf("context-guard не включён: %v", got)
	}
	if got := value(t, obj, "extraKnownMarketplaces", "scary-production", "source", "repo"); got != DefaultRepo {
		t.Errorf("маркетплейс: %v", got)
	}
	if got := value(t, obj, "pluginConfigs", "context-guard@scary-production", "options", "warn_at"); got == nil || got.(interface{ String() string }).String() != "40" {
		t.Errorf("warn_at: %v", got)
	}
	if !strings.Contains(f.out.String(), "прежний statusLine заменён: ~/.claude/statusline.sh") {
		t.Errorf("не сказано о прежнем statusLine:\n%s", f.out)
	}
	if backup, _ := os.ReadFile(user + settings.BackupSuffix); string(backup) != prev {
		t.Errorf("в копии не прежний файл: %s", backup)
	}
	script := filepath.Join(f.claude, "plugins", "data", "statusline-scary-production", "statusline.sh")
	if info, err := os.Stat(script); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("скрипт строки состояния: %v", err)
	}
}

func TestInstallKeepsStyleAndForeignMarketplace(t *testing.T) {
	f := newFixture(t)
	user := filepath.Join(f.claude, "settings.json")
	// Маркетплейс, добавленный из локального каталога, установщик не перетирает.
	local := `{"extraKnownMarketplaces": {"scary-production": {"source": {"source": "directory", "path": "/src"}}}}`
	if err := os.WriteFile(user, []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.in.Install(ScopeUser, []string{"statusline"}, Options{Style: "bars"}); err != nil {
		t.Fatal(err)
	}
	if err := f.in.Install(ScopeUser, []string{"statusline"}, Options{}); err != nil {
		t.Fatal(err)
	}

	obj := f.load(t, user)
	if got := value(t, obj, "statusLine", "command"); !strings.HasSuffix(got.(string), "statusline.sh bars") {
		t.Errorf("стиль не сохранился: %v", got)
	}
	if got := value(t, obj, "extraKnownMarketplaces", "scary-production", "source", "source"); got != "directory" {
		t.Errorf("локальный маркетплейс перетёрт: %v", got)
	}
}

func TestProjectInstallWritesUserConfigToUserSettings(t *testing.T) {
	f := newFixture(t)
	err := f.in.Install(ScopeProject, []string{"context-guard"}, Options{Set: map[string]string{"wrap_at": "80"}})
	if err != nil {
		t.Fatal(err)
	}

	project := f.load(t, filepath.Join(f.project, ".claude", "settings.json"))
	if got := value(t, project, "enabledPlugins", "context-guard@scary-production"); got != true {
		t.Errorf("плагин не включён в проекте: %v", got)
	}
	if _, ok := project.Get("pluginConfigs"); ok {
		t.Error("pluginConfigs в проекте: Claude Code читает их только у пользователя")
	}
	user := f.load(t, filepath.Join(f.claude, "settings.json"))
	if got := value(t, user, "pluginConfigs", "context-guard@scary-production", "options", "wrap_at"); got == nil {
		t.Error("wrap_at не записан пользователю")
	}
	if _, err := os.Stat(filepath.Join(f.project, ".claude", "settings.json"+settings.BackupSuffix)); !os.IsNotExist(err) {
		t.Error("копия проектного файла не нужна: он в git")
	}
}

func TestRemove(t *testing.T) {
	f := newFixture(t)
	for _, scope := range []Scope{ScopeUser, ScopeProject} {
		if err := f.in.Install(scope, []string{"statusline"}, Options{}); err != nil {
			t.Fatal(err)
		}
		if err := f.in.Remove(scope, []string{"statusline"}); err != nil {
			t.Fatal(err)
		}
	}

	user := f.load(t, filepath.Join(f.claude, "settings.json"))
	if got := value(t, user, "enabledPlugins", "statusline@scary-production"); got != false {
		t.Errorf("у пользователя плагин должен стать false: %v", got)
	}
	if _, ok := user.Get("statusLine"); ok {
		t.Error("наш statusLine остался у пользователя")
	}
	project := f.load(t, filepath.Join(f.project, ".claude", "settings.json"))
	if keys := project.Keys(); len(keys) != 0 {
		t.Errorf("в проекте остались ключи %v", keys)
	}
}

func TestErrors(t *testing.T) {
	f := newFixture(t)
	cases := map[string]error{
		"нет плагина":  f.in.Install(ScopeUser, []string{"nope"}, Options{}),
		"нет имён":     f.in.Install(ScopeUser, nil, Options{}),
		"чужое поле":   f.in.Install(ScopeUser, []string{"statusline"}, Options{Set: map[string]string{"warn_at": "40"}}),
		"вне границ":   f.in.Install(ScopeUser, []string{"context-guard"}, Options{Set: map[string]string{"warn_at": "500"}}),
		"не тот стиль": f.in.Install(ScopeUser, []string{"statusline"}, Options{Style: "neon"}),
		"нет каталога": New(filepath.Join(f.claude, "missing"), f.project, f.in.market, DefaultRepo, f.out).Install(ScopeUser, []string{"statusline"}, Options{}),
		"нет знака =":  func() error { _, err := ParseSet([]string{"warn_at"}); return err }(),
	}
	for name, err := range cases {
		if err == nil {
			t.Errorf("%s: нет ошибки", name)
		}
	}
}

func TestStatus(t *testing.T) {
	f := newFixture(t)
	if err := f.in.Install(ScopeLocal, []string{"context-guard"}, Options{}); err != nil {
		t.Fatal(err)
	}
	rows, err := f.in.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		want := []State{StateAbsent, StateAbsent, StateAbsent}
		if row.Name == "context-guard" {
			want[2] = StateEnabled
		}
		for i := range want {
			if row.States[i] != want[i] {
				t.Errorf("%s: %v, ждали %v", row.Name, row.States, want)
				break
			}
		}
	}
}

func TestDataID(t *testing.T) {
	if got := dataID("statusline@scary-production"); got != "statusline-scary-production" {
		t.Fatal(got)
	}
}
