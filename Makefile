BINARY     := mycnfm
MODULE     := github.com/master2sid2/mycnfm
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -X '$(MODULE)/internal/version.Version=$(VERSION)'
INSTALL_DIR := $(HOME)/.local/bin
DIST_DIR   := dist

PLATFORMS  := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: all build build-darwin build-darwin-arm64 build-darwin-amd64 dist run clean test vet fmt install uninstall tidy release mycnf-clean

all: build

## build: compile ./mycnfm with the version baked in
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

## build-darwin-arm64: cross-compile for macOS on Apple Silicon -> dist/mycnfm-darwin-arm64
build-darwin-arm64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY)-darwin-arm64 .

## build-darwin-amd64: cross-compile for macOS on Intel -> dist/mycnfm-darwin-amd64
build-darwin-amd64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY)-darwin-amd64 .

## build-darwin: build both macOS binaries (Apple Silicon + Intel) into dist/
build-darwin: build-darwin-arm64 build-darwin-amd64

## dist: release binaries for Linux and macOS (amd64 + arm64) with SHA256SUMS in dist/
dist:
	mkdir -p $(DIST_DIR)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; out=$(DIST_DIR)/$(BINARY)-$(VERSION)-$$os-$$arch; echo $$out; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o $$out . || exit 1; \
	done
	cd $(DIST_DIR) && sha256sum $(BINARY)-$(VERSION)-* > SHA256SUMS

## run: build then launch the TUI
run: build
	./$(BINARY)

## test: run the Go test suite
test:
	go test ./...

## vet: run go vet across the module
vet:
	go vet ./...

## fmt: gofmt all source files
fmt:
	gofmt -w .

## tidy: sync go.mod/go.sum
tidy:
	go mod tidy

## install: build and copy the binary onto $PATH ($(INSTALL_DIR))
install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

## uninstall: remove the installed binary
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)

## clean: remove the local build artifact (does not touch ~/.my.cnf or the vault)
clean:
	rm -f $(BINARY) $(BINARY)-v* $(DIST_DIR)/$(BINARY)-* $(DIST_DIR)/SHA256SUMS

## mycnf-clean: run the app's own `clean` subcommand (wipes ~/.my.cnf, keeps the vault)
mycnf-clean: build
	./$(BINARY) clean

## release: build a version-tagged binary, e.g. mycnfm-v0.1.0
release: build
	cp $(BINARY) $(BINARY)-$(VERSION)
