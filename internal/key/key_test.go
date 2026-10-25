package key

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/trefas/asledger/pkg/core/store"
)

// writeSecret создаёт файл секрета с правами 0600.
func writeSecret(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestParamsRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "book.db")
	params, err := NewParams()
	if err != nil {
		t.Fatalf("NewParams: %v", err)
	}
	if err := SaveParams(dbPath, *params); err != nil {
		t.Fatalf("SaveParams: %v", err)
	}
	path := ParamsPath(dbPath)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat sidecar: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("sidecar доступен группе/другим: %o", info.Mode().Perm())
	}
	loaded, err := LoadParams(dbPath)
	if err != nil {
		t.Fatalf("LoadParams: %v", err)
	}
	if *loaded != *params {
		t.Fatalf("параметры не совпали: %+v != %+v", loaded, params)
	}
}

func TestLoadParamsMissing(t *testing.T) {
	_, err := LoadParams(filepath.Join(t.TempDir(), "book.db"))
	if !errors.Is(err, ErrParamsNotFound) {
		t.Fatalf("LoadParams вернул %v, ожидалась ErrParamsNotFound", err)
	}
}

func TestDeriveDeterministic(t *testing.T) {
	params, err := NewParams()
	if err != nil {
		t.Fatalf("NewParams: %v", err)
	}
	first, err := Derive([]byte("пароль книги"), *params)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	second, err := Derive([]byte("пароль книги"), *params)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Argon2id не детерминирован при одной соли")
	}
	if len(first) != KeyLen {
		t.Fatalf("длина ключа %d, ожидалось %d", len(first), KeyLen)
	}
	other, err := Derive([]byte("другой пароль"), *params)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if bytes.Equal(first, other) {
		t.Fatal("разные пароли дали одинаковый ключ")
	}
}

func TestProviderPasswordFileCreatesParams(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	writeSecret(t, passwordPath, "пароль книги\n")

	provider, err := NewProvider(dbPath, Source{PasswordFile: passwordPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	first, err := provider.Key(context.Background())
	if err != nil {
		t.Fatalf("Key: %v", err)
	}
	if len(first) != KeyLen {
		t.Fatalf("длина ключа %d", len(first))
	}
	if _, err := os.Stat(ParamsPath(dbPath)); err != nil {
		t.Fatalf("sidecar не создан: %v", err)
	}
	second, err := provider.Key(context.Background())
	if err != nil {
		t.Fatalf("повторный Key: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("ключ из одного пароля и sidecar не совпал")
	}

	// Ключ открывает книгу, а sidecar позволяет открыть её повторно.
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath, store.WithKey(first))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened, err := store.Open(ctx, dbPath, store.WithKey(second))
	if err != nil {
		t.Fatalf("повторное store.Open: %v", err)
	}
	defer reopened.Close()
	if !reopened.Encrypted() {
		t.Fatal("книга открыта без шифрования")
	}
}

func TestProviderPlaintextBook(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	writeSecret(t, passwordPath, "пароль\n")

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	provider, err := NewProvider(dbPath, Source{PasswordFile: passwordPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Key(ctx); !errors.Is(err, ErrPlaintextBook) {
		t.Fatalf("Key вернул %v, ожидалась ErrPlaintextBook", err)
	}
}

func TestProviderEncryptedWithoutParams(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	passwordPath := filepath.Join(dir, "password")
	writeSecret(t, passwordPath, "пароль\n")

	ctx := context.Background()
	rawKey := bytes.Repeat([]byte{0x11}, KeyLen)
	db, err := store.Open(ctx, dbPath, store.WithKey(rawKey))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	provider, err := NewProvider(dbPath, Source{PasswordFile: passwordPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Key(ctx); !errors.Is(err, ErrParamsNotFound) {
		t.Fatalf("Key вернул %v, ожидалась ErrParamsNotFound", err)
	}
}

func TestProviderRawKeyFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	raw := bytes.Repeat([]byte{0x2a}, KeyLen)

	cases := []struct {
		name string
		data string
	}{
		{"raw", string(raw)},
		{"hex", hex.EncodeToString(raw)},
		{"base64", base64.RawURLEncoding.EncodeToString(raw)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyPath := filepath.Join(dir, tc.name+".key")
			writeSecret(t, keyPath, tc.data+"\n")
			provider, err := NewProvider(dbPath, Source{KeyFile: keyPath})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			got, err := provider.Key(context.Background())
			if err != nil {
				t.Fatalf("Key: %v", err)
			}
			if !bytes.Equal(got, raw) {
				t.Fatalf("ключ не совпал: %x", got)
			}
		})
	}
}

func TestProviderPasswordFD(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "book.db")
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	fd := int(read.Fd())
	if _, err := write.WriteString("пароль через fd\n"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = write.Close()

	provider, err := NewProvider(dbPath, Source{PasswordFD: &fd})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Key(context.Background()); err != nil {
		t.Fatalf("Key: %v", err)
	}
}

func TestSourceValidate(t *testing.T) {
	fd := 0
	tests := []struct {
		name   string
		source Source
	}{
		{"два пароля", Source{PasswordFile: "a", PasswordFD: &fd}},
		{"два ключа", Source{KeyFile: "a", KeyFD: &fd}},
		{"пароль и ключ", Source{PasswordFile: "a", KeyFile: "b"}},
		{"отрицательный fd", Source{PasswordFD: intPtr(-1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.source.Validate(); err == nil {
				t.Fatalf("Validate(%+v) не вернул ошибку", tc.source)
			}
		})
	}
	if err := (Source{PasswordFile: "a"}).Validate(); err != nil {
		t.Fatalf("Validate корректного источника: %v", err)
	}
}

func TestSystemdCredential(t *testing.T) {
	dir := t.TempDir()
	credDir := filepath.Join(dir, "credentials")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeSecret(t, filepath.Join(credDir, "asledger-password"), "пароль systemd\n")
	t.Setenv("CREDENTIALS_DIRECTORY", credDir)

	dbPath := filepath.Join(dir, "book.db")
	provider, err := NewProvider(dbPath, Source{})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Key(context.Background()); err != nil {
		t.Fatalf("Key из systemd credential: %v", err)
	}
}

func TestParamsValidateRejects(t *testing.T) {
	base, err := NewParams()
	if err != nil {
		t.Fatalf("NewParams: %v", err)
	}
	tests := []struct {
		name string
		edit func(*Params)
	}{
		{"версия", func(p *Params) { p.Version = 2 }},
		{"kdf", func(p *Params) { p.KDF = "pbkdf2" }},
		{"шифр", func(p *Params) { p.Cipher = "sqlite" }},
		{"страница", func(p *Params) { p.PageSize = 1024 }},
		{"соль", func(p *Params) { p.Salt = "короткая" }},
		{"память", func(p *Params) { p.MemoryKiB = 1 }},
		{"итерации", func(p *Params) { p.Iterations = 0 }},
		{"параллелизм", func(p *Params) { p.Parallelism = 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := *base
			tc.edit(&p)
			if err := p.Validate(); err == nil {
				t.Fatalf("Validate не отклонил %+v", p)
			}
		})
	}
}

func TestReadRawKeyBadFormat(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "bad.key")
	writeSecret(t, keyPath, strings.Repeat("z", 20))
	provider, err := NewProvider(filepath.Join(dir, "book.db"), Source{KeyFile: keyPath})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.Key(context.Background()); !errors.Is(err, ErrBadKey) {
		t.Fatalf("Key вернул %v, ожидалась ErrBadKey", err)
	}
}

func intPtr(v int) *int { return &v }
