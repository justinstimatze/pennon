# Version is derived from the git tag — `git describe` gives v0.1.0 at the
# tag and v0.1.0-3-gabc1234 three commits later. There is no version
# constant to hand-edit; `git tag vX.Y.Z` is the single source of truth.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: install build test lint version

# Install to $GOBIN/$GOPATH/bin with the version baked in.
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/pennon

# Build a local ./pennon binary with the version baked in.
build:
	go build -ldflags "$(LDFLAGS)" -o pennon ./cmd/pennon

test:
	go test ./...

lint:
	golangci-lint run

# Print the version that a build would stamp.
version:
	@echo $(VERSION)
