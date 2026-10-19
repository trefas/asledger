package httpui

import (
	"context"
	"net/http"
	"strings"

	"github.com/trefas/asledger/pkg/core/auth"
	"github.com/trefas/asledger/pkg/core/model"
)

// usersView — данные страницы пользователей семьи.
type usersView struct {
	Users []userView
	// Token — только что выпущенный токен устройства (показывается один раз).
	Token string
	// TokenOwner — пользователь, которому выпущен токен.
	TokenOwner string
	// TokenClient — устройство, которому выпущен токен.
	TokenClient string
}

// userView — пользователь и его устройства.
type userView struct {
	User    model.User
	Devices []model.Device
}

// handleUsers показывает пользователей и устройства.
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	view, err := s.usersView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "users", pageData{Title: "Пользователи", Active: "users", Data: view})
}

// usersView собирает пользователей вместе с их устройствами.
func (s *Server) usersView(ctx context.Context) (usersView, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return usersView{}, err
	}
	devices, err := s.db.ListDevices(ctx)
	if err != nil {
		return usersView{}, err
	}

	byUser := make(map[int64][]model.Device, len(users))
	for _, device := range devices {
		byUser[device.UserID] = append(byUser[device.UserID], device)
	}

	view := usersView{Users: make([]userView, 0, len(users))}
	for _, user := range users {
		view.Users = append(view.Users, userView{User: user, Devices: byUser[user.ID]})
	}
	return view, nil
}

// handleUsersCreate создаёт пользователя семьи.
func (s *Server) handleUsersCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	secret := r.FormValue("secret")
	if name == "" || secret == "" {
		s.mutationError(w, r, "/users", "укажите имя и секрет пользователя")
		return
	}
	if _, err := s.db.CreateUser(r.Context(), name, secret); err != nil {
		s.mutationError(w, r, "/users", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/users", "Пользователь «"+name+"» создан")
}

// handleUsersRename переименовывает пользователя.
func (s *Server) handleUsersRename(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	id, ok := s.formID(w, r, "user_id", "/users")
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if err := s.db.RenameUser(r.Context(), id, name); err != nil {
		s.mutationError(w, r, "/users", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/users", "Пользователь переименован: «"+name+"»")
}

// handleUserToken выпускает токен устройства и показывает его один раз.
//
// В книге сохраняется только SHA-256 токена; открытое значение возвращается в
// ответе на выпуск и больше нигде не хранится.
func (s *Server) handleUserToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	userID, ok := s.formID(w, r, "user_id", "/users")
	if !ok {
		return
	}
	clientID := strings.TrimSpace(r.FormValue("client_id"))
	if clientID == "" {
		s.mutationError(w, r, "/users", "укажите имя устройства")
		return
	}

	user, err := s.db.User(ctx, userID)
	if err != nil {
		s.mutationError(w, r, "/users", "пользователь не найден")
		return
	}
	token, err := auth.NewToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.db.UpsertDevice(ctx, clientID, userID, auth.HashToken(token)); err != nil {
		s.mutationError(w, r, "/users", statusMessage(err))
		return
	}

	view, err := s.usersView(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	view.Token = token
	view.TokenOwner = user.Name
	view.TokenClient = clientID
	s.render(w, r, "users", pageData{Title: "Пользователи", Active: "users", Data: view})
}

// handleUserRevoke отзывает токен устройства.
func (s *Server) handleUserRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	clientID := strings.TrimSpace(r.FormValue("client_id"))
	if clientID == "" {
		s.mutationError(w, r, "/users", "не удалось определить устройство")
		return
	}
	if err := s.db.DeleteDevice(r.Context(), clientID); err != nil {
		s.mutationError(w, r, "/users", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/users", "Токен устройства «"+clientID+"» отозван")
}
