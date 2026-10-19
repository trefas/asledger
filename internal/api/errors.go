package api

import (
	"errors"
	"net/http"

	"github.com/trefas/asledger/pkg/core/engine"
)

// apiError — тело ошибки в едином JSON-конверте.
type apiError struct {
	// Code — машинный код (см. docs/validation-codes.md).
	Code string `json:"code"`
	// Message — понятное пользователю сообщение.
	Message string `json:"message"`
	// Account — полный путь счёта, если ошибка связана со счётом.
	Account string `json:"account,omitempty"`
	// Cell — ячейка матрицы «операция/сторона/тип счёта».
	Cell string `json:"cell,omitempty"`
	// AmountKop — сумма в копейках, если ошибка связана с суммой.
	AmountKop *int64 `json:"amount_kop,omitempty"`
}

// errorEnvelope — конверт ошибки API.
type errorEnvelope struct {
	Error apiError `json:"error"`
}

// writeError отправляет ошибку в едином формате.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message}})
}

// writeEngineError переводит ошибку движка или слоя данных в JSON-ответ.
//
// Возвращает true, если ошибка распознана и ответ отправлен.
func writeEngineError(w http.ResponseWriter, err error) bool {
	var ee *engine.Error
	if errors.As(err, &ee) {
		payload := apiError{Code: ee.Code, Message: ee.Message, Account: ee.Account, Cell: ee.Cell}
		if ee.Amount != 0 {
			kop := ee.Amount.Kopecks()
			payload.AmountKop = &kop
		}
		writeJSON(w, http.StatusUnprocessableEntity, errorEnvelope{Error: payload})
		return true
	}
	if code, ok := engineCode(err); ok {
		writeError(w, http.StatusUnprocessableEntity, code, err.Error())
		return true
	}
	return false
}

// apiErrorFrom извлекает код и сообщение ошибки в конверт API.
//
// Порядок разбора: ошибка движка с контекстом → ошибка слоя данных →
// общая ошибка. Вложенные причины (errors.Is/As) разбираются прозрачно.
func apiErrorFrom(err error) apiError {
	var ee *engine.Error
	if errors.As(err, &ee) {
		payload := apiError{Code: ee.Code, Message: ee.Message, Account: ee.Account, Cell: ee.Cell}
		if ee.Amount != 0 {
			kop := ee.Amount.Kopecks()
			payload.AmountKop = &kop
		}
		return payload
	}
	if code, ok := engineCode(err); ok {
		return apiError{Code: code, Message: err.Error()}
	}
	return apiError{Code: "ERR_INTERNAL", Message: "внутренняя ошибка сервера"}
}
