package httpui

import (
	"net/url"
	"strconv"
	"time"
)

// encodeQuery кодирует значение для строки запроса (сообщения о результате).
func encodeQuery(value string) string { return url.QueryEscape(value) }

// itoa переводит целое в строку.
func itoa(v int) string { return strconv.Itoa(v) }

// todayString возвращает сегодняшнюю дату в формате формы (ГГГГ-ММ-ДД).
func todayString() string { return time.Now().Format("2006-01-02") }
