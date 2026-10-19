package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/trefas/asledger/pkg/core/export"
	"github.com/trefas/asledger/pkg/core/hledger"
	"github.com/trefas/asledger/pkg/core/store"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// runExport обрабатывает подкоманды выгрузки книги.
func runExport(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "asledgerctl export: нужна подкоманда hledger, csv или json\n\n")
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	switch args[0] {
	case "hledger":
		return runExportHledger(args[1:])
	case "csv":
		return runExportCSV(args[1:])
	case "json":
		return runExportJSON(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "asledgerctl export: неизвестная подкоманда %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

// runExportHledger выгружает книгу в plaintext-формат hledger.
func runExportHledger(args []string) int {
	fs := newFlagSet("asledgerctl export hledger")
	configPath, dbPath := commonFlags(fs)
	out := fs.String("out", "", "файл выгрузки (по умолчанию — стандартный вывод)")
	archived := fs.Bool("archived", false, "включить архивные счета")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	db, err := openBook(*configPath, *dbPath)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)

	ctx := context.Background()
	var buf io.Writer
	if *out == "" {
		buf = os.Stdout
	} else {
		file, err := os.Create(*out) //nolint:gosec // путь задаёт администратор книги
		if err != nil {
			return fail(fmt.Errorf("export: не удалось создать %q: %w", *out, err))
		}
		defer func() { _ = file.Close() }()
		buf = file
	}

	if err := hledger.Export(ctx, db, buf, hledger.WriteOptions{
		Comment:         "asledger: выгрузка книги",
		IncludeArchived: *archived,
	}); err != nil {
		return fail(err)
	}
	if *out != "" {
		fmt.Printf("книга выгружена: %s\n", *out)
	}
	return 0
}

// runExportCSV выгружает журнал или отчёт в CSV.
func runExportCSV(args []string) int {
	fs := newFlagSet("asledgerctl export csv")
	configPath, dbPath := commonFlags(fs)
	report := fs.String("report", "journal", "отчёт: journal, balances или turnover")
	from := fs.String("from", "", "начало периода оборота (ГГГГ-ММ-ДД)")
	to := fs.String("to", "", "конец периода оборота (ГГГГ-ММ-ДД)")
	out := fs.String("out", "", "файл выгрузки (по умолчанию — стандартный вывод)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	db, err := openBook(*configPath, *dbPath)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)

	ctx := context.Background()
	accounts, err := db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return fail(err)
	}

	var buf io.Writer = os.Stdout
	if *out != "" {
		file, err := os.Create(*out) //nolint:gosec // путь задаёт администратор книги
		if err != nil {
			return fail(fmt.Errorf("export: не удалось создать %q: %w", *out, err))
		}
		defer func() { _ = file.Close() }()
		buf = file
	}

	switch *report {
	case "journal":
		transactions, err := db.ListTransactions(ctx, store.TransactionFilter{})
		if err != nil {
			return fail(err)
		}
		err = export.Journal(buf, accounts, transactions)
		if err != nil {
			return fail(err)
		}
	case "balances":
		balances, err := db.Balances(ctx)
		if err != nil {
			return fail(err)
		}
		if err := export.Balances(buf, accounts, balances); err != nil {
			return fail(err)
		}
	case "turnover":
		fromUnix, err := parseBound(*from, false)
		if err != nil {
			return fail(err)
		}
		toUnix, err := parseBound(*to, true)
		if err != nil {
			return fail(err)
		}
		turnovers, err := db.Turnovers(ctx, fromUnix, toUnix)
		if err != nil {
			return fail(err)
		}
		if err := export.Turnovers(buf, accounts, turnovers); err != nil {
			return fail(err)
		}
	default:
		return fail(fmt.Errorf("export: неизвестный отчёт %q (ожидается journal, balances или turnover)", *report))
	}

	if *out != "" {
		fmt.Printf("выгрузка: %s\n", *out)
	}
	return 0
}

// runExportJSON выгружает полный снапшот книги.
func runExportJSON(args []string) int {
	fs := newFlagSet("asledgerctl export json")
	configPath, dbPath := commonFlags(fs)
	out := fs.String("out", "", "файл выгрузки (по умолчанию — стандартный вывод)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	db, err := openBook(*configPath, *dbPath)
	if err != nil {
		return fail(err)
	}
	defer closeDB(db)

	snapshot, err := syncsvc.New(db).Snapshot(context.Background())
	if err != nil {
		return fail(err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fail(fmt.Errorf("export: не удалось сериализовать снапшот: %w", err))
	}
	data = append(data, '\n')

	if *out == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fail(err)
		}
		return 0
	}
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		return fail(fmt.Errorf("export: не удалось записать %q: %w", *out, err))
	}
	fmt.Printf("снапшот книги: %s\n", *out)
	return 0
}

// openBook читает конфигурацию и открывает книгу.
func openBook(configPath, dbPath string) (*store.DB, error) {
	cfg, err := loadConfig(configPath, dbPath)
	if err != nil {
		return nil, err
	}
	db, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// parseBound разбирает границу периода оборота.
//
// Начало периода включается с начала дня, конец — по конец дня.
func parseBound(text string, endOfDay bool) (int64, error) {
	if text == "" {
		return 0, nil
	}
	date, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return 0, fmt.Errorf("export: неверная дата %q (ожидается ГГГГ-ММ-ДД)", text)
	}
	unix := date.UTC().Unix()
	if endOfDay {
		unix = date.UTC().AddDate(0, 0, 1).Unix() - 1
	}
	return unix, nil
}
