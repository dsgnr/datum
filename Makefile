VENV     := .venv
PYTHON   := $(VENV)/bin/python
ZENSICAL := $(VENV)/bin/zensical
STAMP    := $(VENV)/.installed

# Datum runs on Linux, so a build on any other machine is for development only.
# CGO is off because the agent has to run on a host with nothing installed on it.
GO_BUILD := CGO_ENABLED=0 go build

.PHONY: help build build-linux dist test test-linux fmt vet lint shell clean \
	docs-install docs-serve docs-build docs-check

help:
	@echo "build         Build ./bin/datum for this machine"
	@echo "build-linux   Cross-compile for linux/amd64 and linux/arm64"
	@echo "dist          Build every supported target into ./bin"
	@echo "test          Run the Go tests"
	@echo "test-linux    Run the Go tests in a Linux container"
	@echo "fmt           Format the Go sources"
	@echo "vet           Run go vet"
	@echo "lint          fmt check, vet and tests, which is what CI runs"
	@echo "shell         Open a Linux container with datum and the example on it"
	@echo "docs-install  Create $(VENV) and install the documentation toolchain"
	@echo "docs-serve    Preview the documentation locally"
	@echo "docs-build    Build the site into ./site"
	@echo "docs-check    Build the site with --strict"
	@echo "clean         Remove build output and caches"

build:
	$(GO_BUILD) -o bin/datum ./cmd/datum

build-linux: bin/datum-linux-amd64 bin/datum-linux-arm64

bin/datum-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $@ ./cmd/datum

bin/datum-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $@ ./cmd/datum

dist: build build-linux

test:
	go test ./...

# Applying state is implemented on Linux only, so those tests skip everywhere else.
# This runs the whole suite where all of it is reachable.
test-linux:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.25 go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

# Fails if anything is unformatted, rather than reformatting it, so CI reports
# the problem instead of hiding it.
lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "these files need gofmt:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	go test ./...

# A Linux shell with the right binary already on it, because the agent's target is
# Linux and most development machines are not. The architecture is taken from
# Docker so this works on both Apple Silicon and x86.
shell:
	@arch=$$(docker version --format '{{.Server.Arch}}'); \
	echo "building for linux/$$arch"; \
	GOOS=linux GOARCH=$$arch $(GO_BUILD) -o bin/datum-linux-$$arch ./cmd/datum; \
	docker run --rm -it \
		-v "$(CURDIR)/bin/datum-linux-$$arch:/usr/local/bin/datum:ro" \
		-v "$(CURDIR)/examples:/examples:ro" \
		-w / ubuntu:24.04 bash

$(STAMP): requirements-docs.txt
	python3 -m venv $(VENV)
	$(PYTHON) -m pip install --quiet --upgrade pip
	$(PYTHON) -m pip install --quiet --requirement requirements-docs.txt
	@touch $(STAMP)

docs-install: $(STAMP)

docs-serve: $(STAMP)
	$(ZENSICAL) serve

docs-build: $(STAMP)
	$(ZENSICAL) build

docs-check: $(STAMP)
	$(ZENSICAL) build --strict

clean:
	rm -rf site .cache bin
