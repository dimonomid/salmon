.PHONY: all
all: salmon salmon-watch

.PHONY: generate
generate:
	go generate ./...

.PHONY: salmon
salmon: generate
	cd cmd/salmon && go build

.PHONY: salmon-watch
salmon-watch: generate
	cd cmd/salmon-watch && go build

.PHONY: install
install: install_salmon install_salmon-watch

.PHONY: install_salmon
install_salmon:
	cp cmd/salmon/salmon /usr/local/bin

.PHONY: install_salmon-watch
install_salmon-watch:
	cp cmd/salmon-watch/salmon-watch /usr/local/bin
