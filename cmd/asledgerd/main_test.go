package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunContextServesHealthAndStops(t *testing.T) {
	webPort := freePort(t)
	apiPort := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		errc <- runContext(ctx, testArgs(t, webPort, apiPort))
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", apiPort)
	var health struct {
		Status string `json:"status"`
	}
	waitForHealth(t, url, &health)
	if health.Status != "ok" {
		t.Fatalf("health.status = %q, ожидалось ok", health.Status)
	}

	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("runContext: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("сервер не остановился после отмены контекста")
	}

	assertPortFree(t, webPort)
	assertPortFree(t, apiPort)
}

func TestRunContextReportsPortConflict(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("занять порт: %v", err)
	}
	defer func() { _ = occupied.Close() }()

	webAddr := occupied.Addr().String()
	apiPort := freePort(t)
	args := testArgs(t, 0, apiPort)
	args = replaceArg(args, "-web-addr", webAddr)

	err = runContext(context.Background(), args)
	if err == nil {
		t.Fatal("ожидалась ошибка занятого порта")
	}
	if !strings.Contains(err.Error(), "web:") {
		t.Fatalf("ошибка не указывает на web-сервер: %v", err)
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("ошибка не похожа на ошибку listen: %v", err)
	}

	// Второй сервер не должен остаться висеть после ошибки первого.
	assertPortFree(t, apiPort)
}

func testArgs(t *testing.T, webPort, apiPort int) []string {
	t.Helper()
	return []string{
		"-db", filepath.Join(t.TempDir(), "book.db"),
		"-assets", filepath.Join("..", "..", "assets", "logo"),
		"-charts-dir", filepath.Join("..", "..", "templates", "charts"),
		"-web-addr", fmt.Sprintf(":%d", webPort),
		"-api-addr", fmt.Sprintf(":%d", apiPort),
		"-log-level", "error",
	}
}

func replaceArg(args []string, name, value string) []string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			args[i+1] = value
		}
	}
	return args
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("получить свободный порт: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

func assertPortFree(t *testing.T, port int) {
	t.Helper()
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("порт %d остался занят: %v", port, err)
	}
	_ = listener.Close()
}

func waitForHealth(t *testing.T, url string, target any) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if response.StatusCode == http.StatusOK {
			err := json.NewDecoder(response.Body).Decode(target)
			_ = response.Body.Close()
			if err != nil {
				t.Fatalf("health: не JSON: %v", err)
			}
			return
		}
		_ = response.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", response.StatusCode)
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("healthcheck %s не ответил: %v", url, lastErr)
}
