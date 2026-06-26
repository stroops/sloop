.PHONY: all build test proto gen lint clean install

BINARY := sloop
DAEMON := sloopd

all: build

build:
	go build -o bin/$(BINARY) ./cmd/sloop
	go build -o bin/$(DAEMON) ./cmd/sloopd

build-cgo:
	CGO_ENABLED=1 go build -tags libsqlite3 -o bin/$(BINARY) ./cmd/sloop
	CGO_ENABLED=1 go build -tags libsqlite3 -o bin/$(DAEMON) ./cmd/sloopd

test:
	go test -v ./...

proto:
	protoc --go_out=. --go-grpc_out=. api/proto/*.proto

gen: proto

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ api/gen/

install:
	go install ./cmd/sloop
	go install ./cmd/sloopd

run-daemon:
	./bin/$(DAEMON) --socket ~/.sloop/daemon.sock

run-cli:
	./bin/$(BINARY)
