.PHONY: build install test fmt

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PREFIX  ?= $(HOME)/.local

build:
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" \
		-o bin/skill-observatory ./cmd/skill-observatory

install: build
	install -d $(PREFIX)/bin
	install -m 755 bin/skill-observatory $(PREFIX)/bin/skill-observatory

test:
	go test ./...

fmt:
	gofmt -s -w .
