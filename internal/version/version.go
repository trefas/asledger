// Package version хранит сведения о версии сборки asledger.
//
// Значения переменных переопределяются при сборке релиза через
// -ldflags "-X github.com/trefas/asledger/internal/version.Version=…".
package version

import "fmt"

// Сведения о сборке; значения по умолчанию — для локальных сборок.
var (
	// Version — версия сборки (например, «v1.0.0»).
	Version = "dev"
	// Commit — хеш коммита, из которого собрана версия.
	Commit = "none"
	// Date — дата сборки в формате RFC3339 (или «unknown»).
	Date = "unknown"
)

// String возвращает человекочитаемое описание сборки.
func String() string {
	return fmt.Sprintf("%s (%s, %s)", Version, Commit, Date)
}
