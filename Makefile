VERSION = $(git describe --tags --abbrev=0)
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
SOURCES=Makefile go.mod go.sum version cmd cache_stnsd main.go package/
BUILD=tmp/bin
UNAME_S := $(shell uname -s)
.DEFAULT_GOAL := build

GOPATH ?= /go
GOOS=linux
GOARCH=amd64
GO=GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go

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
release:
	goreleaser --rm-dist

.PHONY: bump
bump:
	git semv patch --bump
	git tag | tail -n1 | sed 's/v//g' > version

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

.PHONY: source_for_rpm
source_for_rpm: ## Create source for RPM
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Distributing$(RESET)"
	rm -rf tmp.$(DIST) cache-stnsd-$(VERSION).tar.gz
	mkdir -p tmp.$(DIST)/cache-stnsd-$(VERSION)
	cp -r $(SOURCES) tmp.$(DIST)/cache-stnsd-$(VERSION)
	mkdir -p tmp.$(DIST)/cache-stnsd-$(VERSION)/tmp/bin
	cp -r tmp/bin/* tmp.$(DIST)/cache-stnsd-$(VERSION)/tmp/bin
	cd tmp.$(DIST) && \
		tar cf cache-stnsd-$(VERSION).tar cache-stnsd-$(VERSION) && \
		gzip -9 cache-stnsd-$(VERSION).tar
	cp tmp.$(DIST)/cache-stnsd-$(VERSION).tar.gz ./builds
	rm -rf tmp.$(DIST)

.PHONY: rpm
rpm: source_for_rpm ## Packaging for RPM
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Packaging for RPM$(RESET)"
	cp builds/cache-stnsd-$(VERSION).tar.gz /root/rpmbuild/SOURCES
	spectool -g -R rpm/cache-stnsd.spec
	rpmbuild -ba rpm/cache-stnsd.spec
	cp /root/rpmbuild/RPMS/*/*.rpm /go/src/github.com/STNS/cache-stnsd/builds

.PHONY: pkg

SUPPORTOS=centos7 almalinux9 ubuntu20 ubuntu22 debian10 debian11
pkg: build ## Create some distribution packages
	rm -rf builds && mkdir builds
	for i in $(SUPPORTOS); do \
	  docker-compose build cache_$$i; \
	  docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm cache_$$i; \
	done


.PHONY: source_for_deb
source_for_deb: ## Create source for DEB
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Distributing$(RESET)"
	rm -rf tmp.$(DIST) cache-stnsd-$(VERSION).orig.tar.gz
	mkdir -p tmp.$(DIST)/cache-stnsd-$(VERSION)
	cp -r $(SOURCES) tmp.$(DIST)/cache-stnsd-$(VERSION)
	mkdir -p tmp.$(DIST)/cache-stnsd-$(VERSION)/tmp/bin
	cp -r tmp/bin/* tmp.$(DIST)/cache-stnsd-$(VERSION)/tmp/bin
	cd tmp.$(DIST) && \
	tar zcf cache-stnsd-$(VERSION).tar.gz cache-stnsd-$(VERSION)
	mv tmp.$(DIST)/cache-stnsd-$(VERSION).tar.gz tmp.$(DIST)/cache-stnsd-$(VERSION).orig.tar.gz

.PHONY: deb
deb: source_for_deb ## Packaging for DEB
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Packaging for DEB$(RESET)"
	cd tmp.$(DIST) && \
		tar xf cache-stnsd-$(VERSION).orig.tar.gz && \
		cd cache-stnsd-$(VERSION) && \
		dh_make --single --createorig -y && \
		rm -rf debian/*.ex debian/*.EX debian/README.Debian && \
		cp -r $(GOPATH)/src/github.com/STNS/cache-stnsd/debian/* debian/ && \
		sed -i -e 's/xenial/$(DIST)/g' debian/changelog && \
		debuild -uc -us
	cd tmp.$(DIST) && \
		find . -name "*.deb" | sed -e 's/\(\(.*cache-stnsd.*\).deb\)/mv \1 \2.$(DIST).deb/g' | sh && \
		mkdir -p $(GOPATH)/src/github.com/STNS/cache-stnsd/builds && \
		cp *.deb $(GOPATH)/src/github.com/STNS/cache-stnsd/builds
	rm -rf tmp.$(DIST)

.PHONY: github_release
github_release: ## Create some distribution packages
	ghr -u STNS --replace v$(VERSION) builds/
