GO ?= go
PACK_VERSION ?=
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
TALOS_VERSION ?= v1.14.1
SCHEMATIC_ID ?= 4dd8e3a8b6203d3c14f049da8db4d3bb0d6d3e70c5e89dfcc1e709e81914f63c

LDFLAGS := -X main.Version=$(VERSION) \
           -X main.ExpectedTalosVersion=$(TALOS_VERSION) \
           -X main.ExpectedSchematicID=$(SCHEMATIC_ID)

.PHONY: all fetch-pack build test vet check clean

all: check build

# Download + sha256-verify + extract the pinned install pack into installpack/.
# Required before a release build (the pack is gitignored).
fetch-pack:
	PACK_VERSION=$(PACK_VERSION) scripts/fetch-install-pack.sh

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o dist/naslos-install ./cmd/naslos-install

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

check: vet test

clean:
	rm -rf dist
