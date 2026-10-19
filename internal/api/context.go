package api

import "context"

// contextKeyActor — ключ актора в контексте запроса.
type contextKeyActor struct{}

// Device — устройство, привязанное токеном (в контексте запроса).
type Device struct {
	// ClientID — идентификатор устройства (клиента синхронизации).
	ClientID string
	// UserID — пользователь, которому принадлежит устройство.
	UserID int64
	// UserName — имя пользователя.
	UserName string
}

// withRequestID кладёт идентификатор запроса в контекст.
func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxRequestID, id)
}

// RequestID возвращает идентификатор запроса.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxRequestID).(string); ok {
		return id
	}
	return ""
}

// withActor кладёт устройство в контекст запроса.
func withActor(ctx context.Context, device Device) context.Context {
	return context.WithValue(ctx, contextKeyActor{}, device)
}

// actorFrom возвращает устройство из контекста.
func actorFrom(ctx context.Context) (Device, bool) {
	device, ok := ctx.Value(contextKeyActor{}).(Device)
	return device, ok
}
