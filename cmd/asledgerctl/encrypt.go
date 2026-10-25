package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/trefas/asledger/internal/key"
	"github.com/trefas/asledger/pkg/core/store"
)

// runMigrateEncrypt зашифровывает незашифрованную книгу SQLCipher.
func runMigrateEncrypt(args []string) int {
	fs := newFlagSet("asledgerctl migrate-encrypt")
	opts := commonFlags(fs)
	dryRun := fs.Bool("dry-run", false, "проверить книгу и показать план без изменений")
	backup := fs.String("backup", "", "путь к зашифрованной резервной копии")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if *opts.allowPlaintext {
		return fail(errors.New("migrate-encrypt: -allow-plaintext не нужен: команда сама открывает plaintext-книгу"))
	}

	cfg, err := loadConfig(*opts.configPath, *opts.dbPath)
	if err != nil {
		return fail(err)
	}
	if err := requireBookFile(cfg.DBPath); err != nil {
		return fail(fmt.Errorf("migrate-encrypt: %w", err))
	}
	ctx := context.Background()
	source, err := store.Open(ctx, cfg.DBPath)
	if errors.Is(err, store.ErrKeyRequired) {
		return fail(errors.New("migrate-encrypt: книга уже зашифрована"))
	}
	if err != nil {
		return fail(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = source.Close()
		}
	}()

	if _, err := os.Stat(key.ParamsPath(cfg.DBPath)); err == nil {
		return fail(fmt.Errorf("migrate-encrypt: sidecar %q уже существует; удалите его только после проверки книги",
			key.ParamsPath(cfg.DBPath)))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	if err := source.IntegrityCheck(ctx); err != nil {
		return fail(err)
	}
	if *dryRun {
		// Не меняем схему и WAL: dry-run только проверяет книгу.
		fmt.Printf("книга %s не зашифрована; можно выполнить migrate-encrypt\n", cfg.DBPath)
		return 0
	}
	if err := source.Migrate(ctx); err != nil {
		return fail(err)
	}
	if _, err := source.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fail(fmt.Errorf("migrate-encrypt: checkpoint: %w", err))
	}
	if err := source.IntegrityCheck(ctx); err != nil {
		return fail(err)
	}

	newSource := opts.source()
	if newSource.Any() && cfg.Key.Any() {
		return fail(errors.New("migrate-encrypt: источники нового ключа заданы и флагами, и окружением"))
	}
	if !newSource.Any() {
		newSource = cfg.Key
	}
	newKey, params, err := newKeyForEncryption(ctx, cfg.DBPath, newSource)
	if err != nil {
		return fail(err)
	}
	defer key.Zero(newKey)

	temp, err := tempDBPath(cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.Remove(temp) }()
	if err := source.ExportEncrypted(ctx, temp, newKey); err != nil {
		return fail(err)
	}
	if err := verifyEncrypted(ctx, temp, newKey); err != nil {
		return fail(fmt.Errorf("migrate-encrypt: проверка копии: %w", err))
	}

	backupPath := *backup
	if backupPath == "" {
		backupPath = uniquePath(cfg.DBPath + ".encrypted-backup.db")
	}
	if _, err := os.Stat(backupPath); err == nil {
		return fail(fmt.Errorf("migrate-encrypt: резервная копия %q уже существует", backupPath))
	}
	committed := false
	defer func() {
		// Если миграция не дошла до рабочей книги, зашифрованная копия без
		// sidecar (или с неизвестным ключом) пользователю бесполезна.
		if !committed {
			_ = os.Remove(backupPath)
			_ = os.Remove(key.ParamsPath(backupPath))
		}
	}()
	if err := copyFile(temp, backupPath); err != nil {
		return fail(fmt.Errorf("migrate-encrypt: резервная копия: %w", err))
	}

	// sidecar пишем до замены книги: если замена не удастся, его можно
	// удалить, а исходная plaintext-книга останется на месте.
	sidecarWritten := false
	if params != nil {
		if err := key.SaveParams(cfg.DBPath, *params); err != nil {
			return fail(err)
		}
		sidecarWritten = true
		if err := copyFile(key.ParamsPath(cfg.DBPath), key.ParamsPath(backupPath)); err != nil {
			_ = os.Remove(key.ParamsPath(cfg.DBPath))
			_ = os.Remove(backupPath)
			return fail(fmt.Errorf("migrate-encrypt: sidecar резервной копии: %w", err))
		}
	}

	if err := source.Close(); err != nil {
		if sidecarWritten {
			_ = os.Remove(key.ParamsPath(cfg.DBPath))
		}
		return fail(err)
	}
	closed = true
	removeWAL(cfg.DBPath)
	if err := replaceFile(temp, cfg.DBPath); err != nil {
		if sidecarWritten {
			_ = os.Remove(key.ParamsPath(cfg.DBPath))
		}
		return fail(fmt.Errorf("migrate-encrypt: замена книги: %w", err))
	}
	if err := os.Chmod(cfg.DBPath, 0o600); err != nil {
		return fail(fmt.Errorf("migrate-encrypt: права книги: %w", err))
	}
	committed = true

	fmt.Printf("книга зашифрована: %s\n", cfg.DBPath)
	fmt.Printf("резервная копия: %s\n", backupPath)
	if params == nil {
		fmt.Println("ключ задан raw-ключом; sidecar не создан, сохраните ключ отдельно")
	}
	return 0
}

