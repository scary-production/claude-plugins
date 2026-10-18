//go:build unix

package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// adoptOwner отдаёт пути владельцу каталога над первым из них, если процесс — root.
func adoptOwner(paths []string) error {
	if len(paths) == 0 || os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Stat(filepath.Dir(paths[0]))
	if err != nil {
		return fmt.Errorf("владелец %s: %w", filepath.Dir(paths[0]), err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 {
		return nil
	}
	for _, p := range paths {
		if err := os.Lchown(p, int(st.Uid), int(st.Gid)); err != nil {
			return fmt.Errorf("владелец %s: %w", p, err)
		}
	}
	return nil
}
