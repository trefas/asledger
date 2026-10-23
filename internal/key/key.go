// Package key выводит ключ SQLCipher из пароля и читает его из безопасных
// источников.
//
// Пароль никогда не передаётся в argv или обычных переменных окружения:
// используются TTY, stdin/fd, файл, systemd LoadCredential или системное
// хранилище Electron. Параметры KDF хранятся в sidecar рядом с книгой; сам
// ключ в sidecar не попадает.
package key

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

// Версия формата sidecar и параметры Argon2id по умолчанию.
const (
	// ParamsVersion — версия sidecar.
	ParamsVersion = 1
	// KDFArgon2id — имя алгоритма в sidecar.
	KDFArgon2id = "argon2id"
	// CipherSQLCipher — имя шифра в sidecar.
	CipherSQLCipher = "sqlcipher"
	// SaltLen — длина соли Argon2id.
	SaltLen = 16
	// KeyLen — длина raw-ключа SQLCipher.
	KeyLen = 32
	// DefaultMemoryKiB — память Argon2id по умолчанию (64 МиБ).
	DefaultMemoryKiB = 64 * 1024
	// DefaultIterations — число проходов Argon2id.
	DefaultIterations = 3
	// DefaultParallelism — параллелизм Argon2id.
	DefaultParallelism = 4
)

// Ошибки пакета.
var (
	// ErrParamsNotFound — sidecar с параметрами KDF не найден.
	ErrParamsNotFound = errors.New("key: параметры KDF не найдены")
	// ErrPasswordRequired — нет источника пароля и нет TTY.
	ErrPasswordRequired = errors.New("key: нужен пароль книги (TTY, файл или fd)")
	// ErrPlaintextBook — книга не зашифрована, нужна миграция.
	ErrPlaintextBook = errors.New("key: книга не зашифрована; выполните asledgerctl migrate-encrypt")
	// ErrBadKey — ключ неверного формата.
	ErrBadKey = errors.New("key: ключ должен быть 32 байта, 64 hex-символа или base64")
)

// Params — параметры KDF в sidecar. Соль и параметры не являются секретом;
// результата KDF здесь нет.
type Params struct {
	Version             int    `json:"version"`
	KDF                 string `json:"kdf"`
	Salt                string `json:"salt"`
	MemoryKiB           uint32 `json:"memory_kib"`
	Iterations          uint32 `json:"iterations"`
	Parallelism         uint8  `json:"parallelism"`
	KeyLen              int    `json:"key_len"`
	Cipher              string `json:"cipher"`
	CipherCompatibility int    `json:"cipher_compatibility"`
	PageSize            int    `json:"page_size"`
}

// NewParams создаёт параметры KDF со случайной солью.
func NewParams() (*Params, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("key: не удалось получить соль: %w", err)
	}
	return &Params{
		Version:             ParamsVersion,
		KDF:                 KDFArgon2id,
		Salt:                base64.RawURLEncoding.EncodeToString(salt),
		MemoryKiB:           DefaultMemoryKiB,
		Iterations:          DefaultIterations,
		Parallelism:         DefaultParallelism,
		KeyLen:              KeyLen,
		Cipher:              CipherSQLCipher,
		CipherCompatibility: 4,
		PageSize:            4096,
	}, nil
}

// Validate проверяет параметры и границы KDF.
func (p Params) Validate() error {
	switch {
	case p.Version != ParamsVersion:
		return fmt.Errorf("key: неподдерживаемая версия параметров %d", p.Version)
	case p.KDF != KDFArgon2id:
		return fmt.Errorf("key: неподдерживаемый KDF %q", p.KDF)
	case p.Cipher != CipherSQLCipher:
		return fmt.Errorf("key: неподдерживаемый шифр %q", p.Cipher)
	case p.CipherCompatibility != 4:
		return fmt.Errorf("key: неподдерживаемая совместимость SQLCipher %d", p.CipherCompatibility)
	case p.PageSize != 4096:
		return fmt.Errorf("key: неподдерживаемый размер страницы %d", p.PageSize)
	case p.KeyLen != KeyLen:
		return fmt.Errorf("key: длина ключа %d, ожидалось %d", p.KeyLen, KeyLen)
	case p.MemoryKiB < 8*1024 || p.MemoryKiB > 1024*1024:
		return fmt.Errorf("key: память Argon2id %d КиБ вне допустимого диапазона", p.MemoryKiB)
	case p.Iterations < 1 || p.Iterations > 10:
		return fmt.Errorf("key: число итераций Argon2id %d вне допустимого диапазона", p.Iterations)
	case p.Parallelism < 1 || p.Parallelism > 16:
		return fmt.Errorf("key: параллелизм Argon2id %d вне допустимого диапазона", p.Parallelism)
	}
	salt, err := base64.RawURLEncoding.DecodeString(p.Salt)
	if err != nil {
		return fmt.Errorf("key: соль не в base64url: %w", err)
	}
	if len(salt) != SaltLen {
		return fmt.Errorf("key: длина соли %d, ожидалось %d", len(salt), SaltLen)
	}
	return nil
}

