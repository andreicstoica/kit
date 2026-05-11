PREFIX ?= $(HOME)/.local
BIN := kit
DIST := dist

.PHONY: all build install uninstall test fmt vet tidy clean run demo

all: build

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	@mkdir -p $(DIST)
	go build -ldflags "-s -w -X github.com/andreicstoica/kit/cmd.version=$(VERSION)" -o $(DIST)/$(BIN) .

install: build
	@mkdir -p $(PREFIX)/bin
	cp $(DIST)/$(BIN) $(PREFIX)/bin/$(BIN)
	@# macOS attaches com.apple.provenance to freshly-built local binaries,
	@# which can cause Gatekeeper to SIGKILL the process on launch. Ad-hoc
	@# re-signing clears the flag chain so the binary runs cleanly.
	@if [ "$$(uname)" = "Darwin" ]; then codesign --force --sign - $(PREFIX)/bin/$(BIN) 2>/dev/null || true; fi
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

# Record demo GIFs from vhs/*.tape. Requires `brew install vhs`.
demo:
	@command -v vhs >/dev/null || { echo "vhs not installed — brew install vhs"; exit 1; }
	@for tape in vhs/*.tape; do echo "→ $$tape"; vhs $$tape; done
