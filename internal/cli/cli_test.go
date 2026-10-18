package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run3(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestFlagsAnywhere(t *testing.T) {
	claude := t.TempDir()
	dirs := []string{"-claude-dir", claude, "-project-dir", t.TempDir(), "-marketplace-dir", "../.."}

	args := append(append([]string{}, dirs...), "install", "statusline", "-style", "compact")
	if code, _, stderr := run3(t, args...); code != 0 {
		t.Fatalf("install: %d %s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(claude, "settings.json"))
	if err != nil || !strings.Contains(string(data), "statusline.sh compact") {
		t.Fatalf("настройки: %v\n%s", err, data)
	}

	code, stdout, _ := run3(t, append([]string{"list"}, dirs...)...)
	if code != 0 || !strings.Contains(stdout, "statusline       user ✓  project —  local —") {
		t.Fatalf("list: %d\n%s", code, stdout)
	}
}

// Каталог другого маркетплейса и его репозиторий: в настройки идёт -repo, а не scary-production.
func TestOwnMarketplace(t *testing.T) {
	market := t.TempDir()
	files := map[string]string{
		".claude-plugin/marketplace.json":          `{"name": "own", "plugins": [{"name": "hello", "source": "./plugins/hello"}]}`,
		"plugins/hello/.claude-plugin/plugin.json": `{"name": "hello"}`,
	}
	for name, data := range files {
		path := filepath.Join(market, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	claude := t.TempDir()
	dirs := []string{"-claude-dir", claude, "-project-dir", t.TempDir(), "-marketplace-dir", market, "-repo", "acme/own-plugins"}

	if code, _, stderr := run3(t, append([]string{"install", "hello"}, dirs...)...); code != 0 {
		t.Fatalf("install: %d %s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(claude, "settings.json"))
	for _, want := range []string{`"repo": "acme/own-plugins"`, `"hello@own": true`} {
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("настройки без %s: %v\n%s", want, err, data)
		}
	}
	code, stdout, _ := run3(t, append([]string{"list"}, dirs...)...)
	if code != 0 || !strings.Contains(stdout, "Маркетплейс own — github.com/acme/own-plugins") {
		t.Fatalf("list: %d\n%s", code, stdout)
	}
}

func TestUsageAndErrors(t *testing.T) {
	if code, stdout, _ := run3(t, "-h"); code != 0 || !strings.Contains(stdout, "install [-scope S]") {
		t.Errorf("-h: %d %q", code, stdout)
	}
	if code, stdout, _ := run3(t); code != 0 || stdout == "" {
		t.Errorf("без аргументов: %d", code)
	}
	if code, _, stderr := run3(t, "-marketplace-dir", "../..", "frobnicate"); code != 2 || !strings.Contains(stderr, "неизвестная команда") {
		t.Errorf("неизвестная команда: %d %q", code, stderr)
	}
	for _, repo := range []string{"acme", "acme/", "/own", "acme/own/x"} {
		if code, _, stderr := run3(t, "-marketplace-dir", "../..", "-repo", repo, "list"); code != 2 || !strings.Contains(stderr, "владелец/имя") {
			t.Errorf("-repo %q: %d %q", repo, code, stderr)
		}
	}
	if code, _, stderr := run3(t, "-marketplace-dir", "../..", "-claude-dir", "/nonexistent", "install", "statusline"); code != 1 || !strings.Contains(stderr, `-v "$HOME/.claude:/claude"`) {
		t.Errorf("без монтирования: %d %q", code, stderr)
	}
}