// Derive выводит raw-ключ SQLCipher из пароля.
func Derive(password []byte, p Params) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	salt, err := base64.RawURLEncoding.DecodeString(p.Salt)
	if err != nil {
		return nil, fmt.Errorf("key: не удалось разобрать соль: %w", err)
	}
	return argon2.IDKey(password, salt, p.Iterations, p.MemoryKiB, p.Parallelism, uint32(p.KeyLen)), nil
}

// ParamsPath возвращает путь к sidecar для книги.
func ParamsPath(dbPath string) string { return dbPath + ".kdf.json" }

// LoadParams читает sidecar и проверяет параметры.
func LoadParams(dbPath string) (*Params, error) {
	path := ParamsPath(dbPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrParamsNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("key: не удалось прочитать %q: %w", path, err)
	}
	var p Params
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("key: не удалось разобрать %q: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("key: %s: %w", path, err)
	}
	return &p, nil
}

// SaveParams атомарно записывает sidecar с правами 0600.
func SaveParams(dbPath string, p Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("key: не удалось сериализовать параметры: %w", err)
	}
	data = append(data, '\n')

	path := ParamsPath(dbPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("key: не удалось создать каталог %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".kdf-*.json")
	if err != nil {
		return fmt.Errorf("key: не удалось создать временный sidecar: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("key: не удалось выставить права sidecar: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("key: не удалось записать sidecar: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("key: не удалось синхронизировать sidecar: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("key: не удалось закрыть sidecar: %w", err)
	}
	// На Windows rename поверх существующего файла может не работать; для
	// первого создания sidecar этот путь не нужен, а при смене пароля старый
	// файл удаляется вызывающим кодом после успешной проверки.
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("key: не удалось заменить sidecar %q: %w", path, err)
	}
	return nil
}

// Source задаёт источники секрета. nil в *FD означает «не задан»; 0 — stdin.
type Source struct {
	PasswordFile string
	PasswordFD   *int
	KeyFile      string
	KeyFD        *int
}

// Any сообщает, задан ли хотя бы один источник.
func (s Source) Any() bool {
	return s.PasswordFile != "" || s.PasswordFD != nil || s.KeyFile != "" || s.KeyFD != nil
}

// Validate проверяет, что задан не более чем один источник.
func (s Source) Validate() error {
	password := 0
	if s.PasswordFile != "" {
		password++
	}
	if s.PasswordFD != nil {
		password++
	}
	key := 0
	if s.KeyFile != "" {
		key++
	}
	if s.KeyFD != nil {
		key++
	}
	if password > 1 {
		return errors.New("key: задано несколько источников пароля")
	}
	if key > 1 {
		return errors.New("key: задано несколько источников ключа")
	}
	if password > 0 && key > 0 {
		return errors.New("key: нельзя одновременно задать пароль и готовый ключ")
	}
	if s.PasswordFD != nil && *s.PasswordFD < 0 {
		return errors.New("key: fd пароля не может быть отрицательным")
	}
	if s.KeyFD != nil && *s.KeyFD < 0 {
		return errors.New("key: fd ключа не может быть отрицательным")
	}
	return nil
}

// Provider реализует store.KeyProvider.
type Provider struct {
	dbPath string
	source Source
}

// NewProvider создаёт поставщика ключа для книги.
func NewProvider(dbPath string, source Source) (*Provider, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("key: не задан путь к книге")
	}
	if err := source.Validate(); err != nil {
		return nil, err
	}
	return &Provider{dbPath: dbPath, source: source}, nil
}

// Key читает ключ или выводит его из пароля.
func (p *Provider) Key(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source := p.source
	if !source.Any() {
		source = systemdSource()
	}

	if source.KeyFile != "" || source.KeyFD != nil {
		return readRawKey(source)
	}

	params, err := p.paramsForOpen()
	if err != nil {
		return nil, err
	}
	password, err := readPassword(source)
	if err != nil {
		return nil, err
	}
	defer zero(password)
	key, err := Derive(password, *params)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// paramsForOpen читает sidecar или создаёт его для новой книги.
func (p *Provider) paramsForOpen() (*Params, error) {
	params, err := LoadParams(p.dbPath)
	if err == nil {
		state, inspectErr := inspectBook(p.dbPath)
		if inspectErr != nil {
			return nil, inspectErr
		}
		if state.exists && !state.empty && state.plaintext {
			return nil, ErrPlaintextBook
		}
		return params, nil
	}
	if !errors.Is(err, ErrParamsNotFound) {
		return nil, err
	}

	state, err := inspectBook(p.dbPath)
	if err != nil {
		return nil, err
	}
	if state.exists && !state.empty {
		if state.plaintext {
			return nil, ErrPlaintextBook
		}
		return nil, fmt.Errorf("%w: %s", ErrParamsNotFound, ParamsPath(p.dbPath))
	}

	params, err = NewParams()
	if err != nil {
		return nil, err
	}
	if err := SaveParams(p.dbPath, *params); err != nil {
		return nil, err
	}
	return params, nil
}

// systemdSource ищет credentials, переданные systemd LoadCredential.
func systemdSource() Source {
	dir := os.Getenv("CREDENTIALS_DIRECTORY")
	if dir == "" {
		return Source{}
	}
	if fileExists(filepath.Join(dir, "asledger-key")) {
		return Source{KeyFile: filepath.Join(dir, "asledger-key")}
	}
	if fileExists(filepath.Join(dir, "asledger-password")) {
		return Source{PasswordFile: filepath.Join(dir, "asledger-password")}
	}
	return Source{}
}

// readPassword читает пароль из источника или TTY.
func readPassword(source Source) ([]byte, error) {
	switch {
	case source.PasswordFile != "":
		return readSecretFile(source.PasswordFile)
	case source.PasswordFD != nil:
		return readSecretFD(*source.PasswordFD)
	case term.IsTerminal(int(os.Stdin.Fd())):
		fmt.Fprint(os.Stderr, "Пароль книги: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("key: не удалось прочитать пароль: %w", err)
		}
		if len(password) == 0 {
			return nil, errors.New("key: пустой пароль")
		}
		return password, nil
	default:
		return nil, ErrPasswordRequired
	}
}

// readSecretFile читает файл и убирает завершающие переводы строк.
func readSecretFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("key: не удалось прочитать %q: %w", path, err)
	}
	data = bytes.TrimRight(data, "\r\n")
	if len(data) == 0 {
		return nil, errors.New("key: пустой секрет в файле")
	}
	return data, nil
}

