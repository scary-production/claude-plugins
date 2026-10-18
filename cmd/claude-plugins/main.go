// claude-plugins включает плагины маркетплейса scary-production в настройках Claude Code.
// Запускается в контейнере, каталоги хоста смонтированы: см. README.
package main

import (
	"os"

	"github.com/scary-production/claude-plugins/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
