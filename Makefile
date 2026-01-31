VERSION ?= v1.0.0
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS = -X 'main.Version=$(VERSION)' \
          -X 'main.Commit=$(COMMIT)' \
          -X 'main.Date=$(DATE)'

.PHONY: build build-cli build-all test race vet clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/heimdall ./cmd/heimdall

build-cli:
	go build -ldflags "$(LDFLAGS)" -o bin/heimdall-cli ./cmd/heimdall-cli

build-all: build build-cli

test:
	go test ./...

race:
	go test ./... -race

vet:
	go vet ./...

clean:
	rm -rf bin
