package httpui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/auth"
	"github.com/trefas/asledger/pkg/core/store"
)

// TestASL_11_WebDevices проверяет страницу устройств: список, режим привязки
// и отзыв токена.
func TestASL_11_WebDevices(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)
	uid := w.userID(t)
	ctx := context.Background()

	if err := w.db.UpsertDevice(ctx, "телефон-1", uid, auth.HashToken("токен-1")); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	rec := w.get(t, "/devices")
	if rec.Code != http.StatusOK {
		t.Fatalf("страница устройств: код %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"телефон-1", "Андрей", "Разрешена", "Привязка новых устройств"} {
		if !strings.Contains(body, want) {
			t.Errorf("страница устройств: нет %q", want)
		}
	}

	rec = w.postForm(t, "/devices/enrollment", url.Values{"enrollment": {"deny"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("запрет привязки: код %d", rec.Code)
	}
	if message := flashOf(t, rec); !strings.Contains(message, "запрещена") {
		t.Errorf("flash: %q", message)
	}
	mode, ok, err := w.db.Setting(ctx, store.SettingDevicesEnrollment)
	if err != nil || !ok || mode != store.EnrollmentDeny {
		t.Fatalf("настройка привязки: mode=%q ok=%v err=%v", mode, ok, err)
	}

	rec = w.get(t, "/devices")
	if !strings.Contains(rec.Body.String(), "Запрещена") {
		t.Error("страница не показывает запрет привязки")
	}

	rec = w.postForm(t, "/devices/enrollment", url.Values{"enrollment": {"allow"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("разрешение привязки: код %d", rec.Code)
	}

	rec = w.postForm(t, "/devices/revoke", url.Values{"client_id": {"телефон-1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("отзыв устройства: код %d", rec.Code)
	}
	devices, err := w.db.ListDevices(ctx)
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("после отзыва устройств %d, ожидалось 0", len(devices))
	}

	// Пустой client_id — понятная ошибка, а не молчаливый редирект.
	rec = w.postForm(t, "/devices/revoke", url.Values{"client_id": {""}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("пустой client_id: код %d", rec.Code)
	}
	if message := flashOf(t, rec); !strings.Contains(message, "не удалось определить устройство") {
		t.Errorf("flash пустого client_id: %q", message)
	}
}

// TestASL_11_WebUsersDevicesLink проверяет ссылку со страницы пользователей на
// страницу устройств.
func TestASL_11_WebUsersDevicesLink(t *testing.T) {
	w := newTestWeb(t)
	w.seedUserChart(t)

	rec := w.get(t, "/users")
	if rec.Code != http.StatusOK {
		t.Fatalf("страница пользователей: код %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `href="/devices"`) {
		t.Error("нет ссылки на страницу устройств")
	}
}
