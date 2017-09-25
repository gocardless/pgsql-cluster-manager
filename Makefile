VERSION=0.0.1
PROG=pgsql-cluster-manager
PREFIX=/usr/local
BUILD_COMMAND=go build -ldflags "-X github.com/gocardless/pgsql-cluster-manager/command.Version=$(VERSION)"
PACKAGES=$(shell go list ./... | grep -v /vendor/)

.PHONY: build test clean circleci-dockerfile publish-circleci-dockerfile $(PROG).linux_amd64

build:
	$(BUILD_COMMAND) -o $(PROG) *.go

test:
	go test $(PACKAGES)

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

$(PROG).linux_amd64:
	GOOS=linux GOARCH=amd64 $(BUILD_COMMAND) -o $(PROG).linux_amd64 *.go

publish-circleci-dockerfile:
	docker build -t gocardless/pgsql-cluster-manager-circleci .circleci && \
		docker push gocardless/pgsql-cluster-manager-circleci

publish-pgsql-postgres-member-dockerfile:
	docker build -t gocardless/pgsql-postgres-member docker/postgres-member && \
		docker push gocardless/pgsql-postgres-member

clean:
	rm -vf $(PROG) $(PROG).linux_amd64 *.deb