// runPasswd меняет пароль зашифрованной книги с перешифровкой и проверкой.
func runPasswd(args []string) int {
	fs := newFlagSet("asledgerctl passwd")
	opts := commonFlags(fs)
	newPasswordFile := fs.String("new-password-file", "", "файл с новым паролем книги")
	newPasswordFD := fs.Int("new-password-fd", -1, "fd с новым паролем книги (0 — stdin)")
	backup := fs.String("backup", "", "путь к резервной копии старой книги")
	dryRun := fs.Bool("dry-run", false, "проверить старый пароль без изменений")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if *opts.allowPlaintext {
		return fail(errors.New("passwd: -allow-plaintext не поддерживается"))
	}
	if *newPasswordFile != "" && *newPasswordFD >= 0 {
		return fail(errors.New("passwd: задайте только один источник нового пароля"))
	}

	cfg, err := loadConfig(*opts.configPath, *opts.dbPath)
	if err != nil {
		return fail(err)
	}
	if err := requireBookFile(cfg.DBPath); err != nil {
		return fail(fmt.Errorf("passwd: %w", err))
	}
	if _, err := key.LoadParams(cfg.DBPath); err != nil {
		return fail(fmt.Errorf("passwd: %w", err))
	}
	oldSource := opts.source()
	if oldSource.Any() && cfg.Key.Any() {
		return fail(errors.New("passwd: источники старого ключа заданы и флагами, и окружением"))
	}
	if !oldSource.Any() {
		oldSource = cfg.Key
	}

	ctx := context.Background()
	provider, err := key.NewProvider(cfg.DBPath, oldSource)
	if err != nil {
		return fail(err)
	}
	oldKey, err := provider.Key(ctx)
	if err != nil {
		return fail(err)
	}
	defer key.Zero(oldKey)
	oldDB, err := store.Open(ctx, cfg.DBPath, store.WithKey(oldKey))
	if err != nil {
		return fail(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = oldDB.Close()
		}
	}()
	if !oldDB.Encrypted() {
		return fail(errors.New("passwd: книга не зашифрована"))
	}
	if *dryRun {
		fmt.Println("старый пароль/ключ подходит; книга зашифрована")
		return 0
	}

	newSource := key.Source{PasswordFile: *newPasswordFile}
	if *newPasswordFD >= 0 {
		newSource.PasswordFD = newPasswordFD
	}
	params, err := key.NewParams()
	if err != nil {
		return fail(err)
	}
	newKey, err := deriveNewPassword(newSource, *params)
	if err != nil {
		return fail(err)
	}
	defer key.Zero(newKey)

	if _, err := oldDB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fail(fmt.Errorf("passwd: checkpoint: %w", err))
	}
	temp, err := tempDBPath(cfg.DBPath)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.Remove(temp) }()
	if err := oldDB.ExportEncrypted(ctx, temp, newKey); err != nil {
		return fail(err)
	}
	if err := verifyEncrypted(ctx, temp, newKey); err != nil {
		return fail(fmt.Errorf("passwd: проверка новой книги: %w", err))
	}

	backupPath := *backup
	if backupPath == "" {
		backupPath = uniquePath(cfg.DBPath + ".pre-rekey.db")
	}
	if _, err := os.Stat(backupPath); err == nil {
		return fail(fmt.Errorf("passwd: резервная копия %q уже существует", backupPath))
	}
	if err := oldDB.Close(); err != nil {
		return fail(err)
	}
	closed = true
	removeWAL(cfg.DBPath)
	if err := copyFile(cfg.DBPath, backupPath); err != nil {
		return fail(fmt.Errorf("passwd: резервная копия книги: %w", err))
	}
	if err := copyFile(key.ParamsPath(cfg.DBPath), key.ParamsPath(backupPath)); err != nil {
		_ = os.Remove(backupPath)
		return fail(fmt.Errorf("passwd: резервная копия sidecar: %w", err))
	}
	if err := replaceFile(temp, cfg.DBPath); err != nil {
		return fail(fmt.Errorf("passwd: замена книги: %w", err))
	}
	if err := key.SaveParams(cfg.DBPath, *params); err != nil {
		// Возвращаем старую книгу и старый sidecar, не удаляя резервную
		// копию: пользователь не должен остаться с новым файлом и старым
		// ключом.
		if restoreErr := restoreFile(backupPath, cfg.DBPath); restoreErr != nil {
			return fail(fmt.Errorf("passwd: не удалось записать sidecar: %w; не удалось восстановить книгу из %s: %s",
				err, backupPath, restoreErr.Error()))
		}
		_ = os.Remove(key.ParamsPath(cfg.DBPath))
		if copyErr := copyFile(key.ParamsPath(backupPath), key.ParamsPath(cfg.DBPath)); copyErr != nil {
			return fail(fmt.Errorf("passwd: не удалось записать sidecar: %w; книга восстановлена из %s, восстановите sidecar %s вручную: %s",
				err, backupPath, key.ParamsPath(backupPath), copyErr.Error()))
		}
		return fail(fmt.Errorf("passwd: не удалось записать sidecar: %w", err))
	}
	if err := os.Chmod(cfg.DBPath, 0o600); err != nil {
		return fail(fmt.Errorf("passwd: права книги: %w", err))
	}

	fmt.Printf("пароль книги изменён: %s\n", cfg.DBPath)
	fmt.Printf("резервная копия: %s\n", backupPath)
	fmt.Println("если ключ сохранён в Electron, его нужно заменить при следующем открытии")
	return 0
}

