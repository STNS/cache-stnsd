VERSION = $(shell git tag | sed 's/v//g' |sort --version-sort | tail -n1)
REVISION := $(shell git rev-parse --short HEAD)
INFO_COLOR=\033[1;34m
RESET=\033[0m
BOLD=\033[1m
TEST ?= $(shell go list ./... | grep -v -e vendor -e keys -e tmp)

GOVERSION=$(shell go version)

TESTCONFIG="misc/test.conf"

DIST ?= unknown
PREFIX=/usr
BINDIR=$(PREFIX)/sbin
SOURCES=Makefile go.mod go.sum cmd cache_stnsd main.go package/
BUILD=tmp/bin
UNAME_S := $(shell uname -s)
.DEFAULT_GOAL := build

GOPATH ?= /go
GO=CGO_ENABLED=0 go

.PHONY: build
## build: build the nke
build:
	$(GO) build -o $(BUILD)/cache-stnsd -buildvcs=false -ldflags "-X github.com/STNS/cache-stnsd/cmd.version=$(VERSION) -s -w"

.PHONY: install
install: ## Install
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Installing as Server$(RESET)"
	cp $(BUILD)/cache-stnsd $(BINDIR)/cache-stnsd

.PHONY: release
## release: release nke (tagging and exec goreleaser)
release: goreleaser
	goreleaser --rm-dist

.PHONY: bump
bump:
	git semv patch --bump

.PHONY: releasedeps
releasedeps: git-semv goreleaser

.PHONY: git-semv
git-semv:
	brew tap linyows/git-semv
	brew install git-semv


.PHONY: goreleaser
goreleaser:
	test -e goreleaser > /dev/null || curl -sfL https://goreleaser.com/static/run | bash

.PHONY: tidy
tidy:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Tidying up$(RESET)"
	$(GO) mod tidy

.PHONY: test
test: tidy
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Testing$(RESET)"
	$(GO) test -v $(TEST) -timeout=30s -parallel=4
	CGO_ENABLED=1 go test -race $(TEST)

.PHONY: run
run: build
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Runing$(RESET)"
	tmp/bin/cache-stnsd -c $(TESTCONFIG) -p /tmp/cache-stnsd.pid -l /tmp/cache-stnsd.log server -s /tmp/cache-stnsd.sock --log-level debug

.PHONY: integration
integration: ## Run integration test after Server wakeup
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Integration HTTP Testing$(RESET)"
	./misc/server stop || true
	./misc/server start
	$(GO) test $(VERBOSE) -integration $(TEST) $(TEST_OPTIONS)
	./misc/server stop || true
