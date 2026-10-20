// Package release проверяет согласованность артефактов релиза: конфигурацию
// GoReleaser, workflow публикации, systemd-юнит и документацию по установке.
//
// Проверки статические: они не собирают бинарники, а гарантируют, что тег
// запустит полный релиз (обе платформы, архивы, SHA-256, шаблоны и ассеты)
// и что служба описана корректно.
package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trefas/asledger/internal/version"
)

// repoFile читает файл из корня репозитория.
func repoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("не удалось прочитать %s: %v", name, err)
	}
	return string(data)
}

// requireContains проверяет наличие подстроки и сообщает, чего не хватает.
func requireContains(t *testing.T, name, content, want string) {
	t.Helper()
	if !strings.Contains(content, want) {
		t.Errorf("%s: нет %q", name, want)
	}
}

// TestASL_13_ReleaseConfig проверяет, что по тегу собираются все целевые
// платформы, в архивы попадают шаблоны и ассеты, а публикация ждёт тестов.
func TestASL_13_ReleaseConfig(t *testing.T) {
	goreleaser := repoFile(t, ".goreleaser.yaml")
	for _, want := range []string{
		"project_name: asledger",
		"main: ./cmd/asledgerd",
		"main: ./cmd/asledgerctl",
		"binary: asledgerd",
		"binary: asledgerctl",
		"linux",
		"windows",
		"amd64",
		"arm64",
		"CGO_ENABLED=0",
		"-X github.com/trefas/asledger/internal/version.Version={{ .Version }}",
		"checksums.txt",
		"sha256",
		"templates/charts",
		"assets/logo",
		"packaging/systemd",
		"README.md",
		"LICENSE",
	} {
		requireContains(t, ".goreleaser.yaml", goreleaser, want)
	}

	workflow := repoFile(t, ".github/workflows/release.yml")
	for _, want := range []string{
		`- "v*"`,
		"needs: test",
		"make check",
		"make test-race",
		"goreleaser",
		"GITHUB_TOKEN",
		`grep -F "${GITHUB_REF_NAME}"`,
	} {
		requireContains(t, ".github/workflows/release.yml", workflow, want)
	}

	makefile := repoFile(t, "Makefile")
	for _, want := range []string{
		"-X github.com/trefas/asledger/internal/version.Version=$(VERSION)",
		"-X github.com/trefas/asledger/internal/version.Commit=$(COMMIT)",
		"-X github.com/trefas/asledger/internal/version.Date=$(DATE)",
	} {
		requireContains(t, "Makefile", makefile, want)
	}
}

// TestASL_13_SystemdUnit проверяет юнит службы Linux: запуск демона, порты,
// каталог книги и ограничения безопасности.
func TestASL_13_SystemdUnit(t *testing.T) {
	unit := repoFile(t, "packaging/systemd/asledgerd.service")
	for _, want := range []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"ExecStart=/usr/local/bin/asledgerd",
		"-db /var/lib/asledger/asledger.db",
		"-assets /usr/local/share/asledger/assets/logo",
		"-charts-dir /usr/local/share/asledger/templates/charts",
		"-web-addr :8777",
		"-api-addr :8778",
		"Restart=on-failure",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"ReadWritePaths=/var/lib/asledger",
		"WantedBy=multi-user.target",
	} {
		requireContains(t, "systemd/asledgerd.service", unit, want)
	}

	env := repoFile(t, "packaging/systemd/asledgerd.env")
	for _, want := range []string{"ASLEDGER_WEB_ADDR=:8777", "ASLEDGER_API_ADDR=:8778"} {
		requireContains(t, "systemd/asledgerd.env", env, want)
	}
}

// TestASL_13_InstallDocs проверяет, что инструкции по установке описывают
// оба пути (systemd и Windows) и порты 8777/8778.
func TestASL_13_InstallDocs(t *testing.T) {
	linux := repoFile(t, "docs/install-linux.md")
	for _, want := range []string{"systemd", "asledgerd", "8777", "8778", "asledgerctl backup", "systemctl enable"} {
		requireContains(t, "docs/install-linux.md", linux, want)
	}

	windows := repoFile(t, "docs/install-windows.md")
	for _, want := range []string{"asledgerd.exe", "asledgerctl.exe", "nssm", "sc.exe", "8777", "8778", "New-NetFirewallRule"} {
		requireContains(t, "docs/install-windows.md", windows, want)
	}

	release := repoFile(t, "docs/release.md")
	for _, want := range []string{"v1.0.0", "checksums.txt", "SHA-256", "linux/arm64", "windows/amd64", "release notes"} {
		requireContains(t, "docs/release.md", release, want)
	}
}

// TestASL_13_VersionDefaults проверяет внятные значения версии по умолчанию
// и формат строки версии: локальная сборка без -ldflags не показывает пустоту.
func TestASL_13_VersionDefaults(t *testing.T) {
	if version.Version == "" || version.Commit == "" || version.Date == "" {
		t.Fatalf("пустые значения версии: %q, %q, %q", version.Version, version.Commit, version.Date)
	}
	if version.Version != "dev" {
		t.Errorf("версия по умолчанию %q, ожидалась dev", version.Version)
	}
	line := version.String()
	for _, want := range []string{version.Version, version.Commit, version.Date} {
		if !strings.Contains(line, want) {
			t.Errorf("строка версии %q не содержит %q", line, want)
		}
	}
}
