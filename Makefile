VERSION = $(shell cat version)
REVISION := $(shell git rev-parse --short HEAD)
INFO_COLOR=\033[1;34m
RESET=\033[0m
BOLD=\033[1m
TEST ?= $(shell go list ./... | grep -v -e vendor -e keys -e tmp)

GOVERSION=$(shell go version)
GO ?= GO111MODULE=on go

TESTCONFIG="misc/test.conf"

DIST ?= unknown
PREFIX=/usr
BINDIR=$(PREFIX)/sbin
SOURCES=Makefile go.mod go.sum version cmd stnsd main.go package/
DISTS=centos7 centos6 ubuntu16
BUILD=tmp/bin
UNAME_S := $(shell uname -s)
.DEFAULT_GOAL := build

.PHONY: build
## build: build the nke
build:
	$(GO) build -o $(BUILD)/stnsd -ldflags "-X github.com/STNS/cache-stnsd/cmd.version=$(VERSION)"

.PHONY: install
install: build ## Install
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Installing as Server$(RESET)"
	cp $(BUILD)/stnsd $(BINDIR)/stnsd

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
run:
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Runing$(RESET)"
	$(GO) run main.go -c $(TESTCONFIG) -p /tmp/stnsd.pid server -s /tmp/stnsd.sock

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
	cd tmp.$(DIST) && \
		tar cf cache-stnsd-$(VERSION).tar cache-stnsd-$(VERSION) && \
		gzip -9 cache-stnsd-$(VERSION).tar
	cp tmp.$(DIST)/cache-stnsd-$(VERSION).tar.gz ./builds
	rm -rf tmp.$(DIST)

.PHONY: rpm
rpm: source_for_rpm ## Packaging for RPM
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Packaging for RPM$(RESET)"
	cp builds/cache-stnsd-$(VERSION).tar.gz /root/rpmbuild/SOURCES
	spectool -g -R rpm/stnsd.spec
	rpmbuild -ba rpm/stnsd.spec
	cp /root/rpmbuild/RPMS/*/*.rpm /go/src/github.com/STNS/cache-stnsd/builds

.PHONY: pkg
pkg: ## Create some distribution packages
	rm -rf builds && mkdir builds
	docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm centos6
	docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm centos7
	docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm debian8
	docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm debian9
	docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm ubuntu16
	docker-compose run -v `pwd`:/go/src/github.com/STNS/cache-stnsd -v ~/pkg:/go/pkg --rm ubuntu18

.PHONY: source_for_deb
source_for_deb: ## Create source for DEB
	@echo "$(INFO_COLOR)==> $(RESET)$(BOLD)Distributing$(RESET)"
	rm -rf tmp.$(DIST) cache-stnsd-$(VERSION).orig.tar.gz
	mkdir -p tmp.$(DIST)/cache-stnsd-$(VERSION)
	cp -r $(SOURCES) tmp.$(DIST)/cache-stnsd-$(VERSION)
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
		cp *.deb $(GOPATH)/src/github.com/STNS/cache-stnsd/builds
	rm -rf tmp.$(DIST)

.PHONY: github_release
github_release: ## Create some distribution packages
	ghr -u STNS --replace v$(VERSION) builds/