// readSecretFD читает секрет из унаследованного дескриптора.
func readSecretFD(fd int) ([]byte, error) {
	file := os.NewFile(uintptr(fd), fmt.Sprintf("secret-fd-%d", fd))
	if file == nil {
		return nil, fmt.Errorf("key: неверный fd %d", fd)
	}
	if fd > 2 {
		defer func() { _ = file.Close() }()
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("key: не удалось прочитать fd %d: %w", fd, err)
	}
	data = bytes.TrimRight(data, "\r\n")
	if len(data) == 0 {
		return nil, errors.New("key: пустой секрет во fd")
	}
	return data, nil
}

// readRawKey читает 32-байтовый ключ в raw, hex или base64-виде.
func readRawKey(source Source) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if source.KeyFile != "" {
		data, err = os.ReadFile(source.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("key: не удалось прочитать ключ %q: %w", source.KeyFile, err)
		}
	} else {
		data, err = readSecretFD(*source.KeyFD)
		if err != nil {
			return nil, err
		}
	}
	data = bytes.TrimSpace(data)
	if len(data) == KeyLen {
		return append([]byte(nil), data...), nil
	}
	if len(data) == 2*KeyLen {
		decoded, err := hex.DecodeString(string(data))
		if err == nil && len(decoded) == KeyLen {
			return decoded, nil
		}
	}
	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if decoded, err := encoding.DecodeString(string(data)); err == nil && len(decoded) == KeyLen {
			return decoded, nil
		}
	}
	return nil, ErrBadKey
}

// inspectBook проверяет, существует ли книга и зашифрована ли она.
type bookState struct {
	exists    bool
	empty     bool
	plaintext bool
}

func inspectBook(path string) (bookState, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return bookState{}, nil
	}
	if err != nil {
		return bookState{}, fmt.Errorf("key: не удалось открыть книгу %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return bookState{}, fmt.Errorf("key: не удалось получить размер книги %q: %w", path, err)
	}
	if info.Size() == 0 {
		return bookState{exists: true, empty: true}, nil
	}
	header := make([]byte, len("SQLite format 3\x00"))
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return bookState{}, fmt.Errorf("key: не удалось прочитать заголовок книги %q: %w", path, err)
	}
	return bookState{
		exists:    true,
		plaintext: n == len(header) && bytes.Equal(header, []byte("SQLite format 3\x00")),
	}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
