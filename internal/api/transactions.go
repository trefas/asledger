package api

import (
	"errors"
	"net/http"

	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// handleTransactions принимает онлайн-проводку и прогоняет её через движок.
//
// Стороны строк назначает сервер: клиент присылает только роли. Проводка
// оформляется коммитом из одной операции, поэтому попадает в хэш-цепочку.
// Повтор с тем же uuid идемпотентен и возвращает прежнюю проводку.
func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req syncsvc.Operation
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	if req.UUID == "" {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не задан uuid проводки")
		return
	}

	head, err := s.db.HeadSeq(r.Context())
	if err != nil {
		s.logger.Error("не удалось прочитать head_seq", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	_, err = s.sync.Push(r.Context(), syncsvc.PushRequest{
		ClientID:   device.ClientID,
		ActorID:    device.UserID,
		BaseSeq:    head,
		CommitID:   "online:" + req.UUID,
		Operations: []syncsvc.Operation{req},
	})
	if errors.Is(err, syncsvc.ErrRejected) {
		if writeEngineError(w, err) {
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "ERR_REQUEST_INVALID", "операция отклонена")
		return
	}
	if err != nil {
		s.logger.Error("не удалось сохранить проводку", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	tr, err := s.db.TransactionByUUID(r.Context(), req.UUID)
	if err != nil {
		s.logger.Error("не удалось прочитать проводку", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeJSON(w, http.StatusCreated, toTransactionDTO(tr))
}
