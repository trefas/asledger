// Package api реализует REST API asledger (раздел 10 эпика) на порту 8778.
//
// Все маршруты живут под /api/v1. Сервер — единственная точка записи: онлайн-
// проводки и пакеты синхронизации проходят через движок и матрицу правил.
// Ошибки отдаются единым JSON-конвертом с машинным кодом валидации.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/engine"
	reconcilesvc "github.com/trefas/asledger/pkg/core/reconcile"
	"github.com/trefas/asledger/pkg/core/store"
	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// Config — параметры API-сервера.
type Config struct {
	// Version — версия сборки для /health.
	Version string
	// ChartsDir — каталог шаблонных планов счетов.
	ChartsDir string
	// Logger — журнал запросов; при nil используется no-op журнал.
	Logger *slog.Logger
}

// Server — HTTP-обработчики REST API.
type Server struct {
	db        *store.DB
	sync      *syncsvc.Service
	reconcile *reconcilesvc.Service
	cfg       Config
	logger    *slog.Logger
}

// New создаёт API-сервер поверх книги.
func New(db *store.DB, cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Server{
		db:        db,
		sync:      syncsvc.New(db),
		reconcile: reconcilesvc.New(db),
		cfg:       cfg,
		logger:    logger,
	}
}

// Handler возвращает маршрутизатор API со всеми middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("POST /api/v1/auth/pair", s.handlePair)
	mux.HandleFunc("POST /api/v1/auth/enroll", s.handleEnroll)
	mux.Handle("DELETE /api/v1/auth/token", s.auth(http.HandlerFunc(s.handleRevokeToken)))

	mux.Handle("POST /api/v1/transactions", s.auth(http.HandlerFunc(s.handleTransactions)))
	mux.Handle("POST /api/v1/transactions/{uuid}/reverse", s.auth(http.HandlerFunc(s.handleReverse)))

	mux.Handle("GET /api/v1/sync/snapshot", s.auth(http.HandlerFunc(s.handleSnapshot)))
	mux.Handle("POST /api/v1/sync/push", s.auth(http.HandlerFunc(s.handlePush)))
	mux.Handle("GET /api/v1/sync/pull", s.auth(http.HandlerFunc(s.handlePull)))

	mux.Handle("GET /api/v1/accounts/suggest", s.auth(http.HandlerFunc(s.handleSuggest)))
	mux.Handle("GET /api/v1/engine/roles", s.auth(http.HandlerFunc(s.handleEngineRoles)))
	mux.Handle("GET /api/v1/charts/templates", s.auth(http.HandlerFunc(s.handleChartTemplates)))
	mux.Handle("POST /api/v1/import", s.auth(http.HandlerFunc(s.handleImport)))
	mux.Handle("GET /api/v1/export", s.auth(http.HandlerFunc(s.handleExport)))

	mux.Handle("GET /api/v1/reports/balances", s.auth(http.HandlerFunc(s.handleReportsBalances)))
	mux.Handle("GET /api/v1/reports/turnovers", s.auth(http.HandlerFunc(s.handleReportsTurnovers)))
	mux.Handle("GET /api/v1/reports/expenses", s.auth(http.HandlerFunc(s.handleReportsExpenses)))
	mux.Handle("GET /api/v1/reports/journal", s.auth(http.HandlerFunc(s.handleReportsJournal)))
	mux.Handle("GET /api/v1/reports/discrepancies", s.auth(http.HandlerFunc(s.handleReportsDiscrepancies)))
	mux.Handle("POST /api/v1/reconcile", s.auth(http.HandlerFunc(s.handleReconcile)))

	var handler http.Handler = mux
	handler = s.jsonFallback(handler)
	handler = s.requestID(handler)
	handler = s.logRequests(handler)
	handler = s.recoverPanic(handler)
	return handler
}

// handleHealth отвечает состоянием сервиса.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        s.cfg.Version,
		"format_version": syncsvc.SnapshotFormatVersion,
	})
}

// --- middleware ---

type contextKey int

const (
	ctxRequestID contextKey = iota
)

// requestID добавляет идентификатор запроса в контекст и заголовок ответа.
func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(withRequestID(r.Context(), id)))
	})
}

// logRequests пишет строку журнала о каждом запросе.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("api",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestID(r.Context()),
		)
	})
}

// recoverPanic не даёт панике обрушить сервер и возвращает JSON 500.
func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("паника при обработке запроса", "err", rec, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// jsonFallback превращает стандартные текстовые 404/405 в JSON-конверт.
func (s *Server) jsonFallback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&fallbackWriter{ResponseWriter: w}, r)
	})
}

// fallbackWriter подменяет тело стандартных ответов ServeMux на JSON.
type fallbackWriter struct {
	http.ResponseWriter
	intercept bool
	wrote     bool
}

// WriteHeader перехватывает 404 и 405.
func (w *fallbackWriter) WriteHeader(code int) {
	if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
		w.intercept = true
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write подменяет текст стандартной ошибки на JSON при перехвате.
func (w *fallbackWriter) Write(b []byte) (int, error) {
	if w.intercept && !w.wrote {
		w.wrote = true
		code, message := "ERR_NOT_FOUND", "маршрут не найден"
		payload, _ := json.Marshal(errorEnvelope{Error: apiError{Code: code, Message: message}})
		return w.ResponseWriter.Write(append(payload, '\n'))
	}
	return w.ResponseWriter.Write(b)
}

// statusRecorder запоминает код ответа для журнала.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader запоминает код ответа.
func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// newRequestID возвращает случайный идентификатор запроса.
func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "req"
	}
	return hex.EncodeToString(buf)
}

// --- JSON ---

// writeJSON отправляет значение в формате JSON с указанным кодом ответа.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON читает тело запроса; пустое тело ошибкой не считается.
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

// maxBodyBytes — предельный размер тела запроса.
const maxBodyBytes = 16 << 20

// bearerToken извлекает токен из заголовка Authorization.
//
// Принимаются и схема «Bearer <токен>», и голое значение токена.
func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return header
}

// engineCode достаёт код валидации из ошибки движка или слоя данных.
func engineCode(err error) (string, bool) {
	var ee *engine.Error
	if errors.As(err, &ee) {
		return ee.Code, true
	}
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		return ve.Code, true
	}
	return "", false
}
