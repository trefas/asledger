package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/trefas/asledger/pkg/core/hledger"
	"github.com/trefas/asledger/pkg/core/store"
)

// runImport обрабатывает подкоманды импорта книги.
func runImport(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "asledgerctl import: нужна подкоманда hledger\n\n")
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	switch args[0] {
	case "hledger":
		return runImportHledger(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "asledgerctl import: неизвестная подкоманда %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

// runImportHledger импортирует книгу в формате hledger plaintext.
func runImportHledger(args []string) int {
	fs := newFlagSet("asledgerctl import hledger")
	opts := commonFlags(fs)
	mode := fs.String("mode", "гибкий", "режим импорта: «гибкий» или «строгий»")
	actor := fs.String("actor", "", "автор проводок: идентификатор или имя (по умолчанию единственный пользователь книги)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, "asledgerctl import hledger: укажите ровно один файл книги\n\n")
		return 2
	}
	file := fs.Arg(0)

	importMode, err := hledger.ParseMode(*mode)
	if err != nil {
		return fail(err)
	}
	cfg, err := loadConfig(*opts.configPath, *opts.dbPath)
	if err != nil {
		return fail(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fail(fmt.Errorf("import: не удалось прочитать %q: %w", file, err))
	}

	ctx := context.Background()
	db, err := opts.open(cfg)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)
	if err := db.Migrate(ctx); err != nil {
		return fail(err)
	}

	actorID, err := resolveActor(ctx, db, *actor)
	if err != nil {
		return fail(err)
	}

	report, err := hledger.NewImporter(db).Import(ctx, data, hledger.Options{
		Mode: importMode, ActorID: actorID, FileName: file,
	})
	if err != nil {
		return fail(err)
	}
	printImportReport(report)
	if report.Rejected {
		// Ненулевой код возврата: строгий режим или неустранимая проблема
		// означают, что книга не изменена.
		return 1
	}
	return 0
}

// printImportReport печатает отчёт об импорте.
func printImportReport(report *hledger.Report) {
	fmt.Printf("режим: %s\n", report.Mode)
	if report.FileName != "" {
		fmt.Printf("файл: %s\n", report.FileName)
	}
	fmt.Printf("принято проводок: %d\n", report.Accepted)
	fmt.Printf("пропущено проводок: %d\n", report.Skipped)
	fmt.Printf("уже было импортировано: %d\n", report.Duplicates)
	if len(report.CreatedAccounts) > 0 {
		fmt.Printf("создано счетов: %d\n", len(report.CreatedAccounts))
		for _, path := range report.CreatedAccounts {
			fmt.Printf("  %s\n", path)
		}
	}
	for _, problem := range report.Problems {
		fmt.Printf("проблема: %s", problem.Error())
		if problem.Snippet != "" {
			fmt.Printf(" (%s)", strings.TrimSpace(problem.Snippet))
		}
		fmt.Println()
	}
	if report.Message != "" {
		fmt.Println(report.Message)
	}
}

// resolveActor определяет автора импортируемых проводок.
//
// Пустое значение означает единственного пользователя книги: если
// пользователей несколько, автора нужно указать явно.
func resolveActor(ctx context.Context, db *store.DB, name string) (int64, error) {
	if strings.TrimSpace(name) == "" {
		users, err := db.ListUsers(ctx)
		if err != nil {
			return 0, err
		}
		switch len(users) {
		case 0:
			return 0, errors.New("import: в книге нет пользователей — создайте пользователя в web-интерфейсе")
		case 1:
			return users[0].ID, nil
		default:
			return 0, errors.New("import: в книге несколько пользователей — укажите автора флагом -actor")
		}
	}
	if id, err := strconv.ParseInt(name, 10, 64); err == nil {
		if _, err := db.User(ctx, id); err != nil {
			return 0, fmt.Errorf("import: пользователь %d не найден", id)
		}
		return id, nil
	}
	users, err := db.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	for _, user := range users {
		if strings.EqualFold(user.Name, name) {
			return user.ID, nil
		}
	}
	return 0, fmt.Errorf("import: пользователь %q не найден", name)
}
