// Package test проверяет скрипты плагинов: statusline.sh рисует то, что видит человек.
package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

const scripts = "../plugins/statusline/scripts"

// render запускает statusline.sh со стилем на входе input и возвращает вывод с цветами.
func render(t *testing.T, style, input string) string {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(scripts, "statusline.sh"), style)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "STATUSLINE_NOW=1800000000", "CLAUDE_LAUNCHER=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v", style, err)
	}
	return string(out)
}

func TestStylesOnSample(t *testing.T) {
	sample, err := os.ReadFile(filepath.Join(scripts, "sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"full":    "Opus 5.5 │ shop │ ctx 42% │ 5ч ▰▰▰▰▰▰▰▰▱▱ 73% ⟳ 2ч 13м │ 7д 12% ⟳ 3д 4ч\n",
		"compact": "Opus 5.5 · ctx 42% · 5ч 73% · 7д 12%\n",
		"bars":    "Opus 5.5 │ shop\nctx ▰▰▰▰▰▱▱▱▱▱ 42% 420k/1M │ 5ч ▰▰▰▰▰▰▰▰▱▱ 73% ⟳ 2ч 13м │ 7д ▰▰▱▱▱▱▱▱▱▱ 12% ⟳ 3д 4ч\n",
		"minimal": "ctx 42%\n",
	}
	for style, text := range want {
		out := render(t, style, string(sample))
		if got := ansi.ReplaceAllString(out, ""); got != text {
			t.Errorf("%s:\n got %q\nwant %q", style, got, text)
		}
	}
	// 73% — жёлтый, 42% — зелёный.
	if out := render(t, "compact", string(sample)); !strings.Contains(out, "\x1b[33m5ч 73%") || !strings.Contains(out, "\x1b[32mctx 42%") {
		t.Errorf("цвета не те: %q", out)
	}
}

func TestMissingFields(t *testing.T) {
	cases := map[string]string{
		// До первого ответа модели нет ни контекста, ни лимитов; у ключа API лимитов нет вовсе.
		`{"model": {"display_name": "Sonnet"}, "context_window": {"used_percentage": null}}`: "Sonnet\n",
		`{}`:       "-\n",
		`not json`: "-\n",
	}
	for input, want := range cases {
		if got := ansi.ReplaceAllString(render(t, "compact", input), ""); got != want {
			t.Errorf("%s: got %q, want %q", input, got, want)
		}
	}
	if got := ansi.ReplaceAllString(render(t, "minimal", `{}`), ""); got != "ctx —\n" {
		t.Errorf("minimal без контекста: %q", got)
	}
	if got := ansi.ReplaceAllString(render(t, "bars", `{}`), ""); got != "- │ -\n" {
		t.Errorf("bars без данных — одна строка: %q", got)
	}
}

func TestBranchFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("нет git")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "feature-x").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	got := ansi.ReplaceAllString(render(t, "full", `{"workspace": {"current_dir": "`+dir+`"}}`), "")
	if !strings.Contains(got, "⎇ feature-x") {
		t.Fatalf("нет ветки: %q", got)
	}
}

func TestUnknownStyle(t *testing.T) {
	if got := render(t, "neon", `{}`); !strings.Contains(got, "неизвестный стиль «neon»") {
		t.Fatalf("%q", got)
	}
}
