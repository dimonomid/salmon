SHELL := bash

VERSION != git describe --dirty --tags --always
COMMIT != git rev-parse HEAD
DATE != date -u +"%Y-%m-%dT%H:%M:%SZ"
GOEXE != go env GOEXE

LDFLAGS := -s -w \
	-X 'github.com/dimonomid/salmon/version.version=$(patsubst v%,%,$(VERSION))' \
	-X 'github.com/dimonomid/salmon/version.commit=$(COMMIT)' \
	-X 'github.com/dimonomid/salmon/version.date=$(DATE)' \
	-X 'github.com/dimonomid/salmon/version.builtBy=make'

.PHONY: all
all: clean salmon salmon-watch

.PHONY: test
test:
	go test --count 1 --race ./...
	node --test cmd/salmon-watch/jstest/*.js

.PHONY: generate
generate:
	go generate ./...

.PHONY: salmon
salmon: generate
	@go build \
		-trimpath \
		-o bin/salmon$(GOEXE) \
		-ldflags "$(LDFLAGS)" \
		./cmd/salmon

.PHONY: salmon-watch
salmon-watch: generate
	@go build \
		-trimpath \
		-o bin/salmon-watch$(GOEXE) \
		-ldflags "$(LDFLAGS)" \
		./cmd/salmon-watch

.PHONY: clean
clean:
	rm -rf bin

PREFIX ?= /usr/local
DESTDIR ?=
BINDIR := $(DESTDIR)$(PREFIX)/bin
INSTALL := install
INSTALL_FLAGS := -m 755

.PHONY: install
install: install-salmon install-salmon-watch

.PHONY: install-salmon
install-salmon:
	$(INSTALL) $(INSTALL_FLAGS) -D bin/salmon$(GOEXE) $(BINDIR)/salmon$(GOEXE)

.PHONY: install-salmon-watch
install-salmon-watch:
	$(INSTALL) $(INSTALL_FLAGS) -D bin/salmon-watch$(GOEXE) $(BINDIR)/salmon-watch$(GOEXE)
