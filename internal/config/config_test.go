package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil) вернул ошибку: %v", err)
	}
	if cfg.WebAddr != ":8777" {
		t.Fatalf("WebAddr = %q, ожидалось :8777", cfg.WebAddr)
	}
	if cfg.APIAddr != ":8778" {
		t.Fatalf("APIAddr = %q, ожидалось :8778", cfg.APIAddr)
	}
	if cfg.DBPath != DefaultDBPath || cfg.AssetsDir != DefaultAssetsDir {
		t.Fatalf("пути по умолчанию не совпадают: %+v", cfg)
	}
}

func TestConfigFileThenEnvThenFlags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asledger.json")
	contents := `{"web_addr": ":18877", "api_addr": ":18878", "db_path": "file.db", "log_level": "warn"}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("не удалось записать конфиг: %v", err)
	}

	// Файл перекрывает значения по умолчанию.
	cfg, err := Load([]string{"-config", path})
	if err != nil {
		t.Fatalf("Load с файлом вернул ошибку: %v", err)
	}
	if cfg.WebAddr != ":18877" || cfg.DBPath != "file.db" || cfg.LogLevel != "warn" {
		t.Fatalf("файл конфигурации не применился: %+v", cfg)
	}
	if cfg.APIAddr != ":18878" {
		t.Fatalf("APIAddr из файла = %q, ожидалось :18878", cfg.APIAddr)
	}

	// Переменная окружения перекрывает файл.
	t.Setenv("ASLEDGER_WEB_ADDR", ":28777")
	cfg, err = Load([]string{"-config", path})
	if err != nil {
		t.Fatalf("Load с env вернул ошибку: %v", err)
	}
	if cfg.WebAddr != ":28777" {
		t.Fatalf("env не перекрыл файл: WebAddr = %q", cfg.WebAddr)
	}

	// Флаг перекрывает переменную окружения.
	cfg, err = Load([]string{"-config", path, "-web-addr", ":38777"})
	if err != nil {
		t.Fatalf("Load с флагом вернул ошибку: %v", err)
	}
	if cfg.WebAddr != ":38777" {
		t.Fatalf("флаг не перекрыл env: WebAddr = %q", cfg.WebAddr)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"пустой адрес web", []string{"-web-addr", ""}},
		{"пустой адрес api", []string{"-api-addr", ""}},
		{"пустой путь к книге", []string{"-db", ""}},
		{"пустой каталог ассетов", []string{"-assets", ""}},
		{"неизвестный уровень журнала", []string{"-log-level", "verbose"}},
		{"неизвестный формат журнала", []string{"-log-format", "xml"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(tc.args); err == nil {
				t.Fatalf("Load(%v) не вернул ошибку", tc.args)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load([]string{"-config", filepath.Join(t.TempDir(), "нет.json")}); err == nil {
		t.Fatal("Load с несуществующим файлом не вернул ошибку")
	}
}
