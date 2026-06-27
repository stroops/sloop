.PHONY: all build test proto gen fmt vet lint sec clean install run-daemon run-cli

BINARY := sloop

all: fmt vet lint test build

build:
	go build -o bin/$(BINARY) ./cmd/sloop

build-cgo:
	CGO_ENABLED=1 go build -tags libsqlite3 -o bin/$(BINARY) ./cmd/sloop

test:
	go test -v ./...

e2e:
	go test -tags e2e -count=1 ./e2e/...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

sec:
	gosec ./...
	govulncheck ./...

install:
	go install ./cmd/sloop

run-cli:
	./bin/$(BINARY)
