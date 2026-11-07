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

	accrualsvc "github.com/trefas/asledger/pkg/core/accrual"
	creditsvc "github.com/trefas/asledger/pkg/core/credit"
	depositsvc "github.com/trefas/asledger/pkg/core/deposit"
	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/money"
	reconcilesvc "github.com/trefas/asledger/pkg/core/reconcile"
	"github.com/trefas/asledger/pkg/core/store"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
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
	cfg     Config
	db      *store.DB
	engine  *engine.Engine
	credit  *creditsvc.Service
	deposit *depositsvc.Service
	accrual *accrualsvc.Service
	sync    *syncsvc.Service
	recon   *reconcilesvc.Service
	logger  *slog.Logger
	pages   map[string]*template.Template
	static  http.Handler
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
	"journal":            "journal.html",
	"entry":              "entry.html",
	"setup":              "setup.html",
	"data":               "data.html",
	"chart":              "chart.html",
	"users":              "users.html",
	"devices":            "devices.html",
	"initial":            "initial.html",
	"reports":            "reports.html",
	"reconcile":          "reconcile.html",
	"shopping":           "shopping.html",
	"shopping_detail":    "shopping_detail.html",
	"shopping_shop":      "shopping_shop.html",
	"shopping_checkout":  "shopping_checkout.html",
	"shopping_templates": "shopping_templates.html",
	"shopping_template":  "shopping_template.html",
	"credits":            "credits.html",
	"deposits":           "deposits.html",
	"accruals":           "accruals.html",
	"products":           "products.html",
	"product":            "product.html",
	"error":              "error.html",
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
		sync:   syncsvc.New(cfg.DB),
		recon:  reconcilesvc.New(cfg.DB),
		logger: logger,
		pages:  make(map[string]*template.Template),
	}
	s.credit = creditsvc.NewWithEngine(cfg.DB, s.engine)
	s.deposit = depositsvc.NewWithEngine(cfg.DB, s.engine)
	s.accrual = accrualsvc.NewWithEngine(cfg.DB, s.engine)
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
		"money":        func(k money.Kop) string { return k.String() },
		"minbalance":   minBalanceLabel,
		"date":         func(t time.Time) string { return t.Format("02.01.2006") },
		"side":         sideLabel,
		"indent":       indent,
		"rate":         rateLabel,
		"kind":         kindLabel,
		"status":       statusLabel,
		"interesttype": interestTypeLabel,
		"repayment":    repaymentOrderLabel,
		"bonus":        bonusTypeLabel,
		"frequency":    accrualFrequencyLabel,
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

	mux.HandleFunc("GET /chart", s.handleChart)
	mux.HandleFunc("POST /chart/create", s.handleChartCreate)
	mux.HandleFunc("POST /chart/rename", s.handleChartRename)
	mux.HandleFunc("POST /chart/move", s.handleChartMove)
	mux.HandleFunc("POST /chart/archive", s.handleChartArchive)
	mux.HandleFunc("POST /chart/min-balance", s.handleChartMinBalance)
	mux.HandleFunc("POST /chart/mode", s.handleChartMode)

	mux.HandleFunc("GET /users", s.handleUsers)
	mux.HandleFunc("POST /users/create", s.handleUsersCreate)
	mux.HandleFunc("POST /users/rename", s.handleUsersRename)
	mux.HandleFunc("POST /users/token", s.handleUserToken)
	mux.HandleFunc("POST /users/revoke", s.handleUserRevoke)

	mux.HandleFunc("GET /devices", s.handleDevices)
	mux.HandleFunc("POST /devices/enrollment", s.handleDevicesEnrollment)
	mux.HandleFunc("POST /devices/revoke", s.handleDeviceRevoke)

	mux.HandleFunc("GET /initial", s.handleInitialForm)
	mux.HandleFunc("POST /initial", s.handleInitialSubmit)

	mux.HandleFunc("GET /data", s.handleData)
	mux.HandleFunc("POST /data/import", s.handleDataImport)
	mux.HandleFunc("GET /data/export/{format}", s.handleDataExport)

	mux.HandleFunc("GET /reports", s.handleReports)
	mux.HandleFunc("GET /reconcile", s.handleReconcileForm)
	mux.HandleFunc("POST /reconcile", s.handleReconcileSubmit)
	mux.HandleFunc("POST /journal/reverse", s.handleJournalReverse)

	mux.HandleFunc("GET /credits", s.handleCredits)
	mux.HandleFunc("POST /credits/preview", s.handleCreditPreview)
	mux.HandleFunc("POST /credits/post", s.handleCreditPost)

	mux.HandleFunc("GET /deposits", s.handleDeposits)
	mux.HandleFunc("POST /deposits/preview", s.handleDepositPreview)
	mux.HandleFunc("POST /deposits/post", s.handleDepositPost)

	mux.HandleFunc("GET /accruals", s.handleAccruals)
	mux.HandleFunc("POST /accruals/preview", s.handleAccrualPreview)
	mux.HandleFunc("POST /accruals/apply", s.handleAccrualApply)

	mux.HandleFunc("GET /products", s.handleProducts)
	mux.HandleFunc("GET /products/{id}", s.handleProductForm)
	mux.HandleFunc("POST /products/{id}", s.handleProductSave)

	mux.HandleFunc("GET /shopping-lists", s.handleShoppingLists)
	mux.HandleFunc("POST /shopping-lists", s.handleShoppingListCreate)
	mux.HandleFunc("POST /shopping-lists/from-template", s.handleShoppingListFromTemplate)
	mux.HandleFunc("GET /shopping-templates", s.handleShoppingTemplates)
	mux.HandleFunc("POST /shopping-templates", s.handleShoppingTemplateCreate)
	mux.HandleFunc("GET /shopping-templates/{id}", s.handleShoppingTemplateDetail)
	mux.HandleFunc("POST /shopping-templates/{id}", s.handleShoppingTemplateUpdate)
	mux.HandleFunc("POST /shopping-templates/{id}/delete", s.handleShoppingTemplateDelete)
	mux.HandleFunc("GET /shopping-lists/{id}", s.handleShoppingListDetail)
	mux.HandleFunc("POST /shopping-lists/{id}/items", s.handleShoppingItemCreate)
	mux.HandleFunc("GET /shopping-lists/{id}/shop", s.handleShoppingShop)
	mux.HandleFunc("POST /shopping-lists/{id}/shop/{item}", s.handleShoppingShopUpdate)
	mux.HandleFunc("GET /shopping-lists/{id}/checkout", s.handleShoppingCheckout)
	mux.HandleFunc("POST /shopping-lists/{id}/checkout", s.handleShoppingCheckoutPreview)
	mux.HandleFunc("POST /shopping-lists/{id}/checkout/post", s.handleShoppingCheckoutPost)
	mux.HandleFunc("POST /shopping-lists/{id}/status", s.handleShoppingListStatus)
	mux.HandleFunc("POST /shopping-lists/{id}/archive", s.handleShoppingListArchive)
	mux.HandleFunc("POST /shopping-lists/{id}/save-template", s.handleShoppingListSaveTemplate)
	mux.HandleFunc("POST /shopping-items/{id}", s.handleShoppingItemUpdate)
	mux.HandleFunc("POST /shopping-items/{id}/delete", s.handleShoppingItemDelete)
	mux.HandleFunc("GET /shopping/suggest.json", s.handleShoppingSuggestJSON)

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

// renderPage отрисовывает страницу из готовых данных.
//
// Используется там, где ошибку нужно показать в самой форме, а не отдельной
// страницей: например, конфликт версий в режиме магазина.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	t, ok := s.pages[name]
	if !ok {
		s.fail(w, fmt.Errorf("httpui: нет шаблона страницы %q", name))
		return
	}
	page := pageData{Title: title, Active: "shopping", Version: s.cfg.Version, Data: data}
	if needsSetup, err := s.needsSetup(r.Context()); err == nil {
		page.Setup = needsSetup
		page.HasChart = !needsSetup
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", page); err != nil {
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
	// Ошибки валидации слоя данных и сервисов несут готовое сообщение без кода.
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		return ve.Message
	}
	return err.Error()
}

// errorView — данные страницы ошибки.
type errorView struct {
	Message string
}
