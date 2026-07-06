.PHONY: fmt lint test vuln build all

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

all: fmt lint test vuln build
