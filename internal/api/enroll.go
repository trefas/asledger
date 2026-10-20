package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/trefas/asledger/pkg/core/auth"
	"github.com/trefas/asledger/pkg/core/model"
	"github.com/trefas/asledger/pkg/core/store"
)

// enrollRequest — запрос привязки устройства без пароля.
type enrollRequest struct {
	// ClientID — постоянный идентификатор устройства.
	ClientID string `json:"client_id"`
	// DeviceName — человекочитаемое имя устройства (для журнала).
	DeviceName string `json:"device_name"`
	// UserID — выбранный пользователь; пусто при единственном пользователе.
	UserID int64 `json:"user_id,omitempty"`
}

// enrollUserDTO — пользователь в ответе выбора при привязке.
type enrollUserDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// enrollResponse — успешная привязка устройства.
type enrollResponse struct {
	Token    string `json:"token"`
	ClientID string `json:"client_id"`
	UserID   int64  `json:"user_id"`
	UserName string `json:"user_name"`
}

// enrollChoiceResponse — ответ, когда в книге несколько пользователей.
type enrollChoiceResponse struct {
	Error apiError        `json:"error"`
	Users []enrollUserDTO `json:"users"`
}

// handleEnroll привязывает устройство без пароля.
//
// Требование Android-клиента: на первом экране только адрес и порт. Поэтому
// привязка разрешена из локальной сети, а при нескольких пользователях клиент
// выбирает имя из списка. Настройка devices.enrollment может запретить привязку.
func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "не удалось разобрать запрос")
		return
	}
	clientID := strings.TrimSpace(req.ClientID)
	if clientID == "" {
		writeError(w, http.StatusBadRequest, "ERR_REQUEST_INVALID", "нужно поле client_id")
		return
	}
	if !localRequest(r) {
		writeError(w, http.StatusForbidden, "ERR_ENROLL_FORBIDDEN",
			"привязка устройства разрешена только в локальной сети")
		return
	}

	mode, ok, err := s.db.Setting(r.Context(), store.SettingDevicesEnrollment)
	if err != nil {
		s.logger.Error("не удалось прочитать настройку привязки", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	if ok && mode == store.EnrollmentDeny {
		writeError(w, http.StatusForbidden, "ERR_ENROLL_DISABLED",
			"привязка новых устройств запрещена в настройках книги")
		return
	}

	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		s.logger.Error("не удалось прочитать пользователей", "err", err, "request_id", RequestID(r.Context()))
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	if len(users) == 0 {
		writeError(w, http.StatusConflict, "ERR_ENROLL_NO_USERS",
			"в книге нет пользователей: создайте пользователя в web-интерфейсе")
		return
	}

	user, reason := selectEnrollUser(users, req.UserID)
	switch reason {
	case "unknown":
		writeError(w, http.StatusUnprocessableEntity, "ERR_ENROLL_USER_UNKNOWN",
			"выбранный пользователь не найден в книге")
		return
	case "required":
		writeJSON(w, http.StatusConflict, enrollChoiceResponse{
			Error: apiError{
				Code:    "ERR_ENROLL_USER_REQUIRED",
				Message: "выберите пользователя, от имени которого вести учёт",
			},
			Users: enrollUsers(users),
		})
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		s.logger.Error("не удалось создать токен", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}
	if err := s.db.UpsertDevice(r.Context(), clientID, user.ID, auth.HashToken(token)); err != nil {
		s.logger.Error("не удалось привязать устройство", "err", err)
		writeError(w, http.StatusInternalServerError, "ERR_INTERNAL", "внутренняя ошибка сервера")
		return
	}

	s.logger.Info("устройство привязано без пароля",
		"client_id", clientID, "device", strings.TrimSpace(req.DeviceName),
		"user", user.Name, "remote_addr", r.RemoteAddr,
		"request_id", RequestID(r.Context()))
	writeJSON(w, http.StatusOK, enrollResponse{
		Token:    token,
		ClientID: clientID,
		UserID:   user.ID,
		UserName: user.Name,
	})
}

// selectEnrollUser выбирает пользователя для привязки.
//
// Без user_id при единственном пользователе выбор однозначен; при нескольких
// возвращается reason="required", и клиент показывает список. Неизвестный
// user_id даёт reason="unknown".
func selectEnrollUser(users []model.User, userID int64) (*model.User, string) {
	if userID != 0 {
		for i := range users {
			if users[i].ID == userID {
				return &users[i], ""
			}
		}
		return nil, "unknown"
	}
	if len(users) == 1 {
		return &users[0], ""
	}
	return nil, "required"
}

// enrollUsers отдаёт список пользователей без секретов.
func enrollUsers(users []model.User) []enrollUserDTO {
	out := make([]enrollUserDTO, 0, len(users))
	for _, u := range users {
		out = append(out, enrollUserDTO{ID: u.ID, Name: u.Name})
	}
	return out
}

// localRequest сообщает, пришёл ли запрос из локальной сети.
//
// Проверяется только RemoteAddr: заголовкам X-Forwarded-For доверять нельзя.
// За обратным прокси на localhost привязка разрешена, что ожидаемо для
// домашней установки.
func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}
