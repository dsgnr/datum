VENV     := .venv
PYTHON   := $(VENV)/bin/python
ZENSICAL := $(VENV)/bin/zensical
STAMP    := $(VENV)/.installed

.PHONY: help install serve build check clean

help:
	@echo "install  Create $(VENV) and install the documentation toolchain"
	@echo "serve    Preview the documentation at http://localhost:8000"
	@echo "build    Build the site into ./site"
	@echo "check    Build with --strict, failing on broken links and anchors"
	@echo "clean    Remove build output and cache"

$(STAMP): requirements-docs.txt
	python3 -m venv $(VENV)
	$(PYTHON) -m pip install --quiet --upgrade pip
	$(PYTHON) -m pip install --quiet --requirement requirements-docs.txt
	@touch $(STAMP)

install: $(STAMP)

serve: $(STAMP)
	$(ZENSICAL) serve

build: $(STAMP)
	$(ZENSICAL) build

check: $(STAMP)
	$(ZENSICAL) build --strict

clean:
	rm -rf site .cache
