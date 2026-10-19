package api

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/trefas/asledger/pkg/core/export"
	"github.com/trefas/asledger/pkg/core/hledger"
	"github.com/trefas/asledger/pkg/core/store"
)

// formatHledger — имя формата plaintext-книги hledger в импорте и экспорте.
const formatHledger = "hledger"

// importRequest — тело запроса импорта.
type importRequest struct {
	// Format — формат содержимого: chart-json или hledger.
	Format string `json:"format"`
	// Mode — режим импорта hledger: «строгий» или «гибкий».
	Mode string `json:"mode"`
	// Content — содержимое файла.
	Content string `json:"content"`
	// FileName — имя исходного файла (для отчёта и коммита).
	FileName string `json:"file_name"`
}

// problemDTO — одна проблема импорта в ответе.
type problemDTO struct {
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
	Snippet string `json:"snippet,omitempty"`
}

// importReportDTO — отчёт об импорте в ответе.
type importReportDTO struct {
	Format          string       `json:"format"`
	Mode            string       `json:"mode,omitempty"`
	FileName        string       `json:"file_name,omitempty"`
	Accepted        int          `json:"accepted"`
	Skipped         int          `json:"skipped"`
	Duplicates      int          `json:"duplicates"`
	CreatedAccounts []string     `json:"created_accounts"`
	Problems        []problemDTO `json:"problems"`
	Rejected        bool         `json:"rejected"`
	Message         string       `json:"message,omitempty"`
}

// handleImport принимает файл импорта: план счетов в JSON или книгу hledger.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	format := req.Format
	if format == "" {
		format = formatChartJSON
	}

	switch format {
	case formatChartJSON:
		s.importChart(w, r, req.Content)
	case formatHledger:
		s.importHledger(w, r, req)
	default:
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID",
			"неизвестный формат импорта "+format)
	}
}

// importChart импортирует план счетов из вложенного JSON.
func (s *Server) importChart(w http.ResponseWriter, r *http.Request, content string) {
	chart, err := store.ParseChart([]byte(content))
	if err != nil {
		if writeEngineError(w, err) {
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "ERR_CHART_INVALID", err.Error())
		return
	}
	if err := store.ValidateChart(chart); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "ERR_CHART_INVALID", err.Error())
		return
	}
	report, err := s.db.ImportChart(r.Context(), chart)
	if err != nil {
		if writeEngineError(w, err) {
			return
		}
		s.logger.Error("не удалось импортировать план счетов", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"format":   formatChartJSON,
		"created":  report.Created,
		"existing": report.Existing,
	})
}

// importHledger импортирует книгу hledger.
//
// Проблемы данных (ошибки разбора, неизвестные счета, невалидные проводки) —
// не ошибка запроса: они возвращаются в отчёте с кодом 200, потому что импорт
// мог быть применён частично (гибкий режим). Ошибка инфраструктуры отдаётся
// конвертом 500.
func (s *Server) importHledger(w http.ResponseWriter, r *http.Request, req importRequest) {
	actor, ok := actorFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "требуется токен устройства")
		return
	}

	mode, err := hledger.ParseMode(req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
		return
	}

	importer := hledger.NewImporter(s.db)
	report, err := importer.Import(r.Context(), []byte(req.Content), hledger.Options{
		Mode: mode, ActorID: actor.UserID, FileName: req.FileName,
	})
	if err != nil {
		s.logger.Error("не удалось импортировать книгу hledger", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, importReport(report))
}

// importReport переводит отчёт импорта в DTO ответа.
func importReport(report *hledger.Report) importReportDTO {
	dto := importReportDTO{
		Format:          formatHledger,
		Mode:            string(report.Mode),
		FileName:        report.FileName,
		Accepted:        report.Accepted,
		Skipped:         report.Skipped,
		Duplicates:      report.Duplicates,
		CreatedAccounts: report.CreatedAccounts,
		Rejected:        report.Rejected,
		Message:         report.Message,
		Problems:        make([]problemDTO, 0, len(report.Problems)),
	}
	if dto.CreatedAccounts == nil {
		dto.CreatedAccounts = []string{}
	}
	for _, problem := range report.Problems {
		dto.Problems = append(dto.Problems, problemDTO{
			Line: problem.Line, Message: problem.Message, Snippet: problem.Snippet,
		})
	}
	return dto
}

