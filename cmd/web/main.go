// Команда web — отдельный бинарник web-интерфейса asledger.
//
// Может запускаться как самостоятельный процесс, обслуживающий только
// web-интерфейс (полезно для отладки вёрстки), либо использоваться как
// отправная точка для решения о разделении интерфейса и демона (веха S4).
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
	"github.com/trefas/asledger/pkg/core/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "web:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel, cfg.LogFormat)

	db, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err := db.Migrate(context.Background()); err != nil {
		return err
	}

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

	srv := &http.Server{Addr: cfg.WebAddr, Handler: webServer.Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		logger.Info("web-интерфейс запущен", "addr", cfg.WebAddr, "version", version.String())
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("получен сигнал завершения, останавливаем сервер")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
