.PHONY: all
all: salmon aquascope

.PHONY: generate
generate:
	go generate ./...

.PHONY: salmon
salmon: generate
	cd cmd/salmon && go build

.PHONY: aquascope
aquascope: generate
	cd cmd/aquascope && go build

.PHONY: install
install: install_salmon install_aquascope

.PHONY: install_salmon
install_salmon:
	cp cmd/salmon/salmon /usr/local/bin

.PHONY: install_aquascope
install_aquascope:
	cp cmd/aquascope/aquascope /usr/local/bin
