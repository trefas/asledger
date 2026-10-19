# Makefile asledger: единые команды сборки, тестирования и проверок.
#
# Цели используются и локально, и в CI, чтобы результаты совпадали.

GO      ?= go
BIN_DIR ?= bin
CMDS    := asledgerd asledgerctl web

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/trefas/asledger/internal/version.Version=$(VERSION) \
           -X github.com/trefas/asledger/internal/version.Commit=$(COMMIT) \
           -X github.com/trefas/asledger/internal/version.Date=$(DATE)

.PHONY: all build test test-race cover fmt fmt-check vet lint lint-money lint-go check run clean help

all: check build

## build: собрать все команды в bin/
build:
	@mkdir -p $(BIN_DIR)
	@for cmd in $(CMDS); do \
		echo "сборка cmd/$$cmd"; \
		$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$$cmd ./cmd/$$cmd || exit 1; \
	done

## test: запустить юнит-тесты
test:
	$(GO) test ./...

## test-race: тесты с детектором гонок
test-race:
	$(GO) test -race ./...

## cover: покрытие по всем пакетам
cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 1

## fmt: форматировать код
fmt:
	gofmt -w .

## fmt-check: проверить форматирование без изменения файлов
fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "нужно форматирование:"; echo "$$out"; exit 1; fi

## vet: запустить go vet
vet:
	$(GO) vet ./...

## lint-money: запрет типов с плавающей точкой в денежном коде
lint-money:
	./scripts/check-no-float.sh

## lint-go: golangci-lint (в CI обязателен, локально может отсутствовать)
lint-go:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint не установлен, шаг пропущен"; \
	fi

## lint: все статические проверки
lint: fmt-check vet lint-money lint-go

## check: полная проверка перед коммитом
check: lint test

## run: запустить демон с локальной конфигурацией
run:
	$(GO) run ./cmd/asledgerd

## clean: удалить артефакты сборки
clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html

## help: показать список целей
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