// newKeyForEncryption создаёт новый ключ и параметры KDF для миграции.
func newKeyForEncryption(ctx context.Context, dbPath string, source key.Source) ([]byte, *key.Params, error) {
	if source.KeyFile != "" || source.KeyFD != nil {
		provider, err := key.NewProvider(dbPath, source)
		if err != nil {
			return nil, nil, err
		}
		raw, err := provider.Key(ctx)
		if err != nil {
			return nil, nil, err
		}
		return raw, nil, nil
	}
	params, err := key.NewParams()
	if err != nil {
		return nil, nil, err
	}
	if source.Any() {
		raw, err := key.DeriveFromSource(source, *params)
		if err != nil {
			return nil, nil, err
		}
		return raw, params, nil
	}
	first, err := key.PromptPassword("Новый пароль книги: ")
	if err != nil {
		return nil, nil, err
	}
	second, err := key.PromptPassword("Повторите пароль: ")
	if err != nil {
		key.Zero(first)
		return nil, nil, err
	}
	defer key.Zero(second)
	if !bytes.Equal(first, second) {
		key.Zero(first)
		return nil, nil, errors.New("пароли не совпадают")
	}
	raw, err := key.Derive(first, *params)
	key.Zero(first)
	if err != nil {
		return nil, nil, err
	}
	return raw, params, nil
}

// deriveNewPassword выводит новый ключ из файла/fd или интерактивного ввода.
func deriveNewPassword(source key.Source, params key.Params) ([]byte, error) {
	if source.Any() {
		return key.DeriveFromSource(source, params)
	}
	first, err := key.PromptPassword("Новый пароль книги: ")
	if err != nil {
		return nil, err
	}
	second, err := key.PromptPassword("Повторите пароль: ")
	if err != nil {
		key.Zero(first)
		return nil, err
	}
	defer key.Zero(second)
	if !bytes.Equal(first, second) {
		key.Zero(first)
		return nil, errors.New("пароли не совпадают")
	}
	raw, err := key.Derive(first, params)
	key.Zero(first)
	return raw, err
}

// verifyEncrypted открывает копию новым ключом, применяет миграции и
// проверяет целостность.
func verifyEncrypted(ctx context.Context, path string, rawKey []byte) error {
	db, err := store.Open(ctx, path, store.WithKey(rawKey))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if !db.Encrypted() {
		return errors.New("книга открылась без шифрования")
	}
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	return db.IntegrityCheck(ctx)
}

// requireBookFile проверяет, что книга существует и не является каталогом.
func requireBookFile(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("книга не найдена: %s", path)
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("путь книги является каталогом: %s", path)
	}
	return nil
}

// tempDBPath создаёт свободное имя во временном каталоге рядом с книгой.
func tempDBPath(dbPath string) (string, error) {
	dir := filepath.Dir(dbPath)
	f, err := os.CreateTemp(dir, ".asledger-encrypt-*.db")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

// uniquePath добавляет суффикс, если путь уже занят.
func uniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	return path + "." + time.Now().Format("20060102-150405")
}

// removeWAL удаляет осиротевшие WAL/SHM после закрытия книги.
func removeWAL(dbPath string) {
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
}

// copyFile копирует файл с правами 0600 и синхронизацией.
func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(target)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

// restoreFile копирует source в target, сохраняя source. Используется для
// отката, чтобы резервная копия осталась на месте.
func restoreFile(source, target string) error {
	temp, err := tempDBPath(target)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	if err := copyFile(source, temp); err != nil {
		return err
	}
	return replaceFile(temp, target)
}

// replaceFile заменяет target файлом source. На Unix rename атомарен; на
// Windows target сначала уводится в .old, затем заменяется.
func replaceFile(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	old := target + ".old"
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(old, target)
		return err
	}
	_ = os.Remove(old)
	return nil
}
