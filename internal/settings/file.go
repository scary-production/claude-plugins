package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// BackupSuffix — окончание копии файла настроек до первой правки установщиком.
const BackupSuffix = ".claude-plugins.bak"

// Load читает файл настроек; файла нет — пустой объект.
func Load(path string) (*Object, error) {
	data, err := os.ReadFile(path) //nolint:gosec // файл настроек выбирает пользователь установщика
	if errors.Is(err, fs.ErrNotExist) {
		return NewObject(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("чтение %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return NewObject(), nil
	}
	obj, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("разбор %s: %w", path, err)
	}
	return obj, nil
}

// Save пишет объект в файл, если содержимое изменилось, и сообщает, писал ли.
// backup — перед записью сохранить прежнее содержимое рядом (BackupSuffix).
// Существующий файл пишется поверх: права и владелец остаются его.
func Save(path string, obj *Object, backup bool) (bool, error) {
	data, err := obj.Encode()
	if err != nil {
		return false, err
	}
	old, err := os.ReadFile(path) //nolint:gosec // файл настроек выбирает пользователь установщика
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("чтение %s: %w", path, err)
	}
	if bytes.Equal(old, data) {
		return false, nil
	}
	if backup && old != nil {
		if err := WriteFile(path+BackupSuffix, old, 0o600); err != nil {
			return false, err
		}
	}
	if err := WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// WriteFile пишет файл, создавая каталоги. Под root (контейнер без --user) новые
// каталоги и файлы отдаются владельцу ближайшего существующего каталога: иначе
// Claude Code на хосте не сможет их править.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	created, err := mkdirAll(dir)
	if err != nil {
		return err
	}
	_, statErr := os.Stat(path)
	isNew := errors.Is(statErr, fs.ErrNotExist)
	if err := os.WriteFile(path, data, perm); err != nil { //nolint:gosec // путь — в смонтированных каталогах пользователя
		return fmt.Errorf("запись %s: %w", path, err)
	}
	if isNew {
		created = append(created, path)
	}
	return adoptOwner(created)
}

// mkdirAll создаёт каталог с родителями и возвращает созданные, от внешнего к внутреннему.
func mkdirAll(dir string) ([]string, error) {
	var missing []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			break
		}
		missing = append([]string{d}, missing...)
		if filepath.Dir(d) == d {
			break
		}
	}
	// 0755, как у каталогов, которые создаёт сам Claude Code, и у .claude в git.
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // см. выше
		return nil, fmt.Errorf("каталог %s: %w", dir, err)
	}
	return missing, nil
}
