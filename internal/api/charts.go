package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/trefas/asledger/pkg/core/store"
)

// formatChartJSON — имя формата импорта вложенного JSON-плана счетов.
const formatChartJSON = "chart-json"

// templateDTO — шаблонный план счетов в ответе.
type templateDTO struct {
	File     string `json:"file"`
	Name     string `json:"name"`
	Accounts int    `json:"accounts"`
}

// handleChartTemplates отдаёт список шаблонных планов счетов из каталога.
func (s *Server) handleChartTemplates(w http.ResponseWriter, _ *http.Request) {
	files, err := filepath.Glob(filepath.Join(s.cfg.ChartsDir, "*.json"))
	if err != nil {
		s.logger.Error("не удалось прочитать каталог шаблонов", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	sort.Strings(files)

	templates := make([]templateDTO, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		chart, err := store.ParseChart(data)
		if err != nil {
			continue
		}
		name := chart.Name
		if name == "" {
			name = filepath.Base(file)
		}
		templates = append(templates, templateDTO{
			File:     filepath.Base(file),
			Name:     name,
			Accounts: countChartNodes(chart),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

// countChartNodes считает вершины плана счетов.
func countChartNodes(chart *store.Chart) int {
	total := 0
	var walk func(nodes []store.ChartNode)
	walk = func(nodes []store.ChartNode) {
		for _, node := range nodes {
			total++
			walk(node.Children)
		}
	}
	walk(chart.Accounts)
	return total
}

// importRequest — тело запроса импорта.
type importRequest struct {
	// Format — формат содержимого: chart-json (hledger появится в вехе S5).
	Format string `json:"format"`
	// Mode — режим импорта: «строгий» или «гибкий» (для hledger).
	Mode string `json:"mode"`
	// Content — содержимое файла.
	Content string `json:"content"`
}

// handleImport принимает файл импорта.
//
// Каркас вехи S3: план счетов из JSON импортируется через слой данных;
// форматы hledger появятся в вехе S5 и пока отвечают 501.
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
		chart, err := store.ParseChart([]byte(req.Content))
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
			"format":   format,
			"created":  report.Created,
			"existing": report.Existing,
		})
	case "hledger":
		writeError(w, http.StatusNotImplemented, "ERR_NOT_IMPLEMENTED",
			"импорт hledger будет реализован в вехе S5")
	default:
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID",
			"неизвестный формат импорта "+format)
	}
}

// handleExport выгружает книгу.
//
// Каркас вехи S3: JSON-снапшот книги отдаётся сразу; CSV и hledger появятся
// в вехе S5 и пока отвечают 501.
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
	case "csv", "hledger":
		writeError(w, http.StatusNotImplemented, "ERR_NOT_IMPLEMENTED",
			"экспорт в формате "+format+" будет реализован в вехе S5")
	default:
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID",
			"неизвестный формат экспорта "+format)
	}
}
