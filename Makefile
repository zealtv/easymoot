GO ?= go
PREFIX ?= $(HOME)/.local
DESTDIR ?=
BIN := bin/easymoot
# The release version, from VERSION at the root, stamped into the binary as vX.Y.Z,
# which is what `go install …@vX.Y.Z` reports too.
VERSION := $(shell cat VERSION 2>/dev/null)
LDFLAGS := $(if $(VERSION),-ldflags "-X main.version=v$(VERSION)")
export CGO_ENABLED = 0
# Never reuse another worktree's module or build cache.
export GOPATH := $(CURDIR)/.cache/go
export GOMODCACHE := $(CURDIR)/.cache/go/pkg/mod
export GOCACHE := $(CURDIR)/.cache/go-build
# Keep the module cache writable, so a checkout (or a release tool's temporary
# export) can be removed with plain rm -rf.
GOFLAGS += -modcacherw
export GOFLAGS

.PHONY: build test check install cross-check acceptance clean
build:
	$(GO) build $(LDFLAGS) -o $(BIN) ./cmd/easymoot

test:
	$(GO) test ./...

check:
	@files="$$(gofmt -l $$(ls -d cmd internal 2>/dev/null))" && test -z "$$files" || { echo 'Go formatting needed'; exit 1; }
	$(GO) vet ./...

# The MVP acceptance session (internal/acceptance): one session over a synthetic
# home of moots, through the model and the real moot.sh, then the built binary.
# EASYMOOT_ACCEPTANCE=1 turns it on (plain `go test` skips it). Needs bash.
acceptance:
	EASYMOOT_ACCEPTANCE=1 $(GO) test -count=1 -v ./internal/acceptance

install: build
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -m 755 "$(BIN)" "$(DESTDIR)$(PREFIX)/bin/easymoot"

cross-check:
	@set -e; for os in darwin linux; do \
	  for arch in amd64 arm64; do \
	    echo "Building $$os/$$arch (CGO_ENABLED=0)"; \
	    GOOS=$$os GOARCH=$$arch $(GO) build $(LDFLAGS) -o "dist/easymoot-$$os-$$arch" ./cmd/easymoot; \
	  done; \
	done

clean:
	rm -rf bin dist
