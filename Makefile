VENV     := .venv
PYTHON   := $(VENV)/bin/python
ZENSICAL := $(VENV)/bin/zensical
STAMP    := $(VENV)/.installed

# Datum runs on Linux, so a build on any other machine is for development only.
# CGO is off because the agent has to run on a host with nothing installed on it.
GO_BUILD := CGO_ENABLED=0 go build

# The architecture Docker runs containers as, for the integration tests that ship a
# test binary into an image rather than installing a toolchain in it.
DOCKER_ARCH := $(shell docker version --format '{{.Server.Arch}}' 2>/dev/null)

.PHONY: help build build-linux dist test test-linux test-apt test-dnf test-sysctl test-user test-systemd test-git test-source fmt vet lint shell clean \
	package package-deb package-rpm \
	docs-install docs-serve docs-build docs-check docs-mermaid

help:
	@echo "build         Build ./bin/datum for this machine"
	@echo "build-linux   Cross-compile for linux/amd64 and linux/arm64"
	@echo "dist          Build every supported target into ./bin"
	@echo "package       Build a .deb and an .rpm into ./dist"
	@echo "test          Run the Go tests"
	@echo "test-linux    Run the Go tests in a Linux container"
	@echo "test-git      Run the git client against real signed repositories"
	@echo "test-source   Run revision selection against a real signed repository"
	@echo "test-apt      Run the apt provider against a real Debian image"
	@echo "test-dnf      Run the dnf provider against a real Fedora image"
	@echo "test-sysctl   Run the sysctl provider against a real kernel"
	@echo "test-user     Run the user provider against a real account database"
	@echo "test-systemd  Run the systemd provider against a booted systemd"
	@echo "fmt           Format the Go sources"
	@echo "vet           Run go vet"
	@echo "lint          fmt check, vet and tests, which is what CI runs"
	@echo "shell         Open a Linux container with datum and the example on it"
	@echo "docs-install  Create $(VENV) and install the documentation toolchain"
	@echo "docs-serve    Preview the documentation locally"
	@echo "docs-build    Build the site into ./site"
	@echo "docs-check    Build the site with --strict and check the diagrams"
	@echo "docs-mermaid  Parse every mermaid block with the real parser"
	@echo "clean         Remove build output and caches"

# Every source file, so a binary is rebuilt when the code changes. Without this the
# cross-compiled outputs are up to date the moment they exist, and a container ends up
# running yesterday's build against today's tests.
SOURCES := $(shell find cmd internal -name '*.go') go.mod go.sum

build: bin/datum

bin/datum: $(SOURCES)
	$(GO_BUILD) -o $@ ./cmd/datum

build-linux: bin/datum-linux-amd64 bin/datum-linux-arm64

bin/datum-linux-amd64: $(SOURCES)
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $@ ./cmd/datum

bin/datum-linux-arm64: $(SOURCES)
	GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $@ ./cmd/datum

dist: build build-linux

# Version for a package. No tags yet, so this is a placeholder that sorts below any real
# release rather than something pretending to be one.
VERSION ?= 0.1.0~dev

# The two ecosystems name the same machine differently, and both names end up in a file
# name this Makefile has to be able to predict.
RPMARCH := $(if $(filter arm64,$(DOCKER_ARCH)),aarch64,x86_64)

# Packages are built by each distribution's own tools in its own container, because how a
# package behaves on install is the part worth not guessing at.
package: package-deb package-rpm

package-deb: bin/datum-linux-$(DOCKER_ARCH)
	docker run --rm -e DEBIAN_FRONTEND=noninteractive -v "$(CURDIR):/src" -w /src debian:trixie sh -c \
		'apt-get update -qq >/dev/null && \
		 apt-get install -y -qq --no-install-recommends dpkg-dev python3 >/dev/null && \
		 packaging/build.sh deb $(DOCKER_ARCH) $(VERSION) bin/datum-linux-$(DOCKER_ARCH) dist'

package-rpm: bin/datum-linux-$(DOCKER_ARCH)
	docker run --rm -v "$(CURDIR):/src" -w /src fedora:41 sh -c \
		'dnf install -y -q rpm-build python3 >/dev/null && \
		 packaging/build.sh rpm $(DOCKER_ARCH) $(VERSION) bin/datum-linux-$(DOCKER_ARCH) dist'

test:
	go test ./...

# Applying state is implemented on Linux only, so those tests skip everywhere else.
# This runs the whole suite where all of it is reachable.
test-linux:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.25 go test ./...

# The apt provider drives real programs that install and remove packages, so these
# tests run in a throwaway container rather than on the machine you are sitting at.
# The support matrix asks for behaviour tested against a real image, and this is it.
# The golang image is itself Debian, so it has the real dpkg and apt-get rather than
# a stand-in for them.
test-apt:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.25 \
		go test -tags integration -count=1 ./internal/provider/apt/

# Fedora ships an older Go than this module needs, so the test binary is compiled here
# and carried in. What matters is the real rpm and dnf, not which Go built the test.
test-dnf:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(DOCKER_ARCH) \
		go test -c -tags integration -o bin/dnf.test ./internal/provider/dnf/
	docker run --rm -v "$(CURDIR)/bin/dnf.test:/dnf.test:ro" fedora:41 /dnf.test -test.count=1 -test.v

# A temporary directory is a poor stand-in for procfs, so these run against the real
# thing. Privileged, because writing a kernel parameter needs it, and in a container so
# the parameters that change are the container's own.
test-sysctl:
	docker run --rm --privileged -v "$(CURDIR):/src" -w /src golang:1.25 \
		go test -tags integration -count=1 ./internal/provider/procsys/

# Real accounts are created and removed, so this runs in a container rather than on the
# machine you are sitting at.
test-user:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.25 \
		go test -tags integration -count=1 ./internal/provider/linuxuser/

# systemd has to be PID 1 for any of this to mean anything, which needs a privileged
# container and a real boot rather than docker run of a single command.
test-systemd:
	docker build -q -t datum-systemd-test test/systemd
	docker rm -f datum-systemd >/dev/null 2>&1 || true
	docker run -d --name datum-systemd --privileged --cgroupns=host \
		-v /sys/fs/cgroup:/sys/fs/cgroup:rw -v "$(CURDIR):/src" -w /src \
		datum-systemd-test >/dev/null
	@for i in $$(seq 30); do \
		if docker exec datum-systemd systemctl is-system-running --wait >/dev/null 2>&1; then break; fi; \
		sleep 1; \
	done
	@docker exec datum-systemd go test -tags integration -count=1 \
		./internal/provider/systemd/ ./internal/agent/; \
		status=$$?; \
		docker rm -f datum-systemd >/dev/null; \
		exit $$status

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

docs-check: $(STAMP) docs-mermaid
	$(ZENSICAL) build --strict

# A strict build never parses mermaid, because the browser renders it. In a container so
# this needs no node installed.
docs-mermaid:
	docker run --rm -v "$(CURDIR):/src" -w /src node:22-slim sh -c \
		'npm install --prefix test/mermaid --silent --no-audit --no-fund >/dev/null && \
		 node test/mermaid/check.mjs docs'

clean:
	rm -rf site .cache bin

# The git client is tested against real repositories with real signatures, because a
# mock that returns a good verdict verifies nothing.
test-git:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.25 \
		go test -tags integration -count=1 ./internal/git/

# Revision selection is tested against a real repository with real signatures, because
# the fallback is what stops one bad commit becoming a fleet-wide outage.
test-source:
	docker run --rm -v "$(CURDIR):/src" -w /src golang:1.25 \
		go test -tags integration -count=1 ./internal/source/
