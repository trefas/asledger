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

func TestLoadKeySources(t *testing.T) {
	cfg, err := Load([]string{"-password-file", "/run/asledger/password"})
	if err != nil {
		t.Fatalf("Load с password-file: %v", err)
	}
	if cfg.Key.PasswordFile != "/run/asledger/password" {
		t.Fatalf("PasswordFile = %q", cfg.Key.PasswordFile)
	}

	cfg, err = Load([]string{"-password-fd", "3"})
	if err != nil {
		t.Fatalf("Load с password-fd: %v", err)
	}
	if cfg.Key.PasswordFD == nil || *cfg.Key.PasswordFD != 3 {
		t.Fatalf("PasswordFD = %v", cfg.Key.PasswordFD)
	}

	cfg, err = Load([]string{"-key-file", "/run/asledger/key"})
	if err != nil {
		t.Fatalf("Load с key-file: %v", err)
	}
	if cfg.Key.KeyFile != "/run/asledger/key" {
		t.Fatalf("KeyFile = %q", cfg.Key.KeyFile)
	}

	cfg, err = Load([]string{"-key-fd", "0"})
	if err != nil {
		t.Fatalf("Load с key-fd: %v", err)
	}
	if cfg.Key.KeyFD == nil || *cfg.Key.KeyFD != 0 {
		t.Fatalf("KeyFD = %v", cfg.Key.KeyFD)
	}

	cfg, err = Load([]string{"-allow-plaintext"})
	if err != nil {
		t.Fatalf("Load с allow-plaintext: %v", err)
	}
	if !cfg.AllowPlaintext {
		t.Fatal("AllowPlaintext не включился")
	}
}

func TestLoadKeySourceConflicts(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"два пароля", []string{"-password-file", "a", "-password-fd", "0"}},
		{"два ключа", []string{"-key-file", "a", "-key-fd", "0"}},
		{"пароль и ключ", []string{"-password-file", "a", "-key-file", "b"}},
		{"plaintext и пароль", []string{"-allow-plaintext", "-password-file", "a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(tc.args); err == nil {
				t.Fatalf("Load(%v) не вернул ошибку", tc.args)
			}
		})
	}
}

func TestLoadKeySourcesFromEnv(t *testing.T) {
	t.Setenv("ASLEDGER_PASSWORD_FILE", "/run/asledger/password")
	t.Setenv("ASLEDGER_KEY_FILE", "")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Key.PasswordFile != "/run/asledger/password" {
		t.Fatalf("PasswordFile из env = %q", cfg.Key.PasswordFile)
	}
}

func TestLoadAllowPlaintextFromEnv(t *testing.T) {
	t.Setenv("ASLEDGER_PASSWORD_FILE", "")
	t.Setenv("ASLEDGER_KEY_FILE", "")
	t.Setenv("ASLEDGER_ALLOW_PLAINTEXT", "true")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AllowPlaintext {
		t.Fatal("AllowPlaintext из env не включился")
	}
}
