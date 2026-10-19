// Package auth хранит и проверяет секреты пользователей asledger.
//
// Для хеширования используется PBKDF2-HMAC-SHA256 со случайной солью:
// пароль или токен никогда не хранится и не сравнивается в открытом виде.
// Формат строки — «pbkdf2_sha256$<итерации>$<соль>$<хеш>» в base64url без
// выравнивания, поэтому параметры можно менять без миграции схемы.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Параметры хеширования.
const (
	// algo — имя алгоритма в кодированной строке хеша.
	algo = "pbkdf2_sha256"
	// iterations — число итераций PBKDF2.
	iterations = 100_000
	// saltLen — длина случайной соли в байтах.
	saltLen = 16
	// keyLen — длина производного ключа в байтах.
	keyLen = 32
)

// Ошибки пакета.
var (
	// ErrEmptySecret — пустой секрет.
	ErrEmptySecret = errors.New("auth: пустой секрет")
	// ErrBadHash — строка хеша не соответствует формату.
	ErrBadHash = errors.New("auth: неверный формат хеша")
)

// Hash вычисляет стойкий хеш секрета со случайной солью.
func Hash(secret string) (string, error) {
	if secret == "" {
		return "", ErrEmptySecret
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: не удалось получить соль: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, iterations, keyLen)
	if err != nil {
		return "", fmt.Errorf("auth: не удалось вычислить хеш: %w", err)
	}
	return encode(iterations, salt, key), nil
}

// Verify проверяет секрет против сохранённого хеша за постоянное время.
//
// Любая ошибка разбора хеша или несовпадение дают false: вызывающий код не
// должен различать «нет пользователя» и «неверный секрет».
func Verify(encoded, secret string) bool {
	iter, salt, want, err := decode(encoded)
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, secret, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// encode собирает кодированное представление хеша.
func encode(iter int, salt, key []byte) string {
	return strings.Join([]string{
		algo,
		strconv.Itoa(iter),
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	}, "$")
}

// decode разбирает кодированное представление хеша.
func decode(encoded string) (int, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != algo {
		return 0, nil, nil, ErrBadHash
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter <= 0 {
		return 0, nil, nil, ErrBadHash
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return 0, nil, nil, ErrBadHash
	}
	key, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(key) == 0 {
		return 0, nil, nil, ErrBadHash
	}
	return iter, salt, key, nil
}
