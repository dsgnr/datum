VENV     := .venv
PYTHON   := $(VENV)/bin/python
ZENSICAL := $(VENV)/bin/zensical
STAMP    := $(VENV)/.installed

.PHONY: help build test fmt vet lint clean docs-install docs-serve docs-build docs-check

help:
	@echo "build         Build the datum binary into ./bin"
	@echo "test          Run the Go tests"
	@echo "fmt           Format the Go sources"
	@echo "vet           Run go vet"
	@echo "lint          fmt check, vet and tests, which is what CI runs"
	@echo "docs-install  Create $(VENV) and install the documentation toolchain"
	@echo "docs-serve    Preview the documentation at http://localhost:8000"
	@echo "docs-build    Build the site into ./site"
	@echo "docs-check    Build the site with --strict"
	@echo "clean         Remove build output and caches"

build:
	go build -o bin/datum ./cmd/datum

test:
	go test ./...

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
