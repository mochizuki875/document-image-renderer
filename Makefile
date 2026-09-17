GO ?= go

.PHONY: all build fmt test test-integration vet verify

all: verify

build:
	$(GO) build -o bin/document-image-renderer ./cmd/document-image-renderer

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

test-integration:
	RUN_INTEGRATION_TESTS=1 $(GO) test -v ./test/integration

vet:
	$(GO) vet ./...

verify: fmt vet test
