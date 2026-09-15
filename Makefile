BINARY     := mycnfm
MODULE     := mycnfm
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -X '$(MODULE)/internal/version.Version=$(VERSION)'
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: all build run clean test vet fmt install uninstall tidy release mycnf-clean

all: build

## build: compile ./mycnfm with the version baked in
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

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
	rm -f $(BINARY) $(BINARY)-v*

## mycnf-clean: run the app's own `clean` subcommand (wipes ~/.my.cnf, keeps the vault)
mycnf-clean: build
	./$(BINARY) clean

## release: build a version-tagged binary, e.g. mycnfm-v0.1.0
release: build
	cp $(BINARY) $(BINARY)-$(VERSION)
