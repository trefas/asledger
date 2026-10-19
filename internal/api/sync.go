package api

import (
	"errors"
	"net/http"
	"strconv"

	syncsvc "github.com/trefas/asledger/pkg/core/sync"
)

// handleSnapshot отдаёт полный снапшот книги для клонирования.
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.sync.Snapshot(r.Context())
	if err != nil {
		s.logger.Error("не удалось собрать снапшот", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// handlePull отдаёт дельту журнала изменений с данными хэш-цепочки.
func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	since, err := intQuery(r, "since")
	if err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "параметр since должен быть целым числом")
		return
	}
	knownHash := r.URL.Query().Get("hash")

	pull, err := s.sync.Pull(r.Context(), since, knownHash)
	if err != nil {
		s.logger.Error("не удалось собрать дельту", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, pull)
}

// pushRequest — тело запроса commita операций.
type pushRequest struct {
	BaseSeq    int64               `json:"base_seq"`
	CommitID   string              `json:"commit_id"`
	Operations []syncsvc.Operation `json:"operations"`
}

// pushRejected — тело ответа на отклонённый пакет.
type pushRejected struct {
	Status    syncsvc.Status `json:"status"`
	CommitID  string         `json:"commit_id"`
	ResultSeq int64          `json:"result_seq"`
	Warnings  []string       `json:"warnings"`
	Error     apiError       `json:"error"`
}

// handlePush принимает коммит операций устройства.
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	device, _ := actorFrom(r.Context())

	var req pushRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	if req.CommitID == "" {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не задан commit_id")
		return
	}

	result, err := s.sync.Push(r.Context(), syncsvc.PushRequest{
		ClientID:   device.ClientID,
		ActorID:    device.UserID,
		BaseSeq:    req.BaseSeq,
		CommitID:   req.CommitID,
		Operations: req.Operations,
	})
	if errors.Is(err, syncsvc.ErrRejected) {
		body := pushRejected{Status: syncsvc.StatusRejected, CommitID: req.CommitID, Error: apiErrorFrom(err)}
		if result != nil {
			body.ResultSeq = result.ResultSeq
			body.Warnings = result.Warnings
		}
		writeJSON(w, http.StatusUnprocessableEntity, body)
		return
	}
	if err != nil {
		if writeEngineError(w, err) {
			return
		}
		s.logger.Error("не удалось применить пакет", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// intQuery читает целочисленный параметр запроса; отсутствие = 0.
func intQuery(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
