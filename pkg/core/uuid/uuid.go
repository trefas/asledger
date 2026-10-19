// Package uuid создаёт идентификаторы проводок и корректировок.
//
// Идентификатор — UUID версии 4 из криптографически стойкого источника
// случайности: uuid отделяет повторную отправку от новой операции.
package uuid

import (
	"crypto/rand"
	"encoding/hex"
)

// New возвращает новый UUID версии 4.
func New() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // версия 4
	b[8] = (b[8] & 0x3f) | 0x80 // вариант RFC 4122
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}
