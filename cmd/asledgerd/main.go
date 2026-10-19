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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trefas/asledger/internal/config"
	"github.com/trefas/asledger/internal/httpui"
	"github.com/trefas/asledger/internal/logging"
	"github.com/trefas/asledger/internal/version"
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
	if hasVersionFlag(args) {
		fmt.Println("asledgerd", version.String())
		return nil
	}

	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel, cfg.LogFormat)

	servers := []*http.Server{
		{Addr: cfg.WebAddr, Handler: httpui.NewWebMux(cfg.AssetsDir, version.Version)},
		{Addr: cfg.APIAddr, Handler: httpui.NewAPIMux(version.Version)},
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, len(servers))
	for _, srv := range servers {
		go func(srv *http.Server) {
			logger.Info("сервер запущен", "addr", srv.Addr)
			errc <- srv.ListenAndServe()
		}(srv)
	}
	logger.Info("asledgerd запущен",
		"version", version.String(),
		"web", cfg.WebAddr,
		"api", cfg.APIAddr,
		"db", cfg.DBPath,
	)

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("получен сигнал завершения, останавливаем серверы")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("ошибка остановки сервера", "addr", srv.Addr, "err", err)
		}
	}
	logger.Info("asledgerd остановлен")
	return nil
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
