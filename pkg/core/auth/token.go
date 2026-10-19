package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// tokenLen — длина случайного токена устройства в байтах (256 бит).
const tokenLen = 32

// NewToken возвращает случайный токен устройства.
//
// Токен имеет высокую энтропию, поэтому в книге хранится не он, а его
// быстрый хеш (SHA-256) — поиск устройства по токену выполняется за O(1).
func NewToken() (string, error) {
	buf := make([]byte, tokenLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: не удалось получить токен: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken возвращает SHA-256 токена в hex — то, что хранится в книге.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// EqualToken сравнивает открытый токен с сохранённым хешем за постоянное время.
func EqualToken(tokenHash, token string) bool {
	return subtle.ConstantTimeCompare([]byte(tokenHash), []byte(HashToken(token))) == 1
}
