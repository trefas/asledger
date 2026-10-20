package store

import (
	"context"
	"errors"
	"testing"

	"github.com/trefas/asledger/pkg/core/auth"
)

// TestDevicesCRUD проверяет привязку, поиск по токену, перевыпуск и отзыв.
func TestDevicesCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	user, err := db.CreateUser(ctx, "Андрей", "секрет-андрея")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	firstHash := auth.HashToken("первый-токен")
	if err := db.UpsertDevice(ctx, "телефон-1", user.ID, firstHash); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	device, err := db.DeviceByTokenHash(ctx, firstHash)
	if err != nil {
		t.Fatalf("DeviceByTokenHash: %v", err)
	}
	if device.ClientID != "телефон-1" || device.UserID != user.ID {
		t.Fatalf("устройство %q/%d, ожидалось «телефон-1»/%d",
			device.ClientID, device.UserID, user.ID)
	}
	if device.CreatedAt.IsZero() {
		t.Error("не заполнено время подключения")
	}

	devices, err := db.ListDevices(ctx)
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].ClientID != "телефон-1" {
		t.Fatalf("список устройств: %+v", devices)
	}

	// Повторная привязка того же client_id перевыпускает токен.
	secondHash := auth.HashToken("второй-токен")
	if err := db.UpsertDevice(ctx, "телефон-1", user.ID, secondHash); err != nil {
		t.Fatalf("UpsertDevice повторно: %v", err)
	}
	if _, err := db.DeviceByTokenHash(ctx, firstHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("старый токен: %v, ожидался ErrNotFound", err)
	}
	if _, err := db.DeviceByTokenHash(ctx, secondHash); err != nil {
		t.Fatalf("новый токен: %v", err)
	}
	devices, err = db.ListDevices(ctx)
	if err != nil {
		t.Fatalf("ListDevices после перевыпуска: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("после перевыпуска устройств %d, ожидалось 1", len(devices))
	}

	if err := db.DeleteDevice(ctx, "телефон-1"); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}
	if err := db.DeleteDevice(ctx, "телефон-1"); err != nil {
		t.Fatalf("повторный DeleteDevice: %v", err)
	}
	devices, err = db.ListDevices(ctx)
	if err != nil {
		t.Fatalf("ListDevices после отзыва: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("после отзыва устройств %d, ожидалось 0", len(devices))
	}
}
