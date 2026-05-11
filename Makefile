PREFIX ?= $(HOME)/.local
BIN := kit
DIST := dist

.PHONY: all build install uninstall test fmt vet tidy clean run

all: build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	@mkdir -p $(DIST)
	go build -ldflags "-s -w -X github.com/andreicstoica/kit/cmd.version=$(VERSION)" -o $(DIST)/$(BIN) .

install: build
	@mkdir -p $(PREFIX)/bin
	cp $(DIST)/$(BIN) $(PREFIX)/bin/$(BIN)
	@echo "installed: $(PREFIX)/bin/$(BIN)"
	@echo "ensure $(PREFIX)/bin is on PATH (add to ~/.zshrc if needed)"

uninstall:
	rm -f $(PREFIX)/bin/$(BIN)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf $(DIST)

run:
	go run . $(ARGS)
