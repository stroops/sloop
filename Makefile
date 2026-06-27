.PHONY: all build test proto gen fmt vet lint sec clean install run-daemon run-cli

BINARY := sloop
DAEMON := sloopd

all: fmt vet lint test build

build:
	go build -o bin/$(BINARY) ./cmd/sloop
	go build -o bin/$(DAEMON) ./cmd/sloopd

build-cgo:
	CGO_ENABLED=1 go build -tags libsqlite3 -o bin/$(BINARY) ./cmd/sloop
	CGO_ENABLED=1 go build -tags libsqlite3 -o bin/$(DAEMON) ./cmd/sloopd

test:
	go test -v ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

sec:
	gosec ./...
	govulncheck ./...

proto:
	protoc --go_out=. --go-grpc_out=. api/proto/*.proto

gen: proto


clean:
	rm -rf bin/ api/gen/

install:
	go install ./cmd/sloop
	go install ./cmd/sloopd

run-daemon:
	./bin/$(DAEMON) --socket ~/.sloop/daemon.sock

run-cli:
	./bin/$(BINARY)