// handleExport выгружает книгу в JSON, CSV или plaintext hledger.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	switch format {
	case "json":
		snapshot, err := s.sync.Snapshot(r.Context())
		if err != nil {
			s.logger.Error("не удалось собрать экспорт", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	case formatHledger:
		s.exportHledger(w, r)
	case "csv":
		s.exportCSV(w, r)
	default:
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID",
			"неизвестный формат экспорта "+format)
	}
}

// exportHledger отдаёт книгу файлом plaintext hledger.
func (s *Server) exportHledger(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := hledger.Export(r.Context(), s.db, &buf, hledger.WriteOptions{
		Comment: "asledger: выгрузка книги",
	}); err != nil {
		s.logger.Error("не удалось выгрузить книгу hledger", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeDownload(w, "asledger.journal", "text/plain; charset=utf-8", buf.Bytes())
}

// exportCSV отдаёт CSV журнала или отчёта.
//
// Параметр report выбирает отчёт: journal (по умолчанию), balances, turnover.
// Для оборотов период задаётся параметрами from и to (ГГГГ-ММ-ДД).
func (s *Server) exportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	accounts, err := s.db.ListAccounts(ctx, store.AccountFilter{IncludeArchived: true})
	if err != nil {
		s.logger.Error("не удалось прочитать план счетов", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	report := query.Get("report")
	if report == "" {
		report = "journal"
	}
	var buf bytes.Buffer
	filename := "asledger-" + report + ".csv"

	switch report {
	case "journal":
		transactions, err := s.db.ListTransactions(ctx, store.TransactionFilter{})
		if err != nil {
			s.logger.Error("не удалось прочитать проводки", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		if err := export.Journal(&buf, accounts, transactions); err != nil {
			s.csvError(w, err)
			return
		}
	case "balances":
		balances, err := s.db.Balances(ctx)
		if err != nil {
			s.logger.Error("не удалось посчитать остатки", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		if err := export.Balances(&buf, accounts, balances); err != nil {
			s.csvError(w, err)
			return
		}
	case "turnover":
		from, to, err := period(query)
		if err != nil {
			writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", err.Error())
			return
		}
		turnovers, err := s.db.Turnovers(ctx, from, to)
		if err != nil {
			s.logger.Error("не удалось посчитать обороты", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		if err := export.Turnovers(&buf, accounts, turnovers); err != nil {
			s.csvError(w, err)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID",
			"неизвестный отчёт "+report+" (ожидается journal, balances или turnover)")
		return
	}

	writeDownload(w, filename, "text/csv; charset=utf-8", buf.Bytes())
}

// csvError отвечает ошибкой выгрузки CSV.
func (s *Server) csvError(w http.ResponseWriter, err error) {
	s.logger.Error("не удалось выгрузить CSV", "err", err)
	writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
}

// period разбирает период отчёта об оборотах.
//
// Границы включают весь день: from — начало указанной даты, to — её конец.
// Пустой параметр означает отсутствие границы.
func period(query url.Values) (int64, int64, error) {
	var from, to int64
	if text := query.Get("from"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return 0, 0, errInvalidPeriod("from")
		}
		from = date.UTC().Unix()
	}
	if text := query.Get("to"); text != "" {
		date, err := time.Parse(time.DateOnly, text)
		if err != nil {
			return 0, 0, errInvalidPeriod("to")
		}
		to = date.UTC().AddDate(0, 0, 1).Unix() - 1
	}
	if from != 0 && to != 0 && from > to {
		return 0, 0, errInvalidPeriod("from")
	}
	return from, to, nil
}

// errInvalidPeriod описывает ошибку периода отчёта.
func errInvalidPeriod(name string) error {
	return &periodError{name: name}
}

// periodError — ошибка разбора периода отчёта.
type periodError struct {
	name string
}

// Error возвращает причину с подсказкой формата.
func (e *periodError) Error() string {
	return "неверная граница периода " + e.name + ": ожидается ГГГГ-ММ-ДД"
}

// writeDownload отправляет файл вложением.
func writeDownload(w http.ResponseWriter, filename, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(body))
}
