.PHONY: fmt lint test vuln build db-reset all

DEFAULT_DB_PATH := $(if $(filter Darwin,$(shell uname -s)),$(HOME)/Library/Application Support/neulsang/neulsang.db,$(HOME)/.config/neulsang/neulsang.db)

fmt:
	gofumpt -l -w .

lint:
	golangci-lint run ./...

test:
	go test -race ./...

vuln:
	govulncheck ./...

build:
	go build ./...

# macOS/Linux only. Override the config default with NEULSANG_DB_PATH when needed.
db-reset:
	DB_PATH="$${NEULSANG_DB_PATH:-$(DEFAULT_DB_PATH)}"; rm -f "$${DB_PATH}" "$${DB_PATH}-wal" "$${DB_PATH}-shm"

all: fmt lint test vuln build
