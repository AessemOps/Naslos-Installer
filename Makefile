GO ?= go
PACK_VERSION ?=
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# The pack's Talos version and schematic id are read from the fetched pack so a
# build always matches what it embeds (FR-INSTALL-02). The values below are
# only the fallback for a checkout with no pack (unit tests, `go build ./...`).
PACK_METADATA := installpack/metadata.json
PACK_TALOS_VERSION := $(shell sed -n 's/.*"talosVersion"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' $(PACK_METADATA) 2>/dev/null)
PACK_SCHEMATIC_ID := $(shell sed -n 's/.*"schematicId"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' $(PACK_METADATA) 2>/dev/null)
TALOS_VERSION ?= $(if $(PACK_TALOS_VERSION),$(PACK_TALOS_VERSION),v1.14.1)
SCHEMATIC_ID ?= $(if $(PACK_SCHEMATIC_ID),$(PACK_SCHEMATIC_ID),4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c)

LDFLAGS := -X main.Version=$(VERSION) \
           -X main.ExpectedTalosVersion=$(TALOS_VERSION) \
           -X main.ExpectedSchematicID=$(SCHEMATIC_ID)

.PHONY: all fetch-pack fetch-latest-pack build test vet check clean

all: check build

# Download + sha256-verify + extract the pinned install pack into installpack/.
# Required before a release build (the pack is gitignored).
fetch-pack:
	PACK_VERSION=$(PACK_VERSION) scripts/fetch-install-pack.sh

# Resolve the newest vX.Y.Z tag on Naslos-Linux and embed that pack. This is
# what the release CI and a "latest" local build use.
fetch-latest-pack:
	scripts/fetch-install-pack.sh

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o dist/naslos-install ./cmd/naslos-install

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

check: vet test

clean:
	rm -rf dist
