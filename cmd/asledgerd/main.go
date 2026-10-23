// Команда asledgerd — демон-служба asledger.
//
// Поднимает REST API (по умолчанию :8778) и web-интерфейс (:8777) поверх книги
// SQLite в режиме WAL. Демон является единственной точкой записи книги.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trefas/asledger/internal/api"
	"github.com/trefas/asledger/internal/config"
	"github.com/trefas/asledger/internal/httpui"
	"github.com/trefas/asledger/internal/key"
	"github.com/trefas/asledger/internal/logging"
	"github.com/trefas/asledger/internal/version"
	"github.com/trefas/asledger/pkg/core/store"
)

// Таймауты HTTP-серверов (раздел 12: демон — единственная точка записи).
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "asledgerd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runContext(ctx, args)
}

// runContext выполняет запуск с переданным контекстом. Отдельная функция
// позволяет тестировать сервер и корректную остановку без сигналов процесса.
func runContext(ctx context.Context, args []string) error {
	if hasVersionFlag(args) {
		fmt.Println("asledgerd", version.String())
		return nil
	}

	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel, cfg.LogFormat)

	var openOpts []store.Option
	if cfg.AllowPlaintext {
		logger.Warn("книга открывается без шифрования: явный режим -allow-plaintext")
	} else {
		provider, err := key.NewProvider(cfg.DBPath, cfg.Key)
		if err != nil {
			return err
		}
		openOpts = append(openOpts, store.WithKeyProvider(provider))
	}

	db, err := store.Open(ctx, cfg.DBPath, openOpts...)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	apiServer := api.New(db, api.Config{
		Version:   version.Version,
		ChartsDir: cfg.ChartsDir,
		Logger:    logger,
	})

	webServer, err := httpui.New(httpui.Config{
		Version:   version.Version,
		AssetsDir: cfg.AssetsDir,
		ChartsDir: cfg.ChartsDir,
		DB:        db,
		Logger:    logger,
	})
	if err != nil {
		return err
	}

	servers := []namedServer{
		{
			name: "web",
			srv: &http.Server{
				Addr:              cfg.WebAddr,
				Handler:           webServer.Handler(),
				ReadHeaderTimeout: readHeaderTimeout,
				ReadTimeout:       readTimeout,
				WriteTimeout:      writeTimeout,
				IdleTimeout:       idleTimeout,
			},
		},
		{
			name: "api",
			srv: &http.Server{
				Addr:              cfg.APIAddr,
				Handler:           apiServer.Handler(),
				ReadHeaderTimeout: readHeaderTimeout,
				ReadTimeout:       readTimeout,
				WriteTimeout:      writeTimeout,
				IdleTimeout:       idleTimeout,
			},
		},
	}

	errc := make(chan error, len(servers))
	for _, item := range servers {
		go func(item namedServer) {
			listener, err := net.Listen("tcp", item.srv.Addr)
			if err != nil {
				errc <- fmt.Errorf("%s: %w", item.name, err)
				return
			}
			logger.Info("сервер запущен", "name", item.name, "addr", listener.Addr())
			if err := item.srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("%s: %w", item.name, err)
				return
			}
			errc <- nil
		}(item)
	}
	logger.Info("asledgerd запущен",
		"version", version.String(),
		"web", cfg.WebAddr,
		"api", cfg.APIAddr,
		"db", cfg.DBPath,
	)

	var runErr error
	select {
	case err := <-errc:
		runErr = err
	case <-ctx.Done():
		logger.Info("получен сигнал завершения, останавливаем серверы")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, item := range servers {
		if err := item.srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("ошибка остановки сервера", "name", item.name, "addr", item.srv.Addr, "err", err)
		}
	}
	logger.Info("asledgerd остановлен")
	return runErr
}

// namedServer связывает HTTP-сервер с понятным именем для диагностики.
type namedServer struct {
	name string
	srv  *http.Server
}

// hasVersionFlag сообщает, запрошен ли вывод версии.
func hasVersionFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "-version", "--version", "-v":
			return true
		}
	}
	return false
}
