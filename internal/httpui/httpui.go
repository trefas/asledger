// Package httpui реализует web-интерфейс asledger (порт 8777).
//
// Рендеринг серверный (html/template) — без сборщиков и внешних зависимостей;
// прогрессивное улучшение даёт небольшой файл static/app.js. Интерфейс
// полностью на кириллице, денежные величины выводятся в рублях («₽»).
//
// Web-интерфейс работает с книгой в том же процессе, что и демон, и является
// частью единственной точки записи: формы сохраняют данные через движок,
// поэтому все инварианты матрицы ASL-2.2 соблюдаются и здесь.
package httpui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	"github.com/trefas/asledger/pkg/core/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Config — параметры web-интерфейса.
type Config struct {
	// Version — версия сборки для подвала страниц.
	Version string
	// AssetsDir — единый каталог бренд-ассетов (лого, favicon, иконки).
	AssetsDir string
	// ChartsDir — каталог шаблонных планов счетов.
	ChartsDir string
	// DB — книга.
	DB *store.DB
	// Logger — журнал; при nil используется no-op.
	Logger *slog.Logger
}

// Server — обработчики web-интерфейса.
type Server struct {
	cfg    Config
	db     *store.DB
	engine *engine.Engine
	logger *slog.Logger
	pages  map[string]*template.Template
	static http.Handler
}

// pageData — общие данные страницы (layout + содержимое).
type pageData struct {
	// Title — заголовок страницы.
	Title string
	// Active — активный пункт навигации.
	Active string
	// Version — версия сборки.
	Version string
	// Setup — книга ещё без плана счетов, нужна первичная настройка.
	Setup bool
	// HasChart — план счетов создан (показывать разделы работы с книгой).
	HasChart bool
	// Flash — сообщение о результате действия.
	Flash string
	// FlashKind — вид сообщения: ok или error.
	FlashKind string
	// Data — данные конкретной страницы.
	Data any
}

// pageFiles сопоставляет имя страницы файлу шаблона.
var pageFiles = map[string]string{
	"journal": "journal.html",
	"entry":   "entry.html",
	"setup":   "setup.html",
	"error":   "error.html",
}

// New создаёт web-интерфейс и разбирает шаблоны.
func New(cfg Config) (*Server, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Server{
		cfg:    cfg,
		db:     cfg.DB,
		engine: engine.New(cfg.DB),
		logger: logger,
		pages:  make(map[string]*template.Template),
	}
	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	s.static = http.FileServer(http.FS(static))
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadTemplates разбирает layout вместе с каждой страницей.
func (s *Server) loadTemplates() error {
	funcs := template.FuncMap{
		"money": func(k money.Kop) string { return k.String() },
		"date":  func(t time.Time) string { return t.Format("02.01.2006") },
		"side":  sideLabel,
	}
	for name, file := range pageFiles {
		t, err := template.New("layout").Funcs(funcs).
			ParseFS(templatesFS, "templates/layout.html", "templates/"+file)
		if err != nil {
			return fmt.Errorf("httpui: не удалось разобрать шаблон %q: %w", file, err)
		}
		s.pages[name] = t
	}
	return nil
}

// Handler собирает маршруты web-интерфейса.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(s.cfg.AssetsDir))))
	mux.HandleFunc("GET /favicon.ico", s.handleBrandFile("favicon.ico"))
	mux.HandleFunc("GET /manifest.webmanifest", s.handleBrandFile("manifest.webmanifest"))

	mux.HandleFunc("GET /{$}", s.handleJournal)
	mux.HandleFunc("GET /entry", s.handleEntryForm)
	mux.HandleFunc("POST /entry", s.handleEntrySubmit)

	mux.HandleFunc("GET /setup", s.handleSetupForm)
	mux.HandleFunc("POST /setup/template", s.handleSetupTemplate)
	mux.HandleFunc("POST /setup/import", s.handleSetupImport)

	mux.HandleFunc("GET /suggest.json", s.handleSuggestJSON)
	mux.HandleFunc("GET /matrix.json", s.handleMatrixJSON)

	mux.HandleFunc("/", s.handleNotFound)

	return s.recoverPanic(mux)
}

