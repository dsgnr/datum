VENV    := .venv
PYTHON  := $(VENV)/bin/python
ZENSICAL := $(VENV)/bin/zensical

.PHONY: help install serve build check clean

help:
	@echo "install  Create $(VENV) and install the documentation toolchain"
	@echo "serve    Preview the documentation at http://localhost:8000"
	@echo "build    Build the site into ./site"
	@echo "check    Build with --strict; fails on broken links and anchors"
	@echo "clean    Remove build output and cache"

$(ZENSICAL): requirements-docs.txt
	python3 -m venv $(VENV)
	$(PYTHON) -m pip install --quiet --upgrade pip
	$(PYTHON) -m pip install --quiet --requirement requirements-docs.txt
	@touch $(ZENSICAL)

install: $(ZENSICAL)

serve: $(ZENSICAL)
	$(ZENSICAL) serve

build: $(ZENSICAL)
	$(ZENSICAL) build

check: $(ZENSICAL)
	$(ZENSICAL) build --strict

clean:
	rm -rf site .cache
