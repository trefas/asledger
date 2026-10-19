package api

import (
	"net/http"

	"github.com/trefas/asledger/pkg/core/engine"
	"github.com/trefas/asledger/pkg/core/model"
)

// handleSuggest подсказывает счета по частичному вводу в кириллице.
//
// Параметры: q — часть пути или имени, op_type — тип операции (для фильтра по
// матрице ASL-2.2), limit — предел выдачи. Листовые счета идут первыми и
// помечаются как пригодные; папки отдаются отдельной пометкой is_leaf=false.
func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit, err := intQuery(r, "limit")
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "параметр limit должен быть целым числом")
		return
	}

	accounts, err := s.db.SuggestAccounts(r.Context(), query, int(limit))
	if err != nil {
		s.logger.Error("не удалось подобрать счета", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	var rule engine.Rule
	hasRule := false
	if opType := r.URL.Query().Get("op_type"); opType != "" {
		if parsed, ok := engine.RuleFor(model.OpType(opType)); ok {
			rule, hasRule = parsed, true
		}
	}

	items := make([]accountDTO, 0, len(accounts))
	for _, acc := range accounts {
		usable := acc.IsLeaf
		if hasRule {
			if _, allowed := rule.Allows(acc.Type); !allowed {
				usable = false
			}
		}
		items = append(items, accountDTO{
			ID:       acc.ID,
			Name:     acc.Name,
			FullPath: acc.FullPath,
			Type:     acc.Type,
			IsLeaf:   acc.IsLeaf,
			Usable:   usable,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":              items,
		"accounts_read_only": true,
	})
}
