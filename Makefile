MODULE  = github.com/supermodeltools/cli
BINARY  = supermodel
OUTDIR  = dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS = -s -w \
  -X $(MODULE)/internal/build.Version=$(VERSION) \
  -X $(MODULE)/internal/build.Commit=$(COMMIT) \
  -X $(MODULE)/internal/build.Date=$(DATE)

.PHONY: all build test test-cover lint fmt vet tidy clean release-dry help

all: build

## build: compile the binary to dist/supermodel
build:
	@mkdir -p $(OUTDIR)
	go build -ldflags="$(LDFLAGS)" -o $(OUTDIR)/$(BINARY) .

## test: run tests with race detector and coverage
test:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

## test-cover: open coverage report in browser
test-cover: test
	go tool cover -html=coverage.out

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## fmt: format all Go source files
fmt:
	gofmt -w .
	@which goimports > /dev/null && goimports -w . || true

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy and verify go.mod
tidy:
	go mod tidy
	go mod verify

## clean: remove build artifacts
clean:
	rm -rf $(OUTDIR) coverage.out

## arch-check: validate vertical slice architecture via Supermodel API
arch-check:
	@[ -n "$$SUPERMODEL_API_KEY" ] || (echo "error: SUPERMODEL_API_KEY is not set" && exit 1)
	go run ./scripts/check-architecture

## release-dry: dry-run GoReleaser (builds all platform binaries locally)
release-dry:
	goreleaser release --snapshot --clean

## help: print this message
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
