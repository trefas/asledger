package auth

import (
	"strings"
	"testing"
)

// TestHashVerify проверяет, что хеш не совпадает с секретом и проверяется.
func TestHashVerify(t *testing.T) {
	const secret = "секрет-андрея"

	encoded, err := Hash(secret)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if encoded == secret || strings.Contains(encoded, secret) {
		t.Fatalf("хеш содержит секрет: %q", encoded)
	}
	if !strings.HasPrefix(encoded, "pbkdf2_sha256$") {
		t.Fatalf("неожиданный формат хеша: %q", encoded)
	}
	if !Verify(encoded, secret) {
		t.Fatal("Verify не подтвердил верный секрет")
	}
	if Verify(encoded, "другой-секрет") {
		t.Fatal("Verify подтвердил неверный секрет")
	}
}

// TestHashSaltIsRandom проверяет, что одинаковая строка даёт разные хеши.
func TestHashSaltIsRandom(t *testing.T) {
	a, err := Hash("одинаковый")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	b, err := Hash("одинаковый")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if a == b {
		t.Fatal("два хеша одной строки совпали — соль не случайна")
	}
}

// TestHashEmpty отвергает пустой секрет.
func TestHashEmpty(t *testing.T) {
	if _, err := Hash(""); err == nil {
		t.Fatal("Hash пустого секрета не вернул ошибку")
	}
}

// TestVerifyBadHash отвергает испорченные строки хеша.
func TestVerifyBadHash(t *testing.T) {
	cases := map[string]string{
		"пусто":            "",
		"без алгоритма":    "100000$соль$хеш",
		"чужой алгоритм":   "md5$100000$AAAA$AAAA",
		"нечисловая соль":  "pbkdf2_sha256$100000$AAAA",
		"плохой base64":    "pbkdf2_sha256$100000$!!!$AAAA",
		"нулевые итерации": "pbkdf2_sha256$0$AAAA$AAAA",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if Verify(encoded, "секрет") {
				t.Fatalf("Verify принял некорректный хеш %q", encoded)
			}
		})
	}
}
