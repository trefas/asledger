package httpui

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/trefas/asledger/pkg/core/store"
)

// devicesView — данные страницы устройств.
type devicesView struct {
	// Devices — подключённые устройства с именами пользователей.
	Devices []deviceRow
	// Enrollment — привязка новых устройств разрешена.
	Enrollment bool
	// EnrollmentExplicit — настройка задана явно (иначе действует allow).
	EnrollmentExplicit bool
}

// deviceRow — устройство в таблице страницы.
type deviceRow struct {
	ClientID  string
	UserName  string
	CreatedAt time.Time
}

// handleDevices показывает устройства книги и управление привязкой.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	view, err := s.devicesView(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, r, "devices", pageData{Title: "Устройства", Active: "devices", Data: view})
}

// devicesView собирает устройства и настройку привязки.
func (s *Server) devicesView(ctx context.Context) (devicesView, error) {
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return devicesView{}, err
	}
	names := make(map[int64]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Name
	}
	devices, err := s.db.ListDevices(ctx)
	if err != nil {
		return devicesView{}, err
	}

	view := devicesView{Devices: make([]deviceRow, 0, len(devices)), Enrollment: true}
	for _, d := range devices {
		name := names[d.UserID]
		if name == "" {
			name = "—"
		}
		view.Devices = append(view.Devices, deviceRow{
			ClientID:  d.ClientID,
			UserName:  name,
			CreatedAt: d.CreatedAt,
		})
	}

	mode, ok, err := s.db.Setting(ctx, store.SettingDevicesEnrollment)
	if err != nil {
		return devicesView{}, err
	}
	if ok {
		view.EnrollmentExplicit = true
		view.Enrollment = mode != store.EnrollmentDeny
	}
	return view, nil
}

// handleDevicesEnrollment включает или выключает привязку новых устройств.
func (s *Server) handleDevicesEnrollment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	allow := r.FormValue("enrollment") == "allow"
	value := store.EnrollmentDeny
	message := "Привязка новых устройств запрещена"
	if allow {
		value = store.EnrollmentAllow
		message = "Привязка новых устройств разрешена"
	}
	if err := s.db.SetSetting(r.Context(), store.SettingDevicesEnrollment, value); err != nil {
		s.mutationError(w, r, "/devices", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/devices", message)
}

// handleDeviceRevoke отзывает токен устройства со страницы устройств.
func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, err)
		return
	}
	clientID := strings.TrimSpace(r.FormValue("client_id"))
	if clientID == "" {
		s.mutationError(w, r, "/devices", "не удалось определить устройство")
		return
	}
	if err := s.db.DeleteDevice(r.Context(), clientID); err != nil {
		s.mutationError(w, r, "/devices", statusMessage(err))
		return
	}
	s.redirectOK(w, r, "/devices", "Токен устройства «"+clientID+"» отозван")
}
