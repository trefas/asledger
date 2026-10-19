package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// writeFile создаёт файл с содержимым и завершает тест при ошибке.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("не удалось записать %q: %v", path, err)
	}
}

// TestRunVersionAndHelp проверяет служебные подкоманды.
func TestRunVersionAndHelp(t *testing.T) {
	if code := run([]string{"version"}); code != 0 {
		t.Fatalf("version: код %d, ожидался 0", code)
	}
	if code := run([]string{"help"}); code != 0 {
		t.Fatalf("help: код %d, ожидался 0", code)
	}
}

// TestRunUnknown проверяет коды выхода на неизвестных подкомандах.
func TestRunUnknown(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Fatalf("пустой вызов: код %d, ожидался 2", code)
	}
	if code := run([]string{"посчитай"}); code != 2 {
		t.Fatalf("неизвестная подкоманда: код %d, ожидался 2", code)
	}
	if code := run([]string{"charts"}); code != 2 {
		t.Fatalf("charts без подкоманды: код %d, ожидался 2", code)
	}
}

// TestRunMigrate проверяет приведение книги к актуальной схеме.
func TestRunMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "book.db")

	if code := run([]string{"migrate", "-db", dbPath}); code != 0 {
		t.Fatalf("migrate: код %d, ожидался 0", code)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.AccountByPath(ctx, "Капитал:Начальные"); err != nil {
		t.Fatalf("после migrate служебные счета отсутствуют: %v", err)
	}
}

// TestRunBackup проверяет создание консистентной копии книги.
func TestRunBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	backupPath := filepath.Join(dir, "backup.db")

	if code := run([]string{"migrate", "-db", dbPath}); code != 0 {
		t.Fatalf("migrate: код %d", code)
	}
	if code := run([]string{"backup", "-db", dbPath, "-out", backupPath}); code != 0 {
		t.Fatalf("backup: код %d, ожидался 0", code)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("файл резервной копии не создан: %v", err)
	}
	ctx := context.Background()
	db, err := store.Open(ctx, backupPath)
	if err != nil {
		t.Fatalf("открытие копии: %v", err)
	}
	defer db.Close()
	if _, err := db.AccountByPath(ctx, "Активы:Дебиторка"); err != nil {
		t.Fatalf("в копии нет служебных счетов: %v", err)
	}

	// Повторная запись в существующий файл отклоняется.
	if code := run([]string{"backup", "-db", dbPath, "-out", backupPath}); code != 1 {
		t.Fatalf("повторный backup в существующий файл: код %d, ожидался 1", code)
	}
}

// TestRunChartsList проверяет список шаблонов и реакцию на битый файл.
func TestRunChartsList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "good.json"),
		`{"name":"Тестовый план","accounts":[{"name":"Активы","type":"актив"}]}`)

	if code := run([]string{"charts", "list", "-charts-dir", dir}); code != 0 {
		t.Fatalf("charts list: код %d, ожидался 0", code)
	}

	writeFile(t, filepath.Join(dir, "bad.json"), `{"accounts":[{"name":"???","type":"нет"}]}`)
	if code := run([]string{"charts", "list", "-charts-dir", dir}); code != 1 {
		t.Fatalf("charts list с битым файлом: код %d, ожидался 1", code)
	}
}

// TestRunChartsImport проверяет импорт шаблона через CLI.
func TestRunChartsImport(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "chart.json")
	writeFile(t, chartPath,
		`{"name":"Тестовый план","accounts":[{"name":"Имущество","type":"актив","children":[{"name":"Техника","type":"актив"}]}]}`)
	dbPath := filepath.Join(dir, "book.db")

	if code := run([]string{"charts", "import", "-db", dbPath, chartPath}); code != 0 {
		t.Fatalf("charts import: код %d, ожидался 0", code)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.AccountByPath(ctx, "Имущество:Техника"); err != nil {
		t.Fatalf("счёт из шаблона не создан: %v", err)
	}

	// Неверное число позиционных аргументов — код 2.
	if code := run([]string{"charts", "import", "-db", dbPath}); code != 2 {
		t.Fatalf("charts import без файла: код %d, ожидался 2", code)
	}
}
