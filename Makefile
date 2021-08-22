.PHONY: all
all: salmon aquascope

.PHONY: salmon
salmon:
	cd cmd/salmon && go build

.PHONY: aquascope
aquascope:
	cd cmd/aquascope && go build

.PHONY: install
install: install_salmon install_aquascope

.PHONY: install_salmon
install_salmon:
	cp cmd/salmon/salmon /usr/local/bin

.PHONY: install_aquascope
install_aquascope:
	cp cmd/aquascope/aquascope /usr/local/bin
