.PHONY: fmt lint test vuln build db-reset all

# gvm 등이 export한 GOROOT가 go.work의 toolchain(go1.26.4)과 어긋나면
# lint typecheck가 "version does not match go tool version"으로 깨진다.
# go 명령이 toolchain에 맞는 GOROOT를 스스로 정하도록 넘기지 않는다.
unexport GOROOT

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
