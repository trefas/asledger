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
