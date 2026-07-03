.PHONY: all build test e2e proto gen fmt vet lint sec clean install run-daemon run-cli githooks

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
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Some files are not formatted correctly. Please run 'make fmt'"; \
		gofmt -l .; \
		exit 1; \
	fi
	golangci-lint run ./...

sec:
	gosec ./...
	govulncheck ./...

install:
	go install ./cmd/sloop

# Point git at the repo's committed hooks (pre-push: lint + test). Run once
# per clone.
githooks:
	git config core.hooksPath scripts/githooks

run-cli:
	./bin/$(BINARY)