// handleNotFound отдаёт страницу 404 в оформлении web-интерфейса.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusNotFound, "Страница не найдена.")
}

// handleBrandFile отдаёт бренд-файл из единого каталога ассетов.
func (s *Server) handleBrandFile(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, s.cfg.AssetsDir+"/"+name)
	}
}

// recoverPanic не даёт панике в обработчике обрушить сервер.
func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("паника в web-обработчике", "err", rec, "path", r.URL.Path)
				http.Error(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// render отрисовывает страницу в общем layout.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data pageData) {
	t, ok := s.pages[name]
	if !ok {
		s.fail(w, fmt.Errorf("httpui: нет шаблона страницы %q", name))
		return
	}
	data.Version = s.cfg.Version
	data.Flash = r.URL.Query().Get("flash")
	data.FlashKind = r.URL.Query().Get("kind")
	if data.FlashKind == "" {
		data.FlashKind = "ok"
	}
	needsSetup, err := s.needsSetup(r.Context())
	if err == nil {
		data.Setup = needsSetup
		data.HasChart = !needsSetup
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать страницу", "page", name, "err", err)
	}
}

// renderError показывает страницу ошибки с понятным сообщением.
func (s *Server) renderError(w http.ResponseWriter, r *http.Request, code int, message string) {
	s.logger.Info("страница web-интерфейса не отдана", "path", r.URL.Path, "code", code)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	data := pageData{Title: "Ошибка", Version: s.cfg.Version, Data: errorView{Message: message}}
	if err := s.pages["error"].ExecuteTemplate(w, "layout", data); err != nil {
		s.logger.Error("не удалось отрисовать страницу ошибки", "err", err)
	}
}

// fail журналирует ошибку и отдаёт страницу 500.
func (s *Server) fail(w http.ResponseWriter, err error) {
	s.logger.Error("ошибка web-интерфейса", "err", err)
	http.Error(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
}

// needsSetup сообщает, пуста ли книга по плану счетов пользователя.
//
// Служебные счета (капитал, «Активы:Дебиторка») создаются миграцией и за
// настройку не считаются.
func (s *Server) needsSetup(ctx context.Context) (bool, error) {
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return false, err
	}
	return countUserAccounts(accounts) == 0, nil
}

// countUserAccounts считает счета плана, введённые пользователем.
func countUserAccounts(accounts []model.Account) int {
	count := 0
	for _, acc := range accounts {
		if !acc.IsSystem {
			count++
		}
	}
	return count
}

// accountPaths возвращает карту id → полный путь счёта.
func (s *Server) accountPaths(ctx context.Context) (map[int64]string, error) {
	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	paths := make(map[int64]string, len(accounts))
	for _, acc := range accounts {
		paths[acc.ID] = acc.FullPath
	}
	return paths, nil
}

// sideLabel переводит сторону проводки в кириллицу.
func sideLabel(side model.Side) string {
	switch side {
	case model.SideDebit:
		return "дебет"
	case model.SideCredit:
		return "кредит"
	default:
		return string(side)
	}
}

// statusMessage достаёт человекочитаемое сообщение об ошибке движка.
//
// Сообщение дополняется счётом и ячейкой матрицы, чтобы пользователь видел,
// какая именно ячейка ASL-2.2 не допускает выбор.
func statusMessage(err error) string {
	var ee *engine.Error
	if errors.As(err, &ee) {
		message := ee.Message
		if ee.Account != "" {
			message += " (счёт «" + ee.Account + "»)"
		}
		if ee.Cell != "" {
			message += "; ячейка матрицы " + ee.Cell
		}
		return message
	}
	return err.Error()
}

// errorView — данные страницы ошибки.
type errorView struct {
	Message string
}
