VERSION  := $(shell git tag | tail -n1 | sed 's/v//g')
REVISION := $(shell git rev-parse --short HEAD)
INFO_COLOR=\033[1;34m
RESET=\033[0m
BOLD=\033[1m
TEST ?= $(shell go list ./... | grep -v -e vendor -e keys -e tmp)
ifeq ("$(shell uname)","Darwin")
GO ?= GO111MODULE=on go
else
GO ?= GO111MODULE=on /usr/local/go/bin/go
endif

TESTCONFIG="misc/test.conf"

.DEFAULT_GOAL := build

.PHONY: build
## build: build the nke
build:
	go build -o binary/stnsd -ldflags "-X github.com/pyama86/chache-stnsd/cmd.version=$(VERSION)-$(REVISION)"

.PHONY: release
## release: release nke (tagging and exec goreleaser)
release:
	git semv patch --bump
	goreleaser --rm-dist

.PHONY: releasedeps
releasedeps: git-semv goreleaser

.PHONY: git-semv
git-semv:
	brew tap linyows/git-semv
	brew install git-semv

.PHONY: goreleaser
goreleaser:
	brew install goreleaser/tap/goreleaser
	brew install goreleaser


.PHONY: test
test:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Testing$(RESET)"
	$(GO) test -v $(TEST) -timeout=30s -parallel=4
	$(GO) test -race $(TEST)

.PHONY: run
run:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Runing$(RESET)"
	$(GO) run main.go -c $(TESTCONFIG) server -s /tmp/stnsd.sock

integration: ## Run integration test after Server wakeup
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Integration HTTP Testing$(RESET)"
	./misc/server start -http
	$(GO) test $(VERBOSE) -integration $(TEST) $(TEST_OPTIONS)
	./misc/server stop || true

