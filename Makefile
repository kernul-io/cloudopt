.PHONY: build test lint tidy clean

BINARY ?= main
LDFLAGS ?= -s -w

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/

test:
	go test -race ./...

test-coverage:
	go test -race -coverpkg=./... -coverprofile=coverage.out ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) coverage.out
