VERSION=0.0.1
PROG=pgsql-novips
PREFIX=/usr/local
BUILD_COMMAND=go build -ldflags "-X main.version=$(VERSION)"
PACKAGES=$(shell go list ./... | grep -v /vendor/)

.PHONY: build test clean circleci-dockerfile publish-circleci-dockerfile

build:
	$(BUILD_COMMAND) -o $(PROG) *.go

test:
	go test -v $(PACKAGES)

lint:
	golint $(PACKAGES)

deb: $(PROG).linux_amd64
	rm -fv *.deb
	bundle exec fpm -s dir -t $@ -n $(PROG) -v $(VERSION) \
		--architecture amd64 \
		--deb-no-default-config-files \
		--description "Orchestrator for Postgres clustering with corosync/pacemaker/etcd" \
		--maintainer "GoCardless Engineering <engineering@gocardless.com>" \
		$<=$(PREFIX)/bin/$(PROG)

$(PROG).linux_amd64: lint test
	GOOS=linux GOARCH=amd64 $(BUILD_COMMAND) -o $(PROG).linux_amd64 *.go

publish-circleci-dockerfile:
	cd .circleci && \
		docker build -t pgsql-novips-circleci . && \
		docker tag pgsql-novips-circleci gocardless/pgsql-novips-circleci:latest && \
		docker push gocardless/pgsql-novips-circleci:latest

clean:
	rm -vf $(PROG) $(PROG).linux_amd64 *.deb
