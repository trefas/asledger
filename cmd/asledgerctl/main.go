// Команда asledgerctl — утилита администратора asledger.
//
// Предназначена для миграций, резервного копирования, импорта и экспорта
// из командной строки. На этапе каркаса доступны подкоманды version и help;
// остальные появятся в вехах S1 и S5.
package main

import (
	"fmt"
	"os"

	"github.com/trefas/asledger/internal/version"
)

// usageText — справка по команде.
const usageText = `asledgerctl — утилита администратора asledger

Использование:
  asledgerctl <подкоманда> [флаги]

Подкоманды:
  version   показать версию сборки
  help      показать эту справку

Коды выхода:
  0 — успех
  1 — ошибка выполнения
  2 — неизвестная подкоманда или неверные аргументы
`

func main() {
	os.Exit(run(os.Args[1:]))
}

// run выполняет подкоманду и возвращает код выхода.
func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}

	switch args[0] {
	case "version", "-version", "--version", "-v":
		fmt.Println("asledgerctl", version.String())
		return 0
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "asledgerctl: неизвестная подкоманда %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}
