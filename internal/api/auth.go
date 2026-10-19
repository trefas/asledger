package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/trefas/asledger/pkg/core/auth"
	"github.com/trefas/asledger/pkg/core/store"
)

// pairRequest — запрос привязки устройства.
type pairRequest struct {
	// Name — имя пользователя.
	Name string `json:"name"`
	// Secret — секрет пользователя (пароль).
	Secret string `json:"secret"`
	// ClientID — идентификатор устройства.
	ClientID string `json:"client_id"`
}

// pairResponse — ответ привязки устройства с токеном.
type pairResponse struct {
	// Token — открытый токен: возвращается один раз, в книге хранится хеш.
	Token string `json:"token"`
	// ClientID — идентификатор устройства.
	ClientID string `json:"client_id"`
	// UserID — идентификатор пользователя.
	UserID int64 `json:"user_id"`
	// UserName — имя пользователя.
	UserName string `json:"user_name"`
}

// handlePair привязывает устройство и выдаёт персональный токен.
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.ClientID) == "" {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "нужны поля name и client_id")
		return
	}

	user, err := s.db.Authenticate(r.Context(), req.Name, req.Secret)
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrAuthFailed):
		writeError(w, http.StatusUnauthorized, "ERR_AUTH_FAILED", "неверное имя пользователя или секрет")
		return
	case err != nil:
		s.logger.Error("ошибка аутентификации", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		s.logger.Error("не удалось создать токен", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	if err := s.db.UpsertDevice(r.Context(), req.ClientID, user.ID, auth.HashToken(token)); err != nil {
		s.logger.Error("не удалось привязать устройство", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	s.logger.Info("устройство привязано",
		"client_id", req.ClientID, "user", user.Name, "request_id", RequestID(r.Context()))
	writeJSON(w, http.StatusOK, pairResponse{
		Token:    token,
		ClientID: req.ClientID,
		UserID:   user.ID,
		UserName: user.Name,
	})
}

// handleRevokeToken отзывает токен текущего устройства.
func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	device, ok := actorFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "требуется токен устройства")
		return
	}
	if err := s.db.DeleteDevice(r.Context(), device.ClientID); err != nil {
		s.logger.Error("не удалось отозвать устройство", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	s.logger.Info("токен отозван", "client_id", device.ClientID, "request_id", RequestID(r.Context()))
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "client_id": device.ClientID})
}

// auth — middleware проверки персонального токена устройства.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "требуется токен устройства")
			return
		}
		device, err := s.db.DeviceByTokenHash(r.Context(), auth.HashToken(token))
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "ERR_UNAUTHORIZED", "неизвестный или отозванный токен")
			return
		}
		if err != nil {
			s.logger.Error("не удалось проверить токен", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		user, err := s.db.User(r.Context(), device.UserID)
		if err != nil {
			s.logger.Error("не удалось прочитать пользователя устройства", "err", err)
			writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
			return
		}
		next.ServeHTTP(w, r.WithContext(withActor(r.Context(), Device{
			ClientID: device.ClientID,
			UserID:   user.ID,
			UserName: user.Name,
		})))
	})
}
