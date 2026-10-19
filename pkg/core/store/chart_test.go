package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
)

// TestASL_8_ParseChart проверяет разбор вложенного плана счетов из JSON.
func TestASL_8_ParseChart(t *testing.T) {
	t.Run("объект с accounts", func(t *testing.T) {
		chart, err := ParseChart([]byte(`{"name":"Тест","accounts":[
			{"name":"Активы","type":"актив","children":[{"name":"Наличные","type":"актив","min_balance":0}]}
		]}`))
		if err != nil {
			t.Fatalf("ParseChart: %v", err)
		}
		if chart.Name != "Тест" || len(chart.Accounts) != 1 || len(chart.Accounts[0].Children) != 1 {
			t.Fatalf("разобранный план: %+v", chart)
		}
	})

	t.Run("массив узлов", func(t *testing.T) {
		chart, err := ParseChart([]byte(`[{"name":"Доходы","type":"доход"}]`))
		if err != nil {
			t.Fatalf("ParseChart: %v", err)
		}
		if len(chart.Accounts) != 1 || chart.Accounts[0].Name != "Доходы" {
			t.Fatalf("разобранный план: %+v", chart)
		}
	})

	t.Run("неизвестное поле", func(t *testing.T) {
		if _, err := ParseChart([]byte(`{"accounts":[{"name":"Активы","type":"актив","цвет":"красный"}]}`)); err == nil {
			t.Fatal("неизвестное поле принято без ошибки")
		}
	})

	t.Run("пустой план", func(t *testing.T) {
		if _, err := ParseChart([]byte("   ")); err == nil {
			t.Fatal("пустой план принят без ошибки")
		}
	})
}

// TestASL_8_ImportChartTree проверяет рекурсивную генерацию путей и min_balance.
func TestASL_8_ImportChartTree(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	chart, err := ParseChart([]byte(`{"accounts":[
		{"name":"Имущество","type":"актив","children":[
			{"name":"Техника","type":"актив","children":[
				{"name":"Ноутбук","type":"актив","min_balance":-5000}
			]},
			{"name":"Мебель","type":"актив"}
		]}
	]}`))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}

	report, err := db.ImportChart(ctx, chart)
	if err != nil {
		t.Fatalf("ImportChart: %v", err)
	}
	if report.Created != 4 || report.Existing != 0 {
		t.Fatalf("отчёт об импорте: %+v", report)
	}

	for _, path := range []string{"Имущество", "Имущество:Техника", "Имущество:Техника:Ноутбук", "Имущество:Мебель"} {
		if _, err := db.AccountByPath(ctx, path); err != nil {
			t.Fatalf("счёт %q не создан: %v", path, err)
		}
	}

	laptop, err := db.AccountByPath(ctx, "Имущество:Техника:Ноутбук")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if laptop.MinBalance != money.FromKopecks(-5000) {
		t.Fatalf("min_balance = %s, ожидалось -50,00 ₽", laptop.MinBalance)
	}
	if !laptop.IsLeaf {
		t.Fatal("листовой счёт помечен не листом")
	}
	folder, err := db.AccountByPath(ctx, "Имущество:Техника")
	if err != nil {
		t.Fatalf("AccountByPath: %v", err)
	}
	if folder.IsLeaf {
		t.Fatal("счёт с потомком помечен листом")
	}
}

// TestASL_8_ImportChartDuplicate проверяет, что дубликат пути откатывает импорт.
func TestASL_8_ImportChartDuplicate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	before := countAccounts(t, db)

	chart, err := ParseChart([]byte(`{"accounts":[
		{"name":"Имущество","type":"актив","children":[{"name":"Техника","type":"актив"}]},
		{"name":"Имущество","type":"актив"}
	]}`))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}

	if _, err := db.ImportChart(ctx, chart); !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("err=%v, ожидалась ErrDuplicatePath", err)
	}
	if after := countAccounts(t, db); after != before {
		t.Fatalf("после неудачного импорта счетов %d, было %d", after, before)
	}
}

// TestASL_8_ImportChartTypeMismatch проверяет несогласованный тип в файле.
func TestASL_8_ImportChartTypeMismatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	chart, err := ParseChart([]byte(`{"accounts":[
		{"name":"Активы2","type":"актив","children":[{"name":"Питание","type":"расход"}]}
	]}`))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}

	if _, err := db.ImportChart(ctx, chart); err == nil {
		t.Fatal("импорт с несогласованным типом прошёл без ошибки")
	}
}

// TestASL_8_ImportChartIdempotent проверяет повторный импорт того же плана.
func TestASL_8_ImportChartIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	chart, err := ParseChart([]byte(`{"accounts":[{"name":"Имущество","type":"актив","children":[{"name":"Техника","type":"актив"}]}]}`))
	if err != nil {
		t.Fatalf("ParseChart: %v", err)
	}

	first, err := db.ImportChart(ctx, chart)
	if err != nil {
		t.Fatalf("первый импорт: %v", err)
	}
	if first.Created != 2 {
		t.Fatalf("первый импорт создал %d счетов, ожидалось 2", first.Created)
	}

	second, err := db.ImportChart(ctx, chart)
	if err != nil {
		t.Fatalf("повторный импорт: %v", err)
	}
	if second.Created != 0 || second.Existing != 2 {
		t.Fatalf("отчёт повторного импорта: %+v", second)
	}
}

// TestASL_8_ChartTemplatesImport проверяет, что каждый шаблон из templates/charts
// проходит импортёр (CI-валидация шаблонных планов счетов).
func TestASL_8_ChartTemplatesImport(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "templates", "charts", "*.json"))
	if err != nil {
		t.Fatalf("поиск шаблонов: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("в templates/charts не найдено ни одного шаблона")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("чтение %s: %v", file, err)
			}
			chart, err := ParseChart(data)
			if err != nil {
				t.Fatalf("разбор %s: %v", file, err)
			}
			db := openTestDB(t)
			report, err := db.ImportChart(context.Background(), chart)
			if err != nil {
				t.Fatalf("импорт %s: %v", file, err)
			}
			if report.Created == 0 {
				t.Fatalf("шаблон %s не создал ни одного счёта", file)
			}

			// Служебные счета присутствуют и остаются служебными.
			for _, path := range []string{
				"Капитал:Начальные", "Капитал:Корректировка", "Капитал:Переоценка", "Активы:Дебиторка",
			} {
				acc, err := db.AccountByPath(context.Background(), path)
				if err != nil {
					t.Fatalf("служебный счёт %q отсутствует после импорта %s: %v", path, file, err)
				}
				if !acc.IsSystem {
					t.Fatalf("счёт %q после импорта %s перестал быть служебным", path, file)
				}
			}

			// Типы потомков совпадают с типом родителя (parts := trimmed).
			accounts, err := db.ListAccounts(context.Background(), AccountFilter{})
			if err != nil {
				t.Fatalf("ListAccounts: %v", err)
			}
			byID := map[int64]model.Account{}
			for _, acc := range accounts {
				byID[acc.ID] = acc
			}
			for _, acc := range accounts {
				if acc.ParentID == nil {
					continue
				}
				if parent := byID[*acc.ParentID]; parent.Type != acc.Type {
					t.Fatalf("шаблон %s: тип счёта %q (%s) не совпадает с родителем %q (%s)",
						file, acc.FullPath, acc.Type, parent.FullPath, parent.Type)
				}
			}
		})
	}
}

// countAccounts возвращает число счетов в книге.
func countAccounts(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil {
		t.Fatalf("подсчёт счетов: %v", err)
	}
	return n
}
