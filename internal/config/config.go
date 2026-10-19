// Package config загружает конфигурацию комплекса asledger.
//
// Источники конфигурации применяются по возрастанию приоритета:
// значения по умолчанию → файл JSON → переменные окружения → флаги командной
// строки. Путь к файлу задаётся флагом -config или переменной ASLEDGER_CONFIG.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Значения по умолчанию.
const (
	// DefaultWebAddr — адрес web-интерфейса.
	DefaultWebAddr = ":8777"
	// DefaultAPIAddr — адрес REST API.
	DefaultAPIAddr = ":8778"
	// DefaultDBPath — путь к файлу книги SQLite.
	DefaultDBPath = "asledger.db"
	// DefaultAssetsDir — каталог бренд-ассетов (лого и favicon).
	DefaultAssetsDir = "assets/logo"
	// DefaultChartsDir — каталог шаблонных планов счетов.
	DefaultChartsDir = "templates/charts"
	// DefaultLogLevel — уровень журналирования по умолчанию.
	DefaultLogLevel = "info"
	// DefaultLogFormat — формат журнала по умолчанию.
	DefaultLogFormat = "text"
)

// Config — действующая конфигурация комплекса.
type Config struct {
	// WebAddr — адрес прослушивания web-интерфейса.
	WebAddr string `json:"web_addr"`
	// APIAddr — адрес прослушивания REST API.
	APIAddr string `json:"api_addr"`
	// DBPath — путь к файлу книги SQLite.
	DBPath string `json:"db_path"`
	// AssetsDir — каталог бренд-ассетов.
	AssetsDir string `json:"assets_dir"`
	// ChartsDir — каталог шаблонных планов счетов.
	ChartsDir string `json:"charts_dir"`
	// LogLevel — уровень журналирования: debug, info, warn, error.
	LogLevel string `json:"log_level"`
	// LogFormat — формат журнала: text или json.
	LogFormat string `json:"log_format"`
}

// Default возвращает конфигурацию со значениями по умолчанию.
func Default() *Config {
	return &Config{
		WebAddr:   DefaultWebAddr,
		APIAddr:   DefaultAPIAddr,
		DBPath:    DefaultDBPath,
		AssetsDir: DefaultAssetsDir,
		ChartsDir: DefaultChartsDir,
		LogLevel:  DefaultLogLevel,
		LogFormat: DefaultLogFormat,
	}
}

// Load собирает конфигурацию из всех источников.
//
// Аргументы args — список аргументов командной строки без имени программы.
// При передаче -h/--help возвращается flag.ErrHelp.
func Load(args []string) (*Config, error) {
	cfg := Default()

	configPath := os.Getenv("ASLEDGER_CONFIG")
	if p := scanConfigFlag(args); p != "" {
		configPath = p
	}
	if configPath != "" {
		if err := cfg.loadFile(configPath); err != nil {
			return nil, err
		}
	}

	cfg.applyEnv(os.LookupEnv)
	if err := cfg.applyFlags(args); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadFile читает конфигурацию из JSON-файла.
func (c *Config) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: не удалось прочитать файл %q: %w", path, err)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("config: не удалось разобрать файл %q: %w", path, err)
	}
	return nil
}

// scanConfigFlag ищет флаг -config/--config в аргументах командной строки.
func scanConfigFlag(args []string) string {
	fs := flag.NewFlagSet("config-scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("config", "", "")
	// Ошибки разбора здесь несущественны: полный разбор выполняется позже.
	_ = fs.Parse(args)
	return *path
}

// applyEnv перекрывает конфигурацию значениями переменных окружения.
func (c *Config) applyEnv(lookup func(string) (string, bool)) {
	set := func(dst *string, keys ...string) {
		for _, key := range keys {
			if v, ok := lookup(key); ok && v != "" {
				*dst = v
				return
			}
		}
	}
	set(&c.WebAddr, "ASLEDGER_WEB_ADDR")
	set(&c.APIAddr, "ASLEDGER_API_ADDR")
	set(&c.DBPath, "ASLEDGER_DB_PATH")
	set(&c.AssetsDir, "ASLEDGER_ASSETS_DIR")
	set(&c.ChartsDir, "ASLEDGER_CHARTS_DIR")
	set(&c.LogLevel, "ASLEDGER_LOG_LEVEL")
	set(&c.LogFormat, "ASLEDGER_LOG_FORMAT")
}

// applyFlags перекрывает конфигурацию аргументами командной строки.
func (c *Config) applyFlags(args []string) error {
	fs := flag.NewFlagSet("asledger", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.String("config", "", "путь к файлу конфигурации JSON")
	fs.StringVar(&c.WebAddr, "web-addr", c.WebAddr, "адрес web-интерфейса")
	fs.StringVar(&c.APIAddr, "api-addr", c.APIAddr, "адрес REST API")
	fs.StringVar(&c.DBPath, "db", c.DBPath, "путь к файлу книги SQLite")
	fs.StringVar(&c.AssetsDir, "assets", c.AssetsDir, "каталог бренд-ассетов")
	fs.StringVar(&c.ChartsDir, "charts-dir", c.ChartsDir, "каталог шаблонных планов счетов")
	fs.StringVar(&c.LogLevel, "log-level", c.LogLevel, "уровень журнала: debug, info, warn, error")
	fs.StringVar(&c.LogFormat, "log-format", c.LogFormat, "формат журнала: text или json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("config: неожиданные аргументы: %s", strings.Join(fs.Args(), " "))
	}
	return nil
}

// Validate проверяет корректность конфигурации.
func (c *Config) Validate() error {
	switch {
	case strings.TrimSpace(c.WebAddr) == "":
		return errors.New("config: не задан адрес web-интерфейса (web-addr)")
	case strings.TrimSpace(c.APIAddr) == "":
		return errors.New("config: не задан адрес REST API (api-addr)")
	case strings.TrimSpace(c.DBPath) == "":
		return errors.New("config: не задан путь к книге (db)")
	case strings.TrimSpace(c.AssetsDir) == "":
		return errors.New("config: не задан каталог ассетов (assets)")
	case strings.TrimSpace(c.ChartsDir) == "":
		return errors.New("config: не задан каталог шаблонов (charts-dir)")
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("config: недопустимый уровень журнала %q", c.LogLevel)
	}
	switch strings.ToLower(c.LogFormat) {
	case "text", "json":
	default:
		return fmt.Errorf("config: недопустимый формат журнала %q", c.LogFormat)
	}
	return nil
}
