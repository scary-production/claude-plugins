package settings

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `{
  "env": {
    "CMD": "a && b <c>"
  },
  "model": "opus[1m]",
  "n": 1.50,
  "list": [
    1,
    {
      "x": null
    }
  ],
  "empty": {},
  "none": [],
  "ok": true
}
`

func TestRoundTripKeepsOrderAndText(t *testing.T) {
	obj, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	got, err := obj.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sample {
		t.Fatalf("после разбора и записи файл изменился:\n%s", got)
	}
}

func TestSetKeepsPlaceDeleteRemoves(t *testing.T) {
	obj, err := Parse([]byte(`{"a": 1, "b": 2, "c": 3}`))
	if err != nil {
		t.Fatal(err)
	}
	obj.Set("b", "два")
	obj.Set("d", true)
	if !obj.Delete("a") || obj.Delete("missing") {
		t.Fatal("Delete сообщил не то")
	}
	got, err := obj.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"b\": \"два\",\n  \"c\": 3,\n  \"d\": true\n}\n"
	if string(got) != want {
		t.Fatalf("получилось:\n%s\nждали:\n%s", got, want)
	}
}

func TestChildRefusesNonObject(t *testing.T) {
	obj, err := Parse([]byte(`{"enabledPlugins": []}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.Child("enabledPlugins"); err == nil {
		t.Fatal("массив принят за объект")
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{`[]`, `{"a":1} {}`, `{"a":`, `"x"`} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%q разобран без ошибки", in)
		}
	}
}

func TestSaveWritesOnlyChangesAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")

	obj := NewObject().With("a", 1)
	if changed, err := Save(path, obj, true); err != nil || !changed {
		t.Fatalf("первая запись: changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(path + BackupSuffix); !os.IsNotExist(err) {
		t.Fatal("копия нового файла не нужна")
	}
	if changed, err := Save(path, obj, true); err != nil || changed {
		t.Fatalf("без изменений: changed=%v err=%v", changed, err)
	}

	obj.Set("b", 2)
	if _, err := Save(path, obj, true); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + BackupSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != "{\n  \"a\": 1\n}\n" {
		t.Fatalf("в копии не прежний файл:\n%s", backup)
	}
}

func TestLoadMissingOrEmpty(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "missing.json"), empty} {
		obj, err := Load(path)
		if err != nil || len(obj.Keys()) != 0 {
			t.Fatalf("%s: %v, ключи %v", path, err, obj.Keys())
		}
	}
}
