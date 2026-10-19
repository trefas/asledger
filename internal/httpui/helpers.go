package httpui

import (
	"net/url"
	"strconv"
)

// encodeQuery кодирует значение для строки запроса (сообщения о результате).
func encodeQuery(value string) string { return url.QueryEscape(value) }

// itoa переводит целое в строку.
func itoa(v int) string { return strconv.Itoa(v) }
