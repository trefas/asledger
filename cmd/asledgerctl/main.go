// Команда asledgerctl — утилита администратора asledger.
//
// Подкоманды: migrate (привести книгу к актуальной схеме), backup (создать
// консистентную резервную копию), charts list|import (шаблонные планы счетов),
// import hledger (импорт книги hledger), export hledger|csv|json (выгрузка
// книги) и version. Конфигурация — та же, что у asledgerd: файл JSON,
// переменные окружения ASLEDGER_* и флаги командной строки.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/trefas/asledger/internal/config"
	"github.com/trefas/asledger/internal/version"
	"github.com/trefas/asledger/pkg/core/store"
)

// defaultChartsDir — каталог шаблонных планов счетов по умолчанию.
const defaultChartsDir = "templates/charts"

// usageText — справка по команде.
const usageText = `asledgerctl — утилита администратора asledger

Использование:
  asledgerctl <подкоманда> [флаги]

Подкоманды:
  migrate                привести книгу к актуальной схеме
  backup                 создать консистентную резервную копию книги
  charts list            показать доступные шаблоны планов счетов
  charts import <файл>   импортировать план счетов из JSON
  import hledger <файл>  импортировать книгу hledger (флаги -mode, -actor)
  export hledger         выгрузить книгу в plaintext hledger (флаг -archived)
  export csv             выгрузить журнал или отчёт в CSV (флаги -report, -from, -to)
  export json            выгрузить полный снапшот книги
  version                показать версию сборки
  help                   показать эту справку

Общие флаги:
  -config <путь>   файл конфигурации JSON (или ASLEDGER_CONFIG)
  -db <путь>       путь к книге SQLite (или ASLEDGER_DB_PATH)
  -out <путь>      файл выгрузки (у export; по умолчанию — стандартный вывод)

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
	case "migrate":
		return runMigrate(args[1:])
	case "backup":
		return runBackup(args[1:])
	case "charts":
		return runCharts(args[1:])
	case "import":
		return runImport(args[1:])
	case "export":
		return runExport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "asledgerctl: неизвестная подкоманда %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

// runMigrate применяет миграции к книге.
func runMigrate(args []string) int {
	fs := newFlagSet("asledgerctl migrate")
	configPath, dbPath := commonFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	cfg, err := loadConfig(*configPath, *dbPath)
	if err != nil {
		return fail(err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)

	if err := db.Migrate(ctx); err != nil {
		return fail(err)
	}
	fmt.Printf("книга %s: схема актуальна\n", cfg.DBPath)
	return 0
}

// runBackup создаёт консистентную копию книги с учётом режима WAL.
func runBackup(args []string) int {
	fs := newFlagSet("asledgerctl backup")
	configPath, dbPath := commonFlags(fs)
	out := fs.String("out", "", "путь к файлу резервной копии (по умолчанию рядом с книгой)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	cfg, err := loadConfig(*configPath, *dbPath)
	if err != nil {
		return fail(err)
	}

	target := *out
	if target == "" {
		target = fmt.Sprintf("%s.backup-%s", cfg.DBPath, time.Now().Format("20060102-150405"))
	}
	if _, err := os.Stat(target); err == nil {
		return fail(fmt.Errorf("backup: файл %q уже существует", target))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Errorf("backup: не удалось проверить %q: %w", target, err))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fail(fmt.Errorf("backup: не удалось создать каталог: %w", err))
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)

	// VACUUM INTO создаёт консистентную копию даже при активном WAL.
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return fail(fmt.Errorf("backup: не удалось создать копию: %w", err))
	}
	fmt.Printf("резервная копия: %s\n", target)
	return 0
}

// runCharts обрабатывает подкоманды работы с шаблонами планов счетов.
func runCharts(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "asledgerctl charts: нужна подкоманда list или import\n\n")
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	switch args[0] {
	case "list":
		return runChartsList(args[1:])
	case "import":
		return runChartsImport(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "asledgerctl charts: неизвестная подкоманда %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

// runChartsList печатает список шаблонов из каталога.
func runChartsList(args []string) int {
	fs := newFlagSet("asledgerctl charts list")
	dir := fs.String("charts-dir", defaultChartsDir, "каталог шаблонных планов счетов")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}

	charts, err := listCharts(*dir)
	if err != nil {
		return fail(err)
	}
	if len(charts) == 0 {
		fmt.Printf("шаблоны не найдены в %s\n", *dir)
		return 0
	}
	for _, c := range charts {
		fmt.Printf("%-20s %s\n", filepath.Base(c.path), c.name)
	}
	return 0
}

// runChartsImport импортирует план счетов из файла.
func runChartsImport(args []string) int {
	fs := newFlagSet("asledgerctl charts import")
	configPath, dbPath := commonFlags(fs)
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, "asledgerctl charts import: укажите ровно один файл плана счетов\n\n")
		return 2
	}
	file := fs.Arg(0)

	cfg, err := loadConfig(*configPath, *dbPath)
	if err != nil {
		return fail(err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return fail(fmt.Errorf("charts import: не удалось прочитать %q: %w", file, err))
	}
	chart, err := store.ParseChart(data)
	if err != nil {
		return fail(err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)
	if err := db.Migrate(ctx); err != nil {
		return fail(err)
	}

	report, err := db.ImportChart(ctx, chart)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("план счетов импортирован: создано %d, уже было %d\n", report.Created, report.Existing)
	return 0
}

// chartInfo — сведения о шаблоне плана счетов.
type chartInfo struct {
	path string
	name string
}

// listCharts читает каталог шаблонов и возвращает их отсортированными по имени.
func listCharts(dir string) ([]chartInfo, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("charts: не удалось прочитать каталог %q: %w", dir, err)
	}
	sort.Strings(files)

	charts := make([]chartInfo, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("charts: не удалось прочитать %q: %w", file, err)
		}
		chart, err := store.ParseChart(data)
		if err != nil {
			return nil, fmt.Errorf("charts: файл %q не прошёл разбор: %w", file, err)
		}
		if err := store.ValidateChart(chart); err != nil {
			return nil, fmt.Errorf("charts: файл %q не прошёл проверку: %w", file, err)
		}
		name := chart.Name
		if name == "" {
			name = "(без названия)"
		}
		charts = append(charts, chartInfo{path: file, name: name})
	}
	return charts, nil
}

// newFlagSet создаёт набор флагов подкоманды, печатающий справку в stderr.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// commonFlags объявляет общие флаги -config и -db, общие для всех подкоманд,
// работающих с книгой (единая конфигурация с asledgerd).
func commonFlags(fs *flag.FlagSet) (configPath, dbPath *string) {
	configPath = fs.String("config", "", "путь к файлу конфигурации JSON")
	dbPath = fs.String("db", "", "путь к файлу книги SQLite")
	return configPath, dbPath
}

// parseFlags разбирает аргументы, считая -h/--help успешным завершением.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		return err
	}
	return nil
}

// loadConfig собирает конфигурацию из файла, окружения и переданных флагов.
func loadConfig(configPath, dbPath string) (*config.Config, error) {
	var args []string
	if configPath != "" {
		args = append(args, "-config", configPath)
	}
	if dbPath != "" {
		args = append(args, "-db", dbPath)
	}
	return config.Load(args)
}

// closeDB закрывает книгу, сообщая об ошибке закрытия.
func closeDB(db *store.DB) {
	if err := db.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "asledgerctl: ошибка закрытия книги: %v\n", err)
	}
}

// fail печатает ошибку и возвращает код выхода для ошибок выполнения.
func fail(err error) int {
	fmt.Fprintf(os.Stderr, "asledgerctl: %v\n", err)
	return 1
}
